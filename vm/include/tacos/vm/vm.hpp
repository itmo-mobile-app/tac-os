#pragma once

#include "tacos/runtime/runtime.hpp"

#include <cstdint>
#include <stdexcept>
#include <string>
#include <utility>

namespace tacos::vm {

class VmError : public std::runtime_error {
public:
    explicit VmError(const std::string& message)
        : std::runtime_error(message) {}
};

// Float is VM-owned. The Runtime boundary retains its existing Int/String ABI.
enum class ValueKind { Int, String, Float };

struct Value {
    ValueKind kind = ValueKind::Int;
    union {
        std::int64_t int_value = 0;
        double float_value;
    };
    std::string string_value;

    static Value int_value_of(std::int64_t number) {
        Value value;
        value.int_value = number;
        return value;
    }
    static Value string_value_of(std::string text) {
        Value value;
        value.kind = ValueKind::String;
        value.string_value = std::move(text);
        return value;
    }
    static Value float_value_of(double number) {
        Value value;
        value.kind = ValueKind::Float;
        value.float_value = number;
        return value;
    }
    static Value from_runtime(const tacos::runtime::Value& value) {
        switch (value.kind) {
        case tacos::runtime::ValueKind::Int: return int_value_of(value.int_value);
        case tacos::runtime::ValueKind::String: return string_value_of(value.string_value);
        }
        throw VmError("unsupported Runtime result kind");
    }
    tacos::runtime::Value to_runtime() const {
        switch (kind) {
        case ValueKind::Int: return tacos::runtime::Value::int_value_of(int_value);
        case ValueKind::String: return tacos::runtime::Value::string_value_of(string_value);
        case ValueKind::Float: throw VmError("Runtime Float arguments are not supported");
        }
        throw VmError("unsupported VM value kind");
    }
};

// One isolated execution per call. Concurrent calls require distinct Runtime
// instances, output streams and external resources. See spec/vm-execution.md.
Value run_file(const std::string& path, runtime::Runtime& runtime);

} // namespace tacos::vm
