#include "tacos/runtime/runtime.hpp"
#include "tacos/vm/vm.hpp"

#include <exception>
#include <iostream>

int main(int argc, char **argv) {
    if (argc != 4) {
        std::cerr << "usage: " << argv[0]
                  << " <program.bc> <wm-command-socket> <wm-event-socket>\n";
        return 2;
    }

    try {
        tacos::runtime::Runtime runtime(std::cout, argv[2]);
        runtime.set_wm_event_socket_path(argv[3]);
        (void)tacos::vm::run_file(argv[1], runtime);
        std::cout.flush();
        return 0;
    } catch (const std::exception &error) {
        std::cerr << "window_runtime_runner: " << error.what() << '\n';
        return 1;
    }
}
