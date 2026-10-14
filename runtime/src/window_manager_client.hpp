#pragma once

#include <cstdint>
#include <string>

namespace tacos::runtime::detail {

void send_window_manager_command(
    const std::string& socket_path,
    const std::string& command,
    const std::string& window_id = "",
    const std::string& title = "",
    std::int64_t width = 0,
    std::int64_t height = 0
);

} // namespace tacos::runtime::detail
