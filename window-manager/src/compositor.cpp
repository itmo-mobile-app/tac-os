#include "tacos/wm/compositor.hpp"

#include <ctime>
#include <cstddef>
#include <cstdio>
#include <cstring>
#include <memory>
#include <string>
#include <wayland-server-core.h>
extern "C" {
#define static
#include <wlr/types/wlr_scene.h>
#undef static
#include <wlr/backend.h>
#include <wlr/render/allocator.h>
#include <wlr/render/wlr_renderer.h>
#include <wlr/types/wlr_compositor.h>
#include <wlr/types/wlr_cursor.h>
#include <wlr/types/wlr_input_device.h>
#include <wlr/types/wlr_pointer.h>
#include <wlr/types/wlr_seat.h>
#include <wlr/types/wlr_output.h>
#include <wlr/types/wlr_output_layout.h>
#include <wlr/types/wlr_xdg_shell.h>
#include <wlr/util/log.h>
}

namespace tacos::wm {

struct Compositor::Impl {
    wl_display* display = nullptr;
    wl_event_loop* loop = nullptr;
    wlr_backend* backend = nullptr;
    wlr_renderer* renderer = nullptr;
    wlr_allocator* allocator = nullptr;
    wlr_scene* scene = nullptr;
    wlr_output_layout* output_layout = nullptr;
    wlr_xdg_shell* xdg_shell = nullptr;
    wl_listener new_output{};
    wl_listener new_xdg_surface{};
    wl_listener xdg_destroy{};
    wl_listener new_input{};
    wl_listener cursor_motion{};
    wl_listener cursor_motion_absolute{};
    wl_listener cursor_button{};
    wlr_cursor* cursor = nullptr;
    wlr_seat* seat = nullptr;
    wlr_xdg_surface* active_xdg_surface = nullptr;
    std::string socket_name;
    wlr_scene_tree* window_node = nullptr;
    bool visible = false;
    int width = 960;
    int height = 640;
    std::string title = "Tactical Map";

