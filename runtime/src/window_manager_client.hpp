
#pragma once

#include <cstdint>
#include <nlohmann/json.hpp>
#include <string>

namespace tacos::runtime::detail {

// Existing commands: CREATE, SHOW, CLOSE, QUIT.
void send_window_manager_command(const std::string &socket_path,
                                 const std::string &command,
                                 const std::string &window_id = "",
                                 const std::string &title = "",
                                 std::int64_t width = 0,
                                 std::int64_t height = 0);

// Drawing commands: BEGIN_FRAME, DRAW_IMAGE,
// DRAW_CIRCLE, PRESENT.
void send_window_manager_request(const std::string &socket_path,
                                 const nlohmann::json &request);

} // namespace tacos::runtime::detail
