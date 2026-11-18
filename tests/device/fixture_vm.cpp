#include "tacos/vm/vm.hpp"

#include <iostream>
#include <string_view>

int main(int argc, char** argv) {
    if (argc != 7) {
        std::cerr << "usage: device_fixture_vm <program.bc> <lat> <lon> <heading> <now-ms> <delay-ms>\n";
        return 2;
    }
    try {
        using namespace tacos::runtime;
        DeviceOptions device;
        if (std::string_view(argv[2]) != "-") {
            device.position = DevicePosition{std::stod(argv[2]), std::stod(argv[3])};
        }
        if (std::string_view(argv[4]) != "-") {
            device.heading = std::stod(argv[4]);
        }
        device.fixed_now_ms = std::stoll(argv[5]);
        device.position_delay = device.heading_delay = std::chrono::milliseconds(std::stoll(argv[6]));

        Runtime runtime(std::cout, "/tmp/tacos-wm.sock", "/tmp/tac-os-analyzer-map.sock",
                        CameraOptions{}, device);
        (void)tacos::vm::run_file(argv[1], runtime);
        return 0;
    } catch (const std::exception& error) {
        std::cerr << "device_fixture_vm: " << error.what() << '\n';
        return 1;
    }
}
