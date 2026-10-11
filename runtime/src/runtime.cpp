#include "tacos/runtime/runtime.hpp"

#include <cstdlib>
#include <cstring>
#include <ostream>
#include <string>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#include <unordered_map>
#include <utility>

namespace tacos::runtime {

Value Value::int_value_of(std::int64_t value) {
    Value result{};
    result.kind = ValueKind::Int;
    result.int_value = value;
    return result;
}

Value Value::string_value_of(std::string value) {
    Value result{};
    result.kind = ValueKind::String;
    result.string_value = std::move(value);
    return result;
}

RuntimeError::RuntimeError(const std::string& message)
    : std::runtime_error(message) {}

namespace {

// Pointer to a Runtime function implementation.
using Handler = Value (*)(
    std::ostream&,
    const std::vector<Value>&
);

// Runtime function table entry.
struct FunctionEntry {
    std::uint8_t arg_count;
    Handler handler;
};

std::string wm_socket_path() {
    const char* configured = std::getenv("TACOS_WM_SOCKET");
    return configured && *configured ? configured : "/tmp/tac-os-wm.sock";
}

std::string wm_request(const std::string& command) {
    const std::string path = wm_socket_path();
    sockaddr_un address{};
    if (path.size() >= sizeof(address.sun_path)) {
        throw RuntimeError("Window Manager socket path is too long");
    }

    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) {
        throw RuntimeError("cannot create Window Manager socket");
    }

    address.sun_family = AF_UNIX;
    std::strncpy(address.sun_path, path.c_str(), sizeof(address.sun_path) - 1);

    if (connect(fd, reinterpret_cast<sockaddr*>(&address), sizeof(address)) < 0) {
        close(fd);
        throw RuntimeError("cannot connect to Window Manager at " + path);
    }

    const std::string line = command + "\n";
    const ssize_t written = send(fd, line.data(), line.size(), MSG_NOSIGNAL);
    if (written != static_cast<ssize_t>(line.size())) {
        close(fd);
        throw RuntimeError("failed to send command to Window Manager");
    }

    char buffer[2048]{};
    const ssize_t count = recv(fd, buffer, sizeof(buffer) - 1, 0);
    close(fd);
    if (count <= 0) {
        throw RuntimeError("Window Manager returned no response");
    }

    std::string response(buffer, static_cast<std::size_t>(count));
    while (!response.empty() &&
           (response.back() == '\n' || response.back() == '\r')) {
        response.pop_back();
    }
    if (response.rfind("ERROR", 0) == 0) {
        throw RuntimeError(response);
    }
    return response;
}

Value wm_create(std::ostream&, const std::vector<Value>& args) {
    if (args.size() != 3 || args[0].kind != ValueKind::Int ||
        args[1].kind != ValueKind::Int || args[2].kind != ValueKind::String) {
        throw RuntimeError("wm.create expects Int width, Int height, String title");
    }

    const auto width = args[0].int_value;
    const auto height = args[1].int_value;
    const auto& title = args[2].string_value;
    if (width < 1 || width > 4096 || height < 1 || height > 4096 ||
        title.empty() || title.find_first_of("\r\n") != std::string::npos) {
        throw RuntimeError("wm.create received invalid dimensions or title");
    }

    const auto response = wm_request("CREATE " + std::to_string(width) + " " +
                                     std::to_string(height) + " " + title);
    if (response.rfind("OK ", 0) != 0) {
        throw RuntimeError("invalid Window Manager response: " + response);
    }
    try {
        return Value::int_value_of(std::stoll(response.substr(3)));
    } catch (...) {
        throw RuntimeError("invalid window id from Window Manager");
    }
}

Value wm_show(std::ostream&, const std::vector<Value>& args) {
    if (args.size() != 1 || args[0].kind != ValueKind::Int) {
        throw RuntimeError("wm.show expects one Int window id");
    }
    const auto response = wm_request("SHOW " + std::to_string(args[0].int_value));
    if (response != "OK") {
        throw RuntimeError("invalid Window Manager response: " + response);
    }
    return Value::int_value_of(0);
}

Value wm_close(std::ostream&, const std::vector<Value>& args) {
    if (args.size() != 1 || args[0].kind != ValueKind::Int) {
        throw RuntimeError("wm.close expects one Int window id");
    }
    const auto response = wm_request("CLOSE " + std::to_string(args[0].int_value));
    if (response != "OK") {
        throw RuntimeError("invalid Window Manager response: " + response);
    }
    return Value::int_value_of(0);
}

Value wm_poll_event(std::ostream&, const std::vector<Value>& args) {
    if (!args.empty()) {
        throw RuntimeError("wm.poll_event expects no arguments");
    }
    const auto response = wm_request("POLL_EVENT");
    if (response == "NONE") {
        return Value::string_value_of("");
    }
    if (response.rfind("EVENT ", 0) != 0) {
        throw RuntimeError("invalid Window Manager event: " + response);
    }
    return Value::string_value_of(response.substr(6));
}

// runtime.log(message: String)
Value runtime_log(
    std::ostream& output,
    const std::vector<Value>& args
) {
    if (args.size() != 1 ||
        args[0].kind != ValueKind::String) {
        throw RuntimeError(
            "runtime.log expects one String argument"
        );
    }

    output << args[0].string_value << '\n';
    return Value::int_value_of(0);
}

// Runtime functions by `<namespace>.<function>` name.
const std::unordered_map<std::string, FunctionEntry>&
function_table() {
    static const std::unordered_map<
        std::string, FunctionEntry> table = {
        {"runtime.log", {1, &runtime_log}},
        {"wm.create", {3, &wm_create}},
        {"wm.show", {1, &wm_show}},
        {"wm.close", {1, &wm_close}},
        {"wm.poll_event", {0, &wm_poll_event}},
    };

    return table;
}

} // namespace

Runtime::Runtime(std::ostream& output)
    : output_(output) {}

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
            "unknown Runtime function: " + name
        );
    }

    if (args.size() != it->second.arg_count) {
        throw RuntimeError(
            "wrong argument count for Runtime function: "
            + name
        );
    }

    return it->second.handler(output_, args);
}

} // namespace tacos::runtime
