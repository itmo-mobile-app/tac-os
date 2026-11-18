#include "tacos/vm/vm.hpp"

#include <cmath>
#include <cstdint>
#include <exception>
#include <iostream>
#include <limits>
#include <stdexcept>
#include <string>
#include <thread>
#include <vector>

namespace {
using tacos::vm::Value;
using tacos::vm::ValueKind;

void require(bool ok, const char* message) {
    if (!ok) throw std::runtime_error(message);
}

template<class Error, class Action>
void rejects(Action action) {
    try {
        action();
    } catch (const Error&) {
        return;
    }
    throw std::runtime_error("invalid boundary value was accepted");
}

void numbers() {
    for (double number : {0.0, -0.0, 1.5, -12.25,
                          std::numeric_limits<double>::denorm_min(),
                          std::numeric_limits<double>::max()}) {
        const auto runtime = Value::float_value_of(number).to_runtime();
        require(runtime.kind == tacos::runtime::ValueKind::Float, "Float kind lost in Runtime");
        const auto result = Value::from_runtime(runtime);
        require(result.kind == ValueKind::Float && result.float_value == number,
                "Float payload lost in VM");
        require(std::signbit(result.float_value) == std::signbit(number), "Float sign lost");
    }
    for (std::int64_t number : {std::numeric_limits<std::int64_t>::min(),
                                std::int64_t{9007199254740993LL},
                                std::numeric_limits<std::int64_t>::max()}) {
        const auto value = Value::from_runtime(Value::int_value_of(number).to_runtime());
        require(value.kind == ValueKind::Int && value.int_value == number, "i64 rounded through Float");
    }
    for (double number : {std::numeric_limits<double>::infinity(),
                          -std::numeric_limits<double>::infinity(),
                          std::numeric_limits<double>::quiet_NaN()}) {
        rejects<tacos::runtime::RuntimeError>([&] {
            (void)tacos::runtime::Value::float_value_of(number);
        });
        rejects<tacos::vm::VmError>([&] { (void)Value::float_value_of(number).to_runtime(); });
        // Protect against a buggy handler bypassing the factory.
        tacos::runtime::Value forged = tacos::runtime::Value::int_value_of(0);
        forged.kind = tacos::runtime::ValueKind::Float;
        forged.float_value = number;
        rejects<tacos::vm::VmError>([&] { (void)Value::from_runtime(forged); });
    }
    tacos::runtime::Value invalid = tacos::runtime::Value::int_value_of(0);
    invalid.kind = static_cast<tacos::runtime::ValueKind>(255);
    rejects<tacos::vm::VmError>([&] { (void)Value::from_runtime(invalid); });
    Value invalid_vm;
    invalid_vm.kind = static_cast<ValueKind>(255);
    rejects<tacos::vm::VmError>([&] { (void)invalid_vm.to_runtime(); });
}

void strings(const std::string& marker) {
    // Include heap-backed text, embedded NUL, escapes, UTF-8, and the proposed
    // 64 KiB boundary. This tests ownership, not unapproved helper size limits.
    for (const auto size : {std::size_t{0}, std::size_t{64}, std::size_t{65536}}) {
        const std::string expected = marker + std::string(size, 'x') + std::string("\0\n\"\\", 4) + "цель";
        Value vm;
        {
            auto runtime = tacos::runtime::Value::string_value_of(expected);
            vm = Value::from_runtime(runtime);
            runtime.string_value.assign("overwritten");
        }
        require(vm.string_value == expected, "VM borrowed Runtime string storage");
        auto runtime = vm.to_runtime();
        vm.string_value.clear();
        require(runtime.string_value == expected, "Runtime borrowed VM string storage");
    }
    // Repeated replacement releases previous owners. Sanitizers check lifetimes
    // while String results cross argument/return-style vectors and frame scopes.
    Value local;
    for (int i = 0; i < 30000; ++i) {
        const std::string expected = marker + std::to_string(i) + std::string(256, 's');
        auto runtime = tacos::runtime::Value::string_value_of(expected);
        std::vector<Value> args{Value::from_runtime(runtime)};
        local = std::move(args.front());
        const auto returned = local.to_runtime();
        require(returned.string_value == expected, "generated String lost during replacement");
    }
}
} // namespace

int main() {
    try {
        numbers();
        std::exception_ptr first_error, second_error;
        std::thread first([&] { try { strings("first:"); } catch (...) { first_error = std::current_exception(); } });
        std::thread second;
        try {
            second = std::thread([&] { try { strings("second:"); } catch (...) { second_error = std::current_exception(); } });
        } catch (...) {
            first.join();
            throw;
        }
        first.join();
        second.join();
        if (first_error) std::rethrow_exception(first_error);
        if (second_error) std::rethrow_exception(second_error);
        std::cout << "typed values and owned Strings passed\n";
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 1;
    }
}
