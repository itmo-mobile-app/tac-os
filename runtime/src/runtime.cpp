#include "tacos/runtime/runtime.hpp"

#include <iostream>
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

Runtime::Runtime(std::ostream& output)
    : output_(output) {}

bool Runtime::has_function(const std::string& name, std::uint8_t arg_count) const {
    return name == "runtime.log" && arg_count == 1;
}

Value Runtime::call(const std::string& name, const std::vector<Value>& args) {
    if (name == "runtime.log" && args.size() == 1) {
        if (args[0].kind != ValueKind::String) {
            throw RuntimeError("runtime.log expects a String argument");
        }
        output_ << args[0].string_value << '\n';
        return Value::int_value_of(0);
    }

    throw RuntimeError("unknown Runtime function: " + name);
}

} // namespace tacos::runtime
