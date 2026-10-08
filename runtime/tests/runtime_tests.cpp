
#include "tacos/runtime/runtime.hpp"

#include <iostream>
#include <sstream>
#include <stdexcept>
#include <string>

using tacos::runtime::Runtime;
using tacos::runtime::RuntimeError;
using tacos::runtime::Value;
using tacos::runtime::ValueKind;

void check(bool condition, const char* message) {
    if (!condition) {
        throw std::runtime_error(message);
    }
}

template <typename F>
void expect_runtime_error(F operation) {
    try {
        operation();
    } catch (const RuntimeError&) {
        return;
    }
    throw std::runtime_error("expected RuntimeError");
}

int main() {
    try {
        std::ostringstream output;
        Runtime runtime(output);

        // 1. Function table lookup.
        check(
            runtime.has_function("runtime.log", 1),
            "runtime.log missing"
        );

        check(
            !runtime.has_function("runtime.log", 0),
            "wrong arity accepted"
        );

        check(
            !runtime.has_function("runtime.missing", 1),
            "unknown name accepted"
        );

        // 2. runtime.log output.
        const Value result = runtime.call(
            "runtime.log",
            {Value::string_value_of("test message")}
        );

        check(
            output.str() == "test message\n",
            "incorrect output"
        );

        // 3. runtime.log returns Int 0.
        check(
            result.kind == ValueKind::Int &&
            result.int_value == 0,
            "runtime.log must return Int 0"
        );

        // 4. Unknown function.
        expect_runtime_error([&] {
            runtime.call("runtime.missing", {});
        });

        // 5. Wrong argument count.
        expect_runtime_error([&] {
            runtime.call("runtime.log", {});
        });

        // 6. Wrong argument type.
        expect_runtime_error([&] {
            runtime.call(
                "runtime.log",
                {Value::int_value_of(42)}
            );
        });

        check(
            output.str() == "test message\n",
            "invalid calls wrote output"
        );

        std::cout << "Runtime unit tests passed\n";
        return 0;

    } catch (const std::exception& error) {
        std::cerr
            << "Runtime unit tests failed: "
            << error.what() << '\n';
        return 1;
    }
}
