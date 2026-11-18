
#include "window_manager_client.hpp"

#include "tacos/runtime/runtime.hpp"

#include <fcntl.h>
#include <poll.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

#include <cerrno>
#include <chrono>
#include <cstring>
#include <limits>
#include <string>

namespace tacos::runtime::detail {
namespace {

using Clock = std::chrono::steady_clock;

constexpr auto kWmTimeout = std::chrono::milliseconds(1000);
constexpr std::size_t kMaxResponse = 64 * 1024;

struct SocketCloser {
  int fd = -1;

  ~SocketCloser() {
    if (fd >= 0) {
      ::close(fd);
    }
  }

  SocketCloser(const SocketCloser &) = delete;
  SocketCloser &operator=(const SocketCloser &) = delete;
};

void wait_for_socket(int fd, short events, Clock::time_point deadline) {
  while (true) {
    const auto now = Clock::now();

    if (now >= deadline) {
      throw RuntimeError("WM command timed out after 1000 ms");
    }

    const auto remaining =
        std::chrono::duration_cast<std::chrono::milliseconds>(deadline - now);

    const auto milliseconds = remaining.count() > 0 ? remaining.count() : 1;

    pollfd descriptor{};
    descriptor.fd = fd;
    descriptor.events = events;

    const int result = ::poll(&descriptor, 1, static_cast<int>(milliseconds));

    if (result > 0) {
      if (descriptor.revents & POLLNVAL) {
        throw RuntimeError("invalid WM socket");
      }

      if (descriptor.revents & (events | POLLERR | POLLHUP)) {
        return;
      }

      continue;
    }

    if (result == 0) {
      continue;
    }

    if (errno != EINTR) {
      throw RuntimeError("WM poll failed: " +
                         std::string(std::strerror(errno)));
    }
  }
}

void connect_to_wm(int fd, const std::string &socket_path,
                   Clock::time_point deadline) {
  sockaddr_un address{};
  address.sun_family = AF_UNIX;

  std::memcpy(address.sun_path, socket_path.c_str(), socket_path.size() + 1);

  const int result = ::connect(fd, reinterpret_cast<const sockaddr *>(&address),
                               sizeof(address));

  if (result == 0) {
    return;
  }

  if (errno != EINPROGRESS && errno != EINTR) {
    throw RuntimeError("cannot connect to WM: " +
                       std::string(std::strerror(errno)));
  }

  wait_for_socket(fd, POLLOUT, deadline);

  int error = 0;
  socklen_t length = sizeof(error);

  if (::getsockopt(fd, SOL_SOCKET, SO_ERROR, &error, &length) != 0) {
    throw RuntimeError("cannot check WM connection");
  }

  if (error != 0) {
    throw RuntimeError("cannot connect to WM: " +
                       std::string(std::strerror(error)));
  }
}

void send_all(int fd, const std::string &bytes, Clock::time_point deadline) {
  std::size_t sent = 0;

  while (sent < bytes.size()) {
    wait_for_socket(fd, POLLOUT, deadline);

    const ssize_t count = ::send(fd, bytes.data() + sent, bytes.size() - sent,
                                 MSG_NOSIGNAL | MSG_DONTWAIT);

    if (count > 0) {
      sent += static_cast<std::size_t>(count);
      continue;
    }

    if (count < 0 &&
        (errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK)) {
      continue;
    }

    throw RuntimeError("failed to send WM request");
  }
}

std::string receive_line(int fd, Clock::time_point deadline) {
  std::string response;
  char buffer[1024];

  while (true) {
    wait_for_socket(fd, POLLIN, deadline);

    const ssize_t count = ::recv(fd, buffer, sizeof(buffer), MSG_DONTWAIT);

    if (count > 0) {
      response.append(buffer, static_cast<std::size_t>(count));

      const auto newline = response.find('\n');

      if (newline != std::string::npos) {
        if (newline > kMaxResponse) {
          throw RuntimeError("WM response is too large");
        }

        response.resize(newline);
        return response;
      }

      if (response.size() > kMaxResponse) {
        throw RuntimeError("WM response is too large");
      }

      continue;
    }

    if (count == 0) {
      throw RuntimeError("WM closed connection without a complete response");
    }

    if (errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK) {
      continue;
    }

    throw RuntimeError("failed to receive WM response");
  }
}

void send_request_with_deadline(const std::string &socket_path,
                                const nlohmann::json &request) {
  if (socket_path.empty() ||
      socket_path.size() >= sizeof(sockaddr_un::sun_path) ||
      socket_path.find('\0') != std::string::npos) {
    throw RuntimeError("invalid WM socket path");
  }

  if (!request.is_object() || !request.contains("command") ||
      !request["command"].is_string()) {
    throw RuntimeError("invalid WM request");
  }

  const auto deadline = Clock::now() + kWmTimeout;

  SocketCloser socket{::socket(AF_UNIX, SOCK_STREAM | SOCK_NONBLOCK, 0)};

  if (socket.fd < 0) {
    throw RuntimeError("cannot create WM socket: " +
                       std::string(std::strerror(errno)));
  }

  connect_to_wm(socket.fd, socket_path, deadline);

  send_all(socket.fd, request.dump() + "\n", deadline);

  const auto response = receive_line(socket.fd, deadline);

  const auto parsed = nlohmann::json::parse(response, nullptr, false);

  if (!parsed.is_object() || !parsed.contains("ok") ||
      !parsed["ok"].is_boolean()) {
    throw RuntimeError("invalid WM response");
  }

  if (!parsed["ok"].get<bool>()) {
    throw RuntimeError("WM rejected " + request["command"].get<std::string>() +
                       ": " + response);
  }
}

} // namespace

void send_window_manager_command(const std::string &socket_path,
                                 const std::string &command,
                                 const std::string &window_id,
                                 const std::string &title, std::int64_t width,
                                 std::int64_t height) {
  nlohmann::json request = {{"command", command}};

  if (!window_id.empty()) {
    request["window_id"] = window_id;
  }

  if (!title.empty()) {
    request["title"] = title;
  }

  if (width != 0) {
    request["width"] = width;
  }

  if (height != 0) {
    request["height"] = height;
  }

  send_request_with_deadline(socket_path, request);
}

void send_window_manager_request(const std::string &socket_path,
                                 const nlohmann::json &request) {
  send_request_with_deadline(socket_path, request);
}

} // namespace tacos::runtime::detail
