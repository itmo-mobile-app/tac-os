#pragma once

#include <cstdint>
#include <string>

namespace tacos::wm {

// One UTF-8 command per line over a Unix-domain stream socket.
// V1 intentionally has a tiny grammar so no JSON dependency is required.
struct Command {
    std::string name;
    std::int64_t window_id = 0;
    int width = 0;
    int height = 0;
    std::string title;
};

inline constexpr const char* kDefaultSocket = "/tmp/tac-os-wm.sock";

} // namespace tacos::wm
