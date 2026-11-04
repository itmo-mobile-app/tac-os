#include "tacos/vm/vm.hpp"

#include <array>
#include <cfenv>
#include <cmath>
#include <exception>
#include <future>
#include <iostream>
#include <sstream>
#include <stdexcept>
#include <string>
#include <thread>

namespace {
struct Result {
    tacos::vm::Value value = tacos::vm::Value::int_value_of(0);
    std::string output;
    std::exception_ptr error;
    int rounding_before = -1;
    int rounding_after = -1;
};

std::array<Result, 2> execute(const std::string& first, const std::string& second) {
    std::array<Result, 2> results;
    std::promise<void> start;
    auto gate = start.get_future().share();
    auto worker = [&](std::size_t index, const std::string& path) {
        std::ostringstream output;
        // Runtime, output, VM state and result storage have distinct owners.
        tacos::runtime::Runtime runtime(output);
        results[index].rounding_before = std::fegetround();
        gate.wait();
        try {
            results[index].value = tacos::vm::run_file(path, runtime);
        } catch (...) {
            results[index].error = std::current_exception();
        }
        results[index].rounding_after = std::fegetround();
        results[index].output = output.str();
    };
    std::thread left(worker, 0, first);
    std::thread right;
    try {
        right = std::thread(worker, 1, second);
    } catch (...) {
        start.set_value();
        left.join();
        throw;
    }
    start.set_value();
    left.join();
    right.join();
    return results;
}

void expect(const Result& result, std::int64_t value, const std::string& output) {
    if (result.error) std::rethrow_exception(result.error);
    if (result.rounding_before != result.rounding_after) {
        throw std::runtime_error("VM leaked a thread rounding mode");
    }
    if (result.value.kind != tacos::vm::ValueKind::Int ||
        result.value.int_value != value || result.output != output) {
        throw std::runtime_error("execution contexts mixed state or output");
    }
}
} // namespace

int main(int argc, char** argv) {
    if (argc != 6) return 2;
    try {
        for (int repeat = 0; repeat < 12; ++repeat) {
            const auto results = execute(argv[1], argv[2]);
            expect(results[0], 10042, "first\n");
            expect(results[1], 20099, "second\n");
        }
        const auto results = execute(argv[3], argv[2]);
        expect(results[1], 20099, "second\n");
        if (!results[0].error) throw std::runtime_error("VM failure was lost");
        try {
            std::rethrow_exception(results[0].error);
        } catch (const tacos::vm::VmError& error) {
            if (std::string(error.what()).find("division by zero") == std::string::npos) throw;
        }
        // A later execution on the same host is unaffected by the failed context.
        std::ostringstream output;
        tacos::runtime::Runtime runtime(output);
        const auto value = tacos::vm::run_file(argv[1], runtime);
        if (value.int_value != 10042 || output.str() != "first\n") return 1;
        const int original_rounding = std::fegetround();
        if (std::fesetround(FE_UPWARD) != 0) throw std::runtime_error("cannot set test rounding");
        const auto floats = execute(argv[4], argv[5]);
        if (std::fegetround() != FE_UPWARD) throw std::runtime_error("host rounding changed");
        for (const auto& result : floats) {
            if (result.error) std::rethrow_exception(result.error);
            if (result.rounding_before != result.rounding_after ||
                result.value.kind != tacos::vm::ValueKind::Float || !result.output.empty()) {
                throw std::runtime_error("Float context state or rounding was lost");
            }
        }
        if (floats[0].value.float_value != 1.0 ||
            floats[1].value.float_value != 0.0 || !std::signbit(floats[1].value.float_value)) {
            throw std::runtime_error("Float rounding or signed zero was lost");
        }
        try {
            (void)tacos::vm::run_file(argv[3], runtime);
            throw std::runtime_error("expected failing execution");
        } catch (const tacos::vm::VmError&) {
            if (std::fegetround() != FE_UPWARD) throw std::runtime_error("error leaked rounding");
        }
        std::fesetround(original_rounding);
        std::cout << "independent VM contexts passed\n";
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 1;
    }
}
