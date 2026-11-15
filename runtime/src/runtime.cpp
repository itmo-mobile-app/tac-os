#include "tacos/runtime/runtime.hpp"
#include "window_manager_client.hpp"
#include "camera_detector_client.hpp"

#include <ostream>
#include <iostream>
#include <unordered_map>
#include <utility>
#include <cerrno>
#include <cstring>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>
#include <unistd.h>
#include <chrono>
#include <poll.h>
#include <algorithm>
#include <cmath>
#include <cstdint>
#include <limits>
#include <unordered_set>
#include <nlohmann/json.hpp>


namespace tacos::runtime {

Value Value::int_value_of(std::int64_t value) {
    Value result;
    result.kind = ValueKind::Int;
    result.int_value = value;
    return result;
}

Value Value::string_value_of(std::string value) {
    Value result;
    result.kind = ValueKind::String;
    result.string_value = std::move(value);
    return result;
}

RuntimeError::RuntimeError(const std::string& message)
    : std::runtime_error(message) {}

namespace {


nlohmann::json read_wm_event_json(int fd, std::string &buffer, int timeout_ms) {
  const auto deadline =
      std::chrono::steady_clock::now() + std::chrono::milliseconds(timeout_ms);

  while (true) {
    const auto newline = buffer.find('\n');

    if (newline != std::string::npos) {
      std::string line = buffer.substr(0, newline);
      buffer.erase(0, newline + 1);

      try {
        return nlohmann::json::parse(line);
      } catch (const nlohmann::json::exception &) {
        throw RuntimeError("invalid WM event JSON");
      }
    }

    const auto now = std::chrono::steady_clock::now();
    if (timeout_ms > 0 && now >= deadline) {
      return nullptr;
    }

    const auto remaining =
        std::chrono::duration_cast<std::chrono::milliseconds>(deadline - now)
            .count();

    pollfd pfd{};
    pfd.fd = fd;
    pfd.events = POLLIN;

    const int wait_ms =
        timeout_ms == 0
            ? 0
            : static_cast<int>(std::max<std::int64_t>(1, remaining));

    const int result = ::poll(&pfd, 1, wait_ms);

    if (result == 0) {
      return nullptr;
    }

    if (result < 0) {
      if (errno == EINTR) {
        continue;
      }
      throw RuntimeError("WM event poll failed: " +
                         std::string(std::strerror(errno)));
    }

    char chunk[4096];
    const ssize_t count = ::recv(fd, chunk, sizeof(chunk), MSG_DONTWAIT);

    if (count > 0) {
      buffer.append(chunk, static_cast<std::size_t>(count));

      if (buffer.size() > 65536) {
        throw RuntimeError("WM event message too large");
      }
    } else if (count == 0) {
      throw RuntimeError("WM event socket disconnected");
    } else if (errno != EINTR && errno != EAGAIN && errno != EWOULDBLOCK) {
      throw RuntimeError("WM event receive failed: " +
                         std::string(std::strerror(errno)));
    }
  }
}

void validate_observation(const std::string& text) {
    using json = nlohmann::json;

    json message;
    bool duplicate_key = false;
    std::unordered_set<std::string> keys;

    try {
        message = json::parse(
            text,
            [&](int, json::parse_event_t event, json& value) {
                if (event == json::parse_event_t::key) {
                    if (!keys.insert(
                            value.get<std::string>()
                        ).second) {
                        duplicate_key = true;
                    }
                }
                return true;
            }
        );
    } catch (const json::exception&) {
        throw RuntimeError("IPC invalid JSON");
    }

    if (!message.is_object() ||
        duplicate_key ||
        message.size() != 7) {
        throw RuntimeError("IPC invalid Observation fields");
    }

    const char* required[] = {
        "type",
        "targetId",
        "observerId",
        "latitude",
        "longitude",
        "confidence",
        "timestamp"
    };

    for (const char* field : required) {
        if (!message.contains(field)) {
            throw RuntimeError(
                std::string("IPC missing field: ") + field
            );
        }
    }

    if (!message["type"].is_string() ||
        message["type"] != "OBSERVATION") {
        throw RuntimeError("IPC invalid message type");
    }

    if (!message["targetId"].is_string()) {
        throw RuntimeError("IPC invalid targetId");
    }

    const auto target_id =
        message["targetId"].get<std::string>();

    if (target_id.empty() ||
        !std::all_of(
            target_id.begin(),
            target_id.end(),
            [](unsigned char c) {
                return c >= '0' && c <= '9';
            }
        )) {
        throw RuntimeError("IPC invalid targetId");
    }

    if (!message["observerId"].is_string() ||
        message["observerId"].get<std::string>().empty()) {
        throw RuntimeError("IPC invalid observerId");
    }

    auto valid_number = [&](const char* field,
                            double minimum,
                            double maximum) {
        const auto& value = message[field];

        if (!value.is_number()) {
            return false;
        }

        double number = value.get<double>();

        return std::isfinite(number) &&
               number >= minimum &&
               number <= maximum;
    };

    if (!valid_number("latitude", -90.0, 90.0)) {
        throw RuntimeError("IPC invalid latitude");
    }

    if (!valid_number("longitude", -180.0, 180.0)) {
        throw RuntimeError("IPC invalid longitude");
    }

    if (!valid_number("confidence", 0.0, 1.0)) {
        throw RuntimeError("IPC invalid confidence");
    }

    const auto& timestamp = message["timestamp"];

    if (!timestamp.is_number_integer() ||
        (timestamp.is_number_unsigned() &&
         timestamp.get<std::uint64_t>() >
             static_cast<std::uint64_t>(
                 std::numeric_limits<std::int64_t>::max()
             ))) {
        throw RuntimeError("IPC invalid timestamp");
    }
}


using IpcDeadline =
    std::chrono::steady_clock::time_point;

void wait_ipc_writable(int fd, IpcDeadline deadline) {
    while (true) {
        const auto remaining =
            std::chrono::duration_cast<
                std::chrono::milliseconds
            >(deadline - std::chrono::steady_clock::now());

        if (remaining.count() <= 0) {
            throw RuntimeError("IPC send timeout");
        }

        pollfd pfd{};
        pfd.fd = fd;
        pfd.events = POLLOUT;

        const int result = ::poll(
            &pfd,
            1,
            static_cast<int>(remaining.count())
        );

        if (result > 0) {
            if (pfd.revents & POLLNVAL) {
                throw RuntimeError("Invalid IPC socket");
            }
            return;
        }

        if (result == 0) {
            throw RuntimeError("IPC send timeout");
        }

        if (errno != EINTR) {
            throw RuntimeError(
                std::string("IPC poll failed: ") +
                std::strerror(errno)
            );
        }
    }
}


using Handler = Value (*)(
    Runtime&,
    std::ostream&,
    const std::vector<Value>&
);

struct FunctionEntry {
    std::uint8_t arg_count;
    Handler handler;
};

Value runtime_log(
    Runtime&,
    std::ostream& output,
    const std::vector<Value>& args
) {
    if (args[0].kind != ValueKind::String) {
        throw RuntimeError(
            "runtime.log expects a String argument"
        );
    }

    output << args[0].string_value << '\n';
    return Value::int_value_of(0);
}

Value window_create(Runtime &runtime, std::ostream &,
                    const std::vector<Value> &args) {
  if (args[0].kind != ValueKind::String) {
    throw RuntimeError("window.create expects title to be a String");
  }

  if (args[1].kind != ValueKind::Int || args[2].kind != ValueKind::Int) {
    throw RuntimeError("window.create expects width and height to be Int");
  }

  if (args[1].int_value <= 0 || args[2].int_value <= 0) {
    throw RuntimeError("window.create expects positive width and height");
  }
  runtime.connect_wm_events();
  detail::send_window_manager_command(runtime.wm_socket_path(), "CREATE", "map",
                                      args[0].string_value, args[1].int_value,
                                      args[2].int_value);
  runtime.mark_wm_window_open();
  return Value::int_value_of(0);
}

Value window_show(Runtime &runtime, std::ostream &,
                  const std::vector<Value> &) {
  detail::send_window_manager_command(runtime.wm_socket_path(), "SHOW", "map");

  return Value::int_value_of(0);
}

Value window_close(Runtime &runtime, std::ostream &,
                   const std::vector<Value> &) {
  if (runtime.is_wm_window_open()) {
    detail::send_window_manager_command(runtime.wm_socket_path(), "CLOSE",
                                        "map");
  }

  runtime.disconnect_wm_events();

  return Value::int_value_of(0);
}

Value window_quit(Runtime &runtime, std::ostream &,
                  const std::vector<Value> &) {
  detail::send_window_manager_command(runtime.wm_socket_path(), "QUIT");

  runtime.disconnect_wm_events();

  return Value::int_value_of(0);
}

Value window_begin_frame(Runtime &runtime, std::ostream &,
                         const std::vector<Value> &args) {
  for (const auto &arg : args) {
    if (arg.kind != ValueKind::Int || arg.int_value < 0 ||
        arg.int_value > 255) {
      throw RuntimeError(
          "window.beginFrame expects four RGBA integers in [0, 255]");
    }
  }

  detail::send_window_manager_request(runtime.wm_socket_path(),
                                      {{"command", "BEGIN_FRAME"},
                                       {"window_id", "map"},
                                       {"color",
                                        {{"r", args[0].int_value},
                                         {"g", args[1].int_value},
                                         {"b", args[2].int_value},
                                         {"a", args[3].int_value}}}});

  return Value::int_value_of(0);
}

Value window_draw_image(Runtime &runtime, std::ostream &,
                        const std::vector<Value> &args) {
  if (args[0].kind != ValueKind::String) {
    throw RuntimeError("window.drawImage expects an image path String");
  }

  for (std::size_t i = 1; i < args.size(); ++i) {
    if (args[i].kind != ValueKind::Int) {
      throw RuntimeError(
          "window.drawImage expects integer coordinates and dimensions");
    }
  }

  if (args[0].string_value.empty() || args[3].int_value <= 0 ||
      args[4].int_value <= 0) {
    throw RuntimeError(
        "window.drawImage expects a nonempty path and positive dimensions");
  }

  detail::send_window_manager_request(runtime.wm_socket_path(),
                                      {{"command", "DRAW_IMAGE"},
                                       {"window_id", "map"},
                                       {"image_path", args[0].string_value},
                                       {"x", args[1].int_value},
                                       {"y", args[2].int_value},
                                       {"width", args[3].int_value},
                                       {"height", args[4].int_value}});

  return Value::int_value_of(0);
}

Value window_draw_circle(Runtime &runtime, std::ostream &,
                         const std::vector<Value> &args) {
  for (const auto &arg : args) {
    if (arg.kind != ValueKind::Int) {
      throw RuntimeError("window.drawCircle expects seven Int arguments");
    }
  }

  if (args[2].int_value <= 0) {
    throw RuntimeError("window.drawCircle expects a positive radius");
  }

  for (std::size_t i = 3; i < 7; ++i) {
    if (args[i].int_value < 0 || args[i].int_value > 255) {
      throw RuntimeError("window.drawCircle expects RGBA values in [0, 255]");
    }
  }

  detail::send_window_manager_request(runtime.wm_socket_path(),
                                      {{"command", "DRAW_CIRCLE"},
                                       {"window_id", "map"},
                                       {"x", args[0].int_value},
                                       {"y", args[1].int_value},
                                       {"radius", args[2].int_value},
                                       {"color",
                                        {{"r", args[3].int_value},
                                         {"g", args[4].int_value},
                                         {"b", args[5].int_value},
                                         {"a", args[6].int_value}}}});

  return Value::int_value_of(0);
}

Value window_present(Runtime &runtime, std::ostream &,
                     const std::vector<Value> &) {
  detail::send_window_manager_request(
      runtime.wm_socket_path(), {{"command", "PRESENT"}, {"window_id", "map"}});

  return Value::int_value_of(0);
}
Value window_poll_event(Runtime &runtime, std::ostream &,
                        const std::vector<Value> &args) {
  if (args[0].kind != ValueKind::Int || args[0].int_value < 0 ||
      args[0].int_value > 1000) {
    throw RuntimeError("window.pollEvent expects timeout in [0, 1000] ms");
  }

  return Value::string_value_of(
      runtime.window_poll_event(static_cast<int>(args[0].int_value)));
}

Value camera_detect_markers_handler(
    Runtime& runtime,
    std::ostream&,
    const std::vector<Value>&
) {
    return Value::string_value_of(runtime.camera_detect_markers());
}

Value ipc_listen_handler(
    Runtime& runtime,
    std::ostream&,
    const std::vector<Value>&
) {
    runtime.ipc_listen();
    return Value::int_value_of(0);
}

Value ipc_receive_handler(
    Runtime& runtime,
    std::ostream&,
    const std::vector<Value>&
) {
    return Value::string_value_of(
        runtime.ipc_receive()
    );
}

Value ipc_send_handler(
    Runtime& runtime,
    std::ostream&,
    const std::vector<Value>& args
) {
    if (args[0].kind != ValueKind::String) {
        throw RuntimeError(
            "runtime.ipc_send expects a String argument"
        );
    }

    try {
        runtime.ipc_send(args[0].string_value);
    } catch (const RuntimeError& error) {
        // Sending is best-effort: .tc can inspect the status and keep running.
        std::cerr << "runtime.ipc_send: " << error.what() << '\n';
        return Value::int_value_of(-1);
    }
    return Value::int_value_of(0);
}


const std::unordered_map<std::string, FunctionEntry> &function_table() {

  static const std::unordered_map<std::string, FunctionEntry> table = {
      {"runtime.log", {1, &runtime_log}},
      {"camera.detectMarkers", {0, &camera_detect_markers_handler}},
      {"window.create", {3, &window_create}},
      {"window.beginFrame", {4, &window_begin_frame}},
      {"window.drawImage", {5, &window_draw_image}},
      {"window.drawCircle", {7, &window_draw_circle}},
      {"window.present", {0, &window_present}},
      {"window.pollEvent", {1, &window_poll_event}},
      {"window.show", {0, &window_show}},
      {"window.close", {0, &window_close}},
      {"window.quit", {0, &window_quit}},

      {"runtime.ipc_listen", {0, &ipc_listen_handler}},
      {"runtime.ipc_receive", {0, &ipc_receive_handler}},
      {"runtime.ipc_send", {1, &ipc_send_handler}},
  };

  return table;
}

} // namespace

Runtime::Runtime(
    std::ostream& output,
    std::string wm_socket_path,
    std::string ipc_socket_path
)
    : Runtime(output, std::move(wm_socket_path), std::move(ipc_socket_path), CameraOptions{}) {}

Runtime::Runtime(
    std::ostream& output,
    std::string wm_socket_path,
    std::string ipc_socket_path,
    CameraOptions camera_options
)
    : output_(output),
      wm_socket_path_(std::move(wm_socket_path)),
      ipc_socket_path_(std::move(ipc_socket_path)),
      camera_(std::make_unique<detail::CameraDetectorClient>(std::move(camera_options))) {}


Runtime::~Runtime() {
  if (wm_window_open_) {
    try {
      detail::send_window_manager_command(wm_socket_path_, "CLOSE", "map");
    } catch (...) {
      // Cleanup must not throw if WM is unavailable or already closed.
    }
  }

  disconnect_wm_events();
  camera_.reset();

  if (ipc_connection_fd_ != -1) {
    ::close(ipc_connection_fd_);
  }

  if (ipc_listen_fd_ != -1) {
    ::close(ipc_listen_fd_);
    ::unlink(ipc_socket_path_.c_str());
  }
}

bool Runtime::has_function(
    const std::string& name,
    std::uint8_t arg_count
) const {
    const auto& table = function_table();
    const auto it = table.find(name);

    return it != table.end() &&
           it->second.arg_count == arg_count;
}

Value Runtime::call(
    const std::string& name,
    const std::vector<Value>& args
) {
    const auto& table = function_table();
    const auto it = table.find(name);

    if (it == table.end()) {
        throw RuntimeError(
            "unknown runtime function: " + name
        );
    }

    if (args.size() != it->second.arg_count) {
        throw RuntimeError(
            "wrong argument count for runtime function: " + name
        );
    }

    return it->second.handler(*this, output_, args);
}

const std::string &Runtime::wm_socket_path() const { return wm_socket_path_; }
void Runtime::set_wm_event_socket_path(std::string path) {
  if (wm_event_fd_ != -1) {
    throw RuntimeError("cannot change WM event socket path while connected");
  }

  if (path.empty()) {
    throw RuntimeError("WM event socket path cannot be empty");
  }

  wm_event_socket_path_ = std::move(path);
}

void Runtime::connect_wm_events() {
  if (wm_event_fd_ != -1) {
    return;
  }

  const std::string path = wm_event_socket_path_.empty()
                               ? "/tmp/tacos-wm-events.sock"
                               : wm_event_socket_path_;

  if (path.size() >= sizeof(sockaddr_un::sun_path)) {
    throw RuntimeError("WM event socket path is too long");
  }

  int fd = ::socket(AF_UNIX, SOCK_STREAM | SOCK_NONBLOCK | SOCK_CLOEXEC, 0);
  if (fd < 0) {
    throw RuntimeError("cannot create WM event socket: " +
                       std::string(std::strerror(errno)));
  }

  sockaddr_un addr{};
  addr.sun_family = AF_UNIX;
  std::memcpy(addr.sun_path, path.c_str(), path.size() + 1);

  if (::connect(fd, reinterpret_cast<sockaddr *>(&addr), sizeof(addr)) != 0) {
    const std::string error = std::strerror(errno);
    ::close(fd);
    throw RuntimeError("cannot connect to WM event socket: " + error);
  }

  try {
    const auto response = read_wm_event_json(fd, wm_event_buffer_, 1000);

    if (!response.is_object() || !response.value("ok", false) ||
        response.value("message", std::string{}) != "subscribed") {
      throw RuntimeError("WM event subscription was not confirmed");
    }

    wm_event_fd_ = fd;
  } catch (...) {
    ::close(fd);
    wm_event_buffer_.clear();
    throw;
  }
}

std::string Runtime::window_poll_event(int timeout_ms) {
  if (wm_event_fd_ == -1) {
    throw RuntimeError("WM event socket is not connected");
  }

  if (timeout_ms < 0 || timeout_ms > 1000) {
    throw RuntimeError("invalid WM event timeout");
  }

  const auto event =
      read_wm_event_json(wm_event_fd_, wm_event_buffer_, timeout_ms);

  if (event.is_null()) {
    return "";
  }

  if (!event.is_object()) {
    throw RuntimeError("invalid WM event response");
  }

  const std::string name = event.value("event", std::string{});

  const std::string window_id = event.value("window_id", std::string{});

  if (name == "window_close" && window_id == "map") {
    wm_window_open_ = false;
    return "window_close";
  }

  return "";
}

void Runtime::mark_wm_window_open() { wm_window_open_ = true; }

bool Runtime::is_wm_window_open() const { return wm_window_open_; }

void Runtime::disconnect_wm_events() {
  if (wm_event_fd_ != -1) {
    ::close(wm_event_fd_);
    wm_event_fd_ = -1;
  }

  wm_event_buffer_.clear();
  wm_window_open_ = false;
}

const std::string& Runtime::ipc_socket_path() const {
    return ipc_socket_path_;
}


std::string Runtime::camera_detect_markers() {
    return camera_->detect_markers();
}

void Runtime::ipc_listen() {
    if (ipc_listen_fd_ != -1) {
        throw RuntimeError("IPC listener already started");
    }

    sockaddr_un addr{};
    addr.sun_family = AF_UNIX;

    if (ipc_socket_path_.empty() ||
        ipc_socket_path_.size() >= sizeof(addr.sun_path) ||
        ipc_socket_path_.find('\0') != std::string::npos) {
        throw RuntimeError("Invalid IPC socket path");
    }

    std::memcpy(
        addr.sun_path,
        ipc_socket_path_.c_str(),
        ipc_socket_path_.size() + 1
    );

    // Проверяем, существует ли уже socket-файл.
    struct stat info{};
    if (::lstat(ipc_socket_path_.c_str(), &info) == 0) {
        if (!S_ISSOCK(info.st_mode)) {
            throw RuntimeError(
                "IPC path exists and is not a socket"
            );
        }

        // Не удаляем сокет работающего Map.
        int probe = ::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC | SOCK_NONBLOCK, 0);
        if (probe == -1) {
            throw RuntimeError("IPC probe socket failed");
        }

        int result = ::connect(
            probe,
            reinterpret_cast<sockaddr*>(&addr),
            sizeof(addr)
        );
        int connect_error = errno;
        ::close(probe);

        if (result == 0 || connect_error != ECONNREFUSED) {
            throw RuntimeError(
                "IPC socket is already in use"
            );
        }

        // Старый сокет остался после завершения Map.
        if (::unlink(ipc_socket_path_.c_str()) == -1) {
            throw RuntimeError(
                "Cannot remove stale IPC socket"
            );
        }
    } else if (errno != ENOENT) {
        throw RuntimeError("Cannot inspect IPC socket path");
    }

