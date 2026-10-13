#include "window_manager_client.hpp"

#include "tacos/runtime/runtime.hpp"

#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

#include <cerrno>
#include <cstring>
#include <sstream>
#include <string>

namespace tacos::runtime::detail {
namespace {

std::string json_escape(const std::string& input) {
    std::string result;

    for (unsigned char c : input) {
        switch (c) {
        case '"':  result += "\\\""; break;
        case '\\': result += "\\\\"; break;
        case '\n': result += "\\n"; break;
        case '\r': result += "\\r"; break;
        case '\t': result += "\\t"; break;
        default:
            if (c < 0x20) {
                throw RuntimeError(
                    "WM request contains an unsupported control character"
                );
            }
            result += static_cast<char>(c);
        }
    }

    return result;
}

bool response_ok(const std::string& response) {
    const auto key = response.find("\"ok\"");
    if (key == std::string::npos) {
        throw RuntimeError("WM response is missing 'ok'");
    }

    const auto colon = response.find(':', key + 4);
    if (colon == std::string::npos) {
        throw RuntimeError("malformed WM response");
    }

    auto value = colon + 1;
    while (value < response.size() &&
           (response[value] == ' ' || response[value] == '\t')) {
        ++value;
    }

    if (response.compare(value, 4, "true") == 0) {
        return true;
    }
    if (response.compare(value, 5, "false") == 0) {
        return false;
    }

    throw RuntimeError("invalid 'ok' value in WM response");
}

} // namespace

void send_window_manager_command(
    const std::string& socket_path,
    const std::string& command,
    const std::string& window_id,
    const std::string& title,
    std::int64_t width,
    std::int64_t height
) {
    if (socket_path.empty() ||
        socket_path.size() >= sizeof(sockaddr_un::sun_path)) {
        throw RuntimeError("invalid WM socket path");
    }

    const int fd = ::socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) {
        throw RuntimeError(
            "cannot create WM socket: " +
            std::string(std::strerror(errno))
        );
    }

    struct SocketCloser {
        int fd;
        ~SocketCloser() { if (fd >= 0) ::close(fd); }
    } closer{fd};

    sockaddr_un address{};
    address.sun_family = AF_UNIX;
    std::memcpy(
        address.sun_path,
        socket_path.c_str(),
        socket_path.size() + 1
    );

    if (::connect(
            fd,
            reinterpret_cast<const sockaddr*>(&address),
            sizeof(address)
        ) < 0) {
        throw RuntimeError(
            "cannot connect to WM at " + socket_path + ": " +
            std::string(std::strerror(errno))
        );
    }

    std::ostringstream request;
    request << "{\"command\":\"" << json_escape(command) << "\"";

    if (!window_id.empty()) {
        request << ",\"window_id\":\"" << json_escape(window_id) << "\"";
    }
    if (!title.empty()) {
        request << ",\"title\":\"" << json_escape(title) << "\"";
    }
    if (width != 0) {
        request << ",\"width\":" << width;
    }
    if (height != 0) {
        request << ",\"height\":" << height;
    }

    request << "}\n";
    const std::string bytes = request.str();

    std::size_t sent = 0;
    while (sent < bytes.size()) {
        const ssize_t n = ::send(
            fd, bytes.data() + sent, bytes.size() - sent, MSG_NOSIGNAL
        );

        if (n < 0 && errno == EINTR) {
            continue;
        }
        if (n <= 0) {
            throw RuntimeError("failed to send request to WM");
        }

        sent += static_cast<std::size_t>(n);
    }

    std::string response;
    char buffer[1024];

    while (response.find('\n') == std::string::npos) {
        const ssize_t n = ::recv(fd, buffer, sizeof(buffer), 0);

        if (n < 0 && errno == EINTR) {
            continue;
        }
        if (n < 0) {
            throw RuntimeError("failed to receive response from WM");
        }
        if (n == 0) {
            break;
        }

        response.append(buffer, static_cast<std::size_t>(n));

        if (response.size() > 64 * 1024) {
            throw RuntimeError("WM response is too large");
        }
    }

    const auto newline = response.find('\n');
    if (newline == std::string::npos) {
        throw RuntimeError("WM closed connection without a complete response");
    }

    response.resize(newline);

    if (!response_ok(response)) {
        throw RuntimeError(
            "WM rejected command " + command + ": " + response
        );
    }
}

} // namespace tacos::runtime::detail