    struct Output {
        Impl* self;
        wlr_output* output;
        wl_listener frame{};
        wl_listener destroy{};
    };
    static void handle_output_frame(wl_listener* listener, void*) {
        auto* out = reinterpret_cast<Output*>(reinterpret_cast<char*>(listener) - offsetof(Output, frame));
        auto* scene_output = wlr_scene_get_scene_output(out->self->scene, out->output);
        if (!scene_output) return;
        if (wlr_scene_output_commit(scene_output, nullptr)) {
            timespec now{};
            clock_gettime(CLOCK_MONOTONIC, &now);
            wlr_scene_output_send_frame_done(scene_output, &now);
        }
    }
    static void handle_output_destroy(wl_listener* listener, void*) {
        auto* out = reinterpret_cast<Output*>(reinterpret_cast<char*>(listener) - offsetof(Output, destroy));
        wl_list_remove(&out->frame.link);
        wl_list_remove(&out->destroy.link);
        delete out;
    }
    static void handle_new_output(wl_listener* listener, void* data) {
        auto* self = reinterpret_cast<Impl*>(reinterpret_cast<char*>(listener) - offsetof(Impl, new_output));
        auto* output = static_cast<wlr_output*>(data);
        if (!wlr_output_init_render(output, self->allocator, self->renderer)) {
            wlr_log(WLR_ERROR, "Failed to initialize output renderer");
            return;
        }
        wlr_output_state state;
        wlr_output_state_init(&state);
        wlr_output_state_set_enabled(&state, true);
        if (!wl_list_empty(&output->modes)) {
            auto* mode = wlr_output_preferred_mode(output);
            if (mode != nullptr) {
                wlr_output_state_set_mode(&state, mode);
            }
        }
        if (!wlr_output_commit_state(output, &state)) {
            wlr_output_state_finish(&state);
            wlr_log(WLR_ERROR, "Failed to commit output state");
            return;
        }
        wlr_output_state_finish(&state);
        wlr_output_layout_add_auto(self->output_layout, output);
        if (wlr_scene_output_create(self->scene, output) == nullptr) {
            wlr_log(WLR_ERROR, "Failed to create scene output for %s", output->name);
            return;
        }
        auto* wrapper = new Output{};
        wrapper->self = self;
        wrapper->output = output;
        wrapper->frame.notify = handle_output_frame;
        wl_signal_add(&output->events.frame, &wrapper->frame);
        wrapper->destroy.notify = handle_output_destroy;
        wl_signal_add(&output->events.destroy, &wrapper->destroy);
        wlr_output_schedule_frame(output);
        wlr_log(WLR_INFO, "Output connected: %s", output->name);
    }
    static void handle_xdg_destroy(wl_listener* listener, void*) {
        auto* self = reinterpret_cast<Impl*>(reinterpret_cast<char*>(listener) - offsetof(Impl, xdg_destroy));
        wl_list_remove(&self->xdg_destroy.link);
        wl_list_init(&self->xdg_destroy.link);
        self->window_node = nullptr;
        self->active_xdg_surface = nullptr;
    }
    static void process_pointer(Impl* self, uint32_t time_msec) {
        if (!self->active_xdg_surface || !self->window_node) return;
        const double sx = self->cursor->x - 80.0;
        const double sy = self->cursor->y - 60.0;
        wlr_seat_pointer_notify_enter(self->seat, self->active_xdg_surface->surface, sx, sy);
        wlr_seat_pointer_notify_motion(self->seat, time_msec, sx, sy);
    }
    static void handle_cursor_motion(wl_listener* listener, void* data) {
        auto* self = reinterpret_cast<Impl*>(reinterpret_cast<char*>(listener) - offsetof(Impl, cursor_motion));
        auto* event = static_cast<wlr_pointer_motion_event*>(data);
        wlr_cursor_move(self->cursor, nullptr, event->delta_x, event->delta_y);
        process_pointer(self, event->time_msec);
    }
    static void handle_cursor_motion_absolute(wl_listener* listener, void* data) {
        auto* self = reinterpret_cast<Impl*>(reinterpret_cast<char*>(listener) - offsetof(Impl, cursor_motion_absolute));
        auto* event = static_cast<wlr_pointer_motion_absolute_event*>(data);
        wlr_cursor_warp_absolute(self->cursor, nullptr, event->x, event->y);
        process_pointer(self, event->time_msec);
    }
    static void handle_cursor_button(wl_listener* listener, void* data) {
        auto* self = reinterpret_cast<Impl*>(reinterpret_cast<char*>(listener) - offsetof(Impl, cursor_button));
        auto* event = static_cast<wlr_pointer_button_event*>(data);
        process_pointer(self, event->time_msec);
        wlr_seat_pointer_notify_button(self->seat, event->time_msec, event->button, event->state);
    }
    static void handle_new_input(wl_listener* listener, void* data) {
        auto* self = reinterpret_cast<Impl*>(reinterpret_cast<char*>(listener) - offsetof(Impl, new_input));
        auto* device = static_cast<wlr_input_device*>(data);
        if (device->type == WLR_INPUT_DEVICE_POINTER) {
            wlr_cursor_attach_input_device(self->cursor, device);
            wlr_seat_set_capabilities(self->seat, WL_SEAT_CAPABILITY_POINTER);
        }
    }
    static void handle_new_xdg_surface(wl_listener* listener, void* data) {
        auto* self = reinterpret_cast<Impl*>(reinterpret_cast<char*>(listener) - offsetof(Impl, new_xdg_surface));
        auto* xdg_surface = static_cast<wlr_xdg_surface*>(data);
        if (self->window_node) {
            // This MVP intentionally admits one app window only; popups and extra toplevels are out of scope.
            return;
        }
        self->window_node = wlr_scene_xdg_surface_create(&self->scene->tree, xdg_surface);
        if (!self->window_node) return;
        self->active_xdg_surface = xdg_surface;
        self->xdg_destroy.notify = handle_xdg_destroy;
        wl_signal_add(&xdg_surface->events.destroy, &self->xdg_destroy);
        wlr_scene_node_set_position(&self->window_node->node, 80, 60);
        wlr_scene_node_set_enabled(&self->window_node->node, self->visible);
    }
};

Compositor::Compositor() : impl_(std::make_unique<Impl>()) {}
Compositor::~Compositor() {
    if (impl_->display) wl_display_destroy_clients(impl_->display);
    if (impl_->display) wl_display_destroy(impl_->display);
}

bool Compositor::initialize() {
    auto& s = *impl_;
    s.display = wl_display_create();
    if (!s.display) return false;
    s.loop = wl_display_get_event_loop(s.display);
    s.backend = wlr_backend_autocreate(s.display, nullptr);
    if (!s.backend) {
        wlr_log(WLR_ERROR, "No usable wlroots backend (run inside a graphical session for nested mode)");
        return false;
    }
    s.renderer = wlr_renderer_autocreate(s.backend);
    if (!s.renderer || !wlr_renderer_init_wl_display(s.renderer, s.display)) return false;
    s.allocator = wlr_allocator_autocreate(s.backend, s.renderer);
    if (!s.allocator) return false;
    wlr_compositor_create(s.display, 5, s.renderer);
    s.output_layout = wlr_output_layout_create();
    if (!s.output_layout) return false;
    s.scene = wlr_scene_create();
    if (!s.scene) return false;
    wlr_scene_attach_output_layout(s.scene, s.output_layout);
    s.cursor = wlr_cursor_create();
    if (!s.cursor) return false;
    wlr_cursor_attach_output_layout(s.cursor, s.output_layout);
    s.seat = wlr_seat_create(s.display, "seat0");
    if (!s.seat) return false;
    s.cursor_motion.notify = Impl::handle_cursor_motion;
    wl_signal_add(&s.cursor->events.motion, &s.cursor_motion);
    s.cursor_motion_absolute.notify = Impl::handle_cursor_motion_absolute;
    wl_signal_add(&s.cursor->events.motion_absolute, &s.cursor_motion_absolute);
    s.cursor_button.notify = Impl::handle_cursor_button;
    wl_signal_add(&s.cursor->events.button, &s.cursor_button);
    s.new_input.notify = Impl::handle_new_input;
    wl_signal_add(&s.backend->events.new_input, &s.new_input);
    s.xdg_shell = wlr_xdg_shell_create(s.display, 3);
    if (!s.xdg_shell) return false;
    s.new_xdg_surface.notify = Impl::handle_new_xdg_surface;
    wl_signal_add(&s.xdg_shell->events.new_surface, &s.new_xdg_surface);
    s.new_output.notify = Impl::handle_new_output;
    wl_signal_add(&s.backend->events.new_output, &s.new_output);
    const char* name = wl_display_add_socket_auto(s.display);
    if (!name) return false;
    s.socket_name = name;
    if (!wlr_backend_start(s.backend)) return false;
    return true;
}
void Compositor::run() { wl_display_run(impl_->display); }
void Compositor::stop() { if (impl_->display) wl_display_terminate(impl_->display); }
const char* Compositor::wayland_socket_name() const { return impl_->socket_name.c_str(); }
wl_event_loop* Compositor::event_loop() const { return impl_->loop; }
void Compositor::set_window_visible(bool visible) {
    impl_->visible = visible;
    if (impl_->window_node) wlr_scene_node_set_enabled(&impl_->window_node->node, visible);
}
void Compositor::set_window_size(int width, int height) { impl_->width = width; impl_->height = height; }
void Compositor::set_window_title(const std::string& title) { impl_->title = title; }

} // namespace tacos::wm