    int fd = ::socket(
        AF_UNIX,
        SOCK_STREAM | SOCK_CLOEXEC,
        0
    );

    if (fd == -1) {
        throw RuntimeError(
            std::string("IPC socket failed: ") +
            std::strerror(errno)
        );
    }

    if (::bind(
            fd,
            reinterpret_cast<sockaddr*>(&addr),
            sizeof(addr)
        ) == -1) {
        int error = errno;
        ::close(fd);

        throw RuntimeError(
            std::string("IPC bind failed: ") +
            std::strerror(error)
        );
    }

    if (::listen(fd, 1) == -1) {
        int error = errno;
        ::close(fd);
        ::unlink(ipc_socket_path_.c_str());

        throw RuntimeError(
            std::string("IPC listen failed: ") +
            std::strerror(error)
        );
    }

    ipc_listen_fd_ = fd;
}


void Runtime::ipc_send(const std::string& json) {
    // Одно сообщение = одна строка JSON.
    if (json.empty() ||
        json.find('\n') != std::string::npos ||
        json.find('\r') != std::string::npos ||
        json.find('\0') != std::string::npos) {
        throw RuntimeError("Invalid IPC message framing");
    }

    sockaddr_un addr{};
    addr.sun_family = AF_UNIX;

    if (ipc_socket_path_.empty() ||
        ipc_socket_path_.size() >= sizeof(addr.sun_path) ||
        ipc_socket_path_.find('\0') != std::string::npos) {
        throw RuntimeError("Invalid IPC socket path");
    }

    std::memcpy(
        addr.sun_path,
        ipc_socket_path_.c_str(),
        ipc_socket_path_.size() + 1
    );

    // Общее ограничение на подключение и отправку.
    const auto deadline =
        std::chrono::steady_clock::now() +
        std::chrono::milliseconds(1000);

    // Подключаемся только при первом вызове.
    if (ipc_connection_fd_ == -1) {
        int fd = ::socket(
            AF_UNIX,
            SOCK_STREAM | SOCK_CLOEXEC | SOCK_NONBLOCK,
            0
        );

        if (fd == -1) {
            throw RuntimeError(
                std::string("IPC socket failed: ") +
                std::strerror(errno)
            );
        }

        try {
            int result = ::connect(
                fd,
                reinterpret_cast<sockaddr*>(&addr),
                sizeof(addr)
            );

            if (result == -1) {
                if (errno != EINPROGRESS) {
                    throw RuntimeError(
                        std::string("IPC Map unavailable: ") +
                        std::strerror(errno)
                    );
                }

                wait_ipc_writable(fd, deadline);

                int socket_error = 0;
                socklen_t error_size = sizeof(socket_error);

                if (::getsockopt(
                        fd, SOL_SOCKET, SO_ERROR,
                        &socket_error, &error_size
                    ) == -1) {
                    throw RuntimeError(
                        "IPC connection check failed"
                    );
                }

                if (socket_error != 0) {
                    throw RuntimeError(
                        std::string("IPC Map unavailable: ") +
                        std::strerror(socket_error)
                    );
                }
            }

            ipc_connection_fd_ = fd;
        } catch (...) {
            ::close(fd);
            throw;
        }
    }

    // Добавляем разделитель сообщений.
    const std::string message = json + '\n';
    std::size_t sent = 0;

    try {
        while (sent < message.size()) {
            ssize_t count = ::send(
                ipc_connection_fd_,
                message.data() + sent,
                message.size() - sent,
                MSG_NOSIGNAL
            );

            if (count > 0) {
                sent += static_cast<std::size_t>(count);
                continue;
            }

            if (count == -1 && errno == EINTR) {
                continue;
            }

            if (count == -1 &&
                (errno == EAGAIN || errno == EWOULDBLOCK)) {
                wait_ipc_writable(
                    ipc_connection_fd_, deadline
                );
                continue;
            }

            throw RuntimeError(
                count == 0
                    ? "IPC connection closed during send"
                    : std::string("IPC send failed: ") +
                      std::strerror(errno)
            );
        }
    } catch (...) {
        ::close(ipc_connection_fd_);
        ipc_connection_fd_ = -1;
        throw;
    }
}

