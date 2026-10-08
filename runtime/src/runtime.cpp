#include "tacos/runtime/runtime.hpp"

#include <ostream>
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
