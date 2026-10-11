#pragma once

#include <memory>
#include <string>

struct wl_display;
struct wl_event_loop;
struct wlr_backend;
struct wlr_renderer;
struct wlr_allocator;
struct wlr_scene;
struct wlr_scene_output_layout;
struct wlr_output_layout;
struct wlr_xdg_shell;
struct wl_listener;

namespace tacos::wm {

class Compositor {
public:
    Compositor();
    ~Compositor();
    Compositor(const Compositor&) = delete;
    Compositor& operator=(const Compositor&) = delete;

    bool initialize();
    void run();
    void stop();
    const char* wayland_socket_name() const;
    wl_event_loop* event_loop() const;
    void set_window_visible(bool visible);
    void set_window_size(int width, int height);
    void set_window_title(const std::string& title);

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
};

} // namespace tacos::wm
