// Optional integration runner for the real Runtime device implementation #162.
// Not built on main until DeviceOptions exists; see docs/testing/vm-data-133.md.
#include "tacos/vm/vm.hpp"
#include <iostream>
#include <sstream>
#include <stdexcept>

int main(int argc, char** argv) {
    if (argc != 3) return 2;
    try {
        std::ostringstream output;
        tacos::runtime::DeviceOptions device;
        device.position = tacos::runtime::DevicePosition{59.95, 30.31};
        device.heading = 25.0;
        device.fixed_now_ms = 9007199254740993LL;
        tacos::runtime::Runtime runtime(output, "/tmp/unused-vm133-wm",
            "/tmp/unused-vm133-ipc", tacos::runtime::CameraOptions{}, device);
        const auto floating = tacos::vm::run_file(argv[1], runtime);
        if (floating.kind != tacos::vm::ValueKind::Float || floating.float_value != 84.95) {
            throw std::runtime_error("real Runtime Float was lost through CALL_RUNTIME/locals/arithmetic/RET");
        }
        const auto timestamp = tacos::vm::run_file(argv[2], runtime);
        if (timestamp.kind != tacos::vm::ValueKind::Int || timestamp.int_value != 9007199254740993LL) {
            throw std::runtime_error("real Runtime timestamp was rounded");
        }
        if (!output.str().empty()) throw std::runtime_error("device getter produced unexpected output");
        std::cout << "real Runtime #162 compatibility passed\n";
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 1;
    }
}
