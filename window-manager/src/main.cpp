#include "tacos/wm/compositor.hpp"
#include "tacos/wm/control_server.hpp"
#include "tacos/wm/protocol.hpp"

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <wayland-server-core.h>
extern "C" {
extern "C" {
#include <wlr/util/log.h>
}
}

int main(int argc, char** argv) {
    std::string socket_path = tacos::wm::kDefaultSocket;
    std::string client_path = "./tac_wm_window_client";
    for (int i = 1; i < argc; ++i) {
        if (std::strcmp(argv[i], "--socket") == 0 && i + 1 < argc) socket_path = argv[++i];
        else if (std::strcmp(argv[i], "--window-client") == 0 && i + 1 < argc) client_path = argv[++i];
        else if (std::strcmp(argv[i], "--help") == 0) {
            std::printf("Usage: tac_wm [--socket PATH] [--window-client PATH]\n");
            return 0;
        } else {
            std::fprintf(stderr, "Unknown argument: %s\n", argv[i]);
            return 2;
        }
    }

    wlr_log_init(WLR_INFO, nullptr);
    tacos::wm::Compositor compositor;
    if (!compositor.initialize()) {
        std::fprintf(stderr, "Window Manager: failed to initialize Wayland compositor\n");
        return 1;
    }
    tacos::wm::ControlServer control(compositor, socket_path, client_path);
    if (!control.initialize()) {
        std::fprintf(stderr, "Window Manager: failed to create control socket %s\n", socket_path.c_str());
        return 1;
    }
    std::printf("TACOS_WM_SOCKET=%s\n", socket_path.c_str());
    std::printf("WAYLAND_DISPLAY=%s\n", compositor.wayland_socket_name());
    std::fflush(stdout);
    compositor.run();
    return 0;
}
