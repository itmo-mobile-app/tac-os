#include "tacos/runtime/runtime.hpp"
#include "window_manager_client.hpp"

#include <ostream>
#include <unordered_map>
#include <utility>

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

Value window_create(
    Runtime& runtime,
    std::ostream&,
    const std::vector<Value>& args
) {
    if (args[0].kind != ValueKind::String) {
        throw RuntimeError(
            "window.create expects title to be a String"
        );
    }

    if (args[1].kind != ValueKind::Int ||
        args[2].kind != ValueKind::Int) {
        throw RuntimeError(
            "window.create expects width and height to be Int"
        );
    }

    if (args[1].int_value <= 0 ||
        args[2].int_value <= 0) {
        throw RuntimeError(
            "window.create expects positive width and height"
        );
    }

    detail::send_window_manager_command(
        runtime.wm_socket_path(),
        "CREATE",
        "map",
        args[0].string_value,
        args[1].int_value,
        args[2].int_value
    );

    return Value::int_value_of(0);
}

Value window_show(
    Runtime& runtime,
    std::ostream&,
    const std::vector<Value>&
) {
    detail::send_window_manager_command(
        runtime.wm_socket_path(),
        "SHOW",
        "map"
    );

    return Value::int_value_of(0);
}

Value window_close(
    Runtime& runtime,
    std::ostream&,
    const std::vector<Value>&
) {
    detail::send_window_manager_command(
        runtime.wm_socket_path(),
        "CLOSE",
        "map"
    );

    return Value::int_value_of(0);
}

Value window_quit(
    Runtime& runtime,
    std::ostream&,
    const std::vector<Value>&
) {
    detail::send_window_manager_command(
        runtime.wm_socket_path(),
        "QUIT"
    );

    return Value::int_value_of(0);
}

const std::unordered_map<std::string, FunctionEntry>&
function_table() {
    static const std::unordered_map<std::string, FunctionEntry> table = {
        {"runtime.log",   {1, &runtime_log}},
        {"window.create", {3, &window_create}},
        {"window.show",   {0, &window_show}},
        {"window.close",  {0, &window_close}},
        {"window.quit",   {0, &window_quit}},
    };

    return table;
}

} // namespace

Runtime::Runtime(
    std::ostream& output,
    std::string wm_socket_path
)
    : output_(output),
      wm_socket_path_(std::move(wm_socket_path)) {}

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

const std::string& Runtime::wm_socket_path() const {
    return wm_socket_path_;
}

} // namespace tacos::runtime