std::string Runtime::ipc_receive() {
    if (ipc_listen_fd_ == -1) {
        throw RuntimeError("IPC listener is not started");
    }

    constexpr std::size_t max_message_size = 64 * 1024;

    auto disconnect = [this]() {
        if (ipc_connection_fd_ != -1) {
            ::close(ipc_connection_fd_);
            ipc_connection_fd_ = -1;
        }
        ipc_buffer_.clear();
    };

    while (true) {
        // Проверяем, есть ли готовое сообщение в буфере.
        auto pos = ipc_buffer_.find('\n');

        if (pos != std::string::npos) {
            if (pos > max_message_size) {
                disconnect();
                throw RuntimeError("IPC message too large");
            }

            std::string message = ipc_buffer_.substr(0, pos);
            ipc_buffer_.erase(0, pos + 1);


if (message.empty() ||
    message.find('\r') != std::string::npos ||
    message.find('\0') != std::string::npos) {
    disconnect();
    throw RuntimeError("Invalid IPC message framing");
}

// Проверяем содержимое по spec/ipc.md.
try {
    validate_observation(message);
} catch (const RuntimeError&) {
    disconnect();
    throw;
}

return message;

        }

        if (ipc_buffer_.size() > max_message_size) {
            disconnect();
            throw RuntimeError("IPC message too large");
        }

        // Если соединения нет, ждём Analyzer.
        if (ipc_connection_fd_ == -1) {
            int fd;

            do {
                fd = ::accept4(
                    ipc_listen_fd_,
                    nullptr,
                    nullptr,
                    SOCK_CLOEXEC
                );
            } while (fd == -1 && errno == EINTR);

            if (fd == -1) {
                throw RuntimeError(
                    std::string("IPC accept failed: ") +
                    std::strerror(errno)
                );
            }

            ipc_connection_fd_ = fd;
        }

        // Ждём данные от подключённого Analyzer.
        pollfd pfd{};
        pfd.fd = ipc_connection_fd_;
        pfd.events = POLLIN;

        // Silence between observations is normal. Bound only a partial frame.
        int ready = ::poll(&pfd, 1, ipc_buffer_.empty() ? -1 : 5000);

        if (ready == 0) {
            disconnect();
            throw RuntimeError("IPC receive timeout");
        }

        if (ready == -1) {
            if (errno == EINTR) {
                continue;
            }

            int error = errno;
            disconnect();

            throw RuntimeError(
                std::string("IPC poll failed: ") +
                std::strerror(error)
            );
        }

        char buffer[4096];

        ssize_t count = ::recv(
            ipc_connection_fd_,
            buffer,
            sizeof(buffer),
            MSG_DONTWAIT
        );

        if (count > 0) {
            ipc_buffer_.append(
                buffer,
                static_cast<std::size_t>(count)
            );
            continue;
        }

        if (count == 0) {
            const bool partial_message = !ipc_buffer_.empty();
            disconnect();
            if (partial_message) {
                throw RuntimeError("IPC Analyzer disconnected during a message");
            }
            // Includes the empty connection used to probe an existing listener.
            continue;
        }

        if (errno == EINTR ||
            errno == EAGAIN ||
            errno == EWOULDBLOCK) {
            continue;
        }

        int error = errno;
        disconnect();

        throw RuntimeError(
            std::string("IPC recv failed: ") +
            std::strerror(error)
        );
    }
}


} // namespace tacos::runtime
