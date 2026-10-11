#define _GNU_SOURCE
#include <wayland-client.h>
#include "xdg-shell-client-protocol.h"

#include <linux/input-event-codes.h>
#include <cerrno>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <sys/socket.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/un.h>
#include <unistd.h>

struct Client {
    wl_display* display = nullptr;
    wl_registry* registry = nullptr;
    wl_compositor* compositor = nullptr;
    wl_shm* shm = nullptr;
    wl_seat* seat = nullptr;
    wl_pointer* pointer = nullptr;
    xdg_wm_base* wm_base = nullptr;
    wl_surface* surface = nullptr;
    xdg_surface* xdg_surface_obj = nullptr;
    xdg_toplevel* toplevel = nullptr;
    wl_buffer* buffer = nullptr;
    int width = 960;
    int height = 640;
    bool configured = false;
    double pointer_x = 0;
    double pointer_y = 0;
    long long window_id = 1;
    std::string socket_path = "/tmp/tac-os-wm.sock";
};

static std::string control_request(const std::string& path, const std::string& command) {
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) return {};
    sockaddr_un address{};
    address.sun_family = AF_UNIX;
    if (path.size() >= sizeof(address.sun_path)) { close(fd); return {}; }
    std::strncpy(address.sun_path, path.c_str(), sizeof(address.sun_path) - 1);
    if (connect(fd, reinterpret_cast<sockaddr*>(&address), sizeof(address)) < 0) { close(fd); return {}; }
    const std::string request = command + "\n";
    if (send(fd, request.data(), request.size(), 0) < 0) { close(fd); return {}; }
    char buffer[1024]{};
    ssize_t n = recv(fd, buffer, sizeof(buffer) - 1, 0);
    close(fd);
    return n > 0 ? std::string(buffer, static_cast<size_t>(n)) : std::string{};
}

static void wm_ping(void* data, xdg_wm_base* base, uint32_t serial) {
    (void)data; xdg_wm_base_pong(base, serial);
}
static const xdg_wm_base_listener wm_listener = {wm_ping};

static void pointer_enter(void* data, wl_pointer*, uint32_t, wl_surface*, wl_fixed_t sx, wl_fixed_t sy) {
    auto* c = static_cast<Client*>(data);
    c->pointer_x = wl_fixed_to_double(sx); c->pointer_y = wl_fixed_to_double(sy);
}
static void pointer_leave(void*, wl_pointer*, uint32_t, wl_surface*) {}
static void pointer_motion(void* data, wl_pointer*, uint32_t, wl_fixed_t sx, wl_fixed_t sy) {
    auto* c = static_cast<Client*>(data);
    c->pointer_x = wl_fixed_to_double(sx); c->pointer_y = wl_fixed_to_double(sy);
}
static void pointer_button(void* data, wl_pointer*, uint32_t, uint32_t, uint32_t button, uint32_t state) {
    auto* c = static_cast<Client*>(data);
    if (button == BTN_LEFT && state == WL_POINTER_BUTTON_STATE_PRESSED &&
        c->pointer_x >= c->width - 42 && c->pointer_y < 42) {
        (void)control_request(c->socket_path, "USER_CLOSE " + std::to_string(c->window_id));
    }
}
static void pointer_axis(void*, wl_pointer*, uint32_t, uint32_t, wl_fixed_t) {}
static void pointer_frame(void*, wl_pointer*) {}
static void pointer_axis_source(void*, wl_pointer*, uint32_t) {}
static void pointer_axis_stop(void*, wl_pointer*, uint32_t, uint32_t) {}
static void pointer_axis_discrete(void*, wl_pointer*, uint32_t, int32_t) {}
static void pointer_axis_value120(void*, wl_pointer*, uint32_t, int32_t) {}
static const wl_pointer_listener pointer_listener = {
    pointer_enter, pointer_leave, pointer_motion, pointer_button, pointer_axis,
    pointer_frame, pointer_axis_source, pointer_axis_stop, pointer_axis_discrete,
#if WL_POINTER_AXIS_VALUE120_SINCE_VERSION
    pointer_axis_value120
#endif
};
static void seat_capabilities(void* data, wl_seat* seat, uint32_t capabilities) {
    auto* c = static_cast<Client*>(data);
    if ((capabilities & WL_SEAT_CAPABILITY_POINTER) && !c->pointer) {
        c->pointer = wl_seat_get_pointer(seat);
        wl_pointer_add_listener(c->pointer, &pointer_listener, c);
    } else if (!(capabilities & WL_SEAT_CAPABILITY_POINTER) && c->pointer) {
        wl_pointer_destroy(c->pointer); c->pointer = nullptr;
    }
}
static void seat_name(void*, wl_seat*, const char*) {}
static const wl_seat_listener seat_listener = {seat_capabilities, seat_name};

static void registry_global(void* data, wl_registry* registry, uint32_t name,
                            const char* interface, uint32_t version) {
    auto* c = static_cast<Client*>(data);
    if (std::strcmp(interface, wl_compositor_interface.name) == 0) {
        c->compositor = static_cast<wl_compositor*>(wl_registry_bind(registry, name,
            &wl_compositor_interface, version < 4 ? version : 4));
    } else if (std::strcmp(interface, wl_shm_interface.name) == 0) {
        c->shm = static_cast<wl_shm*>(wl_registry_bind(registry, name, &wl_shm_interface, 1));
    } else if (std::strcmp(interface, wl_seat_interface.name) == 0) {
        c->seat = static_cast<wl_seat*>(wl_registry_bind(registry, name, &wl_seat_interface, version < 7 ? version : 7));
        wl_seat_add_listener(c->seat, &seat_listener, c);
    } else if (std::strcmp(interface, xdg_wm_base_interface.name) == 0) {
        c->wm_base = static_cast<xdg_wm_base*>(wl_registry_bind(registry, name,
            &xdg_wm_base_interface, version < 3 ? version : 3));
        xdg_wm_base_add_listener(c->wm_base, &wm_listener, c);
    }
}
static void registry_remove(void*, wl_registry*, uint32_t) {}
static const wl_registry_listener registry_listener = {registry_global, registry_remove};

