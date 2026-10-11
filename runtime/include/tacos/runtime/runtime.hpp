#pragma once

#include <cstdint>
#include <iosfwd>
#include <stdexcept>
#include <string>
#include <vector>

namespace tacos::runtime {

enum class ValueKind {
    Int,
    String,
};

struct Value {
    ValueKind kind;
    std::int64_t int_value = 0;
    std::string string_value;

    static Value int_value_of(std::int64_t value);
    static Value string_value_of(std::string value);
};

class RuntimeError : public std::runtime_error {
public:
    explicit RuntimeError(const std::string& message);
};

class Runtime {
public:
    explicit Runtime(std::ostream& output);

    bool has_function(const std::string& name, std::uint8_t arg_count) const;
    Value call(const std::string& name, const std::vector<Value>& args);

private:
    std::ostream& output_;
};

} // namespace tacos::runtime