static int create_shm_file(size_t size) {
    char name[] = "/tmp/tacos-wm-buffer-XXXXXX";
    int fd = mkstemp(name);
    if (fd < 0) return -1;
    unlink(name);
    if (ftruncate(fd, static_cast<off_t>(size)) < 0) { close(fd); return -1; }
    return fd;
}
static void draw_buffer(Client* c) {
    const int stride = c->width * 4;
    const size_t size = static_cast<size_t>(stride) * static_cast<size_t>(c->height);
    int fd = create_shm_file(size);
    if (fd < 0) return;
    auto* pixels = static_cast<uint32_t*>(mmap(nullptr, size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0));
    if (pixels == MAP_FAILED) { close(fd); return; }
    for (int y = 0; y < c->height; ++y) {
        for (int x = 0; x < c->width; ++x) {
            uint32_t color = 0xff252a32;
            if (y < 42) color = 0xff164e63;
            if (x >= c->width - 42 && y < 42) {
                color = (x > c->width - 38 && x < c->width - 4 && y > 4 && y < 38) ? 0xffc0392b : 0xff164e63;
                const int dx = x - (c->width - 21), dy = y - 21;
                if ((dx - dy < 2 && dx - dy > -2) || (dx + dy < 2 && dx + dy > -2)) color = 0xffffffff;
            }
            pixels[y * c->width + x] = color;
        }
    }
    wl_shm_pool* pool = wl_shm_create_pool(c->shm, fd, static_cast<int32_t>(size));
    c->buffer = wl_shm_pool_create_buffer(pool, 0, c->width, c->height, stride, WL_SHM_FORMAT_XRGB8888);
    wl_shm_pool_destroy(pool);
    munmap(pixels, size);
    close(fd);
}
static void xdg_surface_configure(void* data, xdg_surface* surface, uint32_t serial) {
    auto* c = static_cast<Client*>(data);
    xdg_surface_ack_configure(surface, serial);
    if (!c->configured) {
        c->configured = true;
        draw_buffer(c);
        if (c->buffer) {
            wl_surface_attach(c->surface, c->buffer, 0, 0);
            wl_surface_damage(c->surface, 0, 0, c->width, c->height);
            wl_surface_commit(c->surface);
        }
    }
}
static const xdg_surface_listener surface_listener = {xdg_surface_configure};
static void toplevel_configure(void*, xdg_toplevel*, int32_t, int32_t, wl_array*) {}
static void toplevel_close(void*, xdg_toplevel*) {}
static const xdg_toplevel_listener toplevel_listener = {toplevel_configure, toplevel_close};

int main(int argc, char** argv) {
    Client c;
    if (argc >= 3) { c.width = std::atoi(argv[1]); c.height = std::atoi(argv[2]); }
    const std::string title = argc >= 4 ? argv[3] : "Tactical Map";
    if (argc >= 5) c.window_id = std::atoll(argv[4]);
    if (argc >= 6) c.socket_path = argv[5];
    c.display = wl_display_connect(nullptr);
    if (!c.display) { std::fprintf(stderr, "window client: cannot connect to WAYLAND_DISPLAY\n"); return 1; }
    c.registry = wl_display_get_registry(c.display);
    wl_registry_add_listener(c.registry, &registry_listener, &c);
    wl_display_roundtrip(c.display);
    if (!c.compositor || !c.shm || !c.wm_base) {
        std::fprintf(stderr, "window client: compositor lacks required Wayland globals\n"); return 1;
    }
    c.surface = wl_compositor_create_surface(c.compositor);
    c.xdg_surface_obj = xdg_wm_base_get_xdg_surface(c.wm_base, c.surface);
    xdg_surface_add_listener(c.xdg_surface_obj, &surface_listener, &c);
    c.toplevel = xdg_surface_get_toplevel(c.xdg_surface_obj);
    xdg_toplevel_add_listener(c.toplevel, &toplevel_listener, &c);
    xdg_toplevel_set_title(c.toplevel, title.c_str());
    xdg_toplevel_set_app_id(c.toplevel, "tac-os-map");
    wl_surface_commit(c.surface);
    while (wl_display_dispatch(c.display) >= 0) {}
    if (c.buffer) wl_buffer_destroy(c.buffer);
    if (c.toplevel) xdg_toplevel_destroy(c.toplevel);
    if (c.xdg_surface_obj) xdg_surface_destroy(c.xdg_surface_obj);
    if (c.surface) wl_surface_destroy(c.surface);
    if (c.pointer) wl_pointer_destroy(c.pointer);
    if (c.seat) wl_seat_destroy(c.seat);
    if (c.wm_base) xdg_wm_base_destroy(c.wm_base);
    if (c.shm) wl_shm_destroy(c.shm);
    if (c.compositor) wl_compositor_destroy(c.compositor);
    if (c.registry) wl_registry_destroy(c.registry);
    wl_display_disconnect(c.display);
    return 0;
}
