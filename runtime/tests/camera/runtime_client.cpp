#include "tacos/runtime/runtime.hpp"

#include <sys/wait.h>

#include <cerrno>
#include <iostream>
#include <sstream>
#include <stdexcept>
#include <string>

using namespace tacos::runtime;

void no_children() {
    int status;
    if (::waitpid(-1, &status, WNOHANG) != -1 || errno != ECHILD) {
        throw std::runtime_error("Runtime leaked a child process or zombie");
    }
}

int main(int argc, char** argv) {
    try {
        if (argc != 5) throw std::runtime_error("expected helper, fake mode, count, unwind");
        const int count = std::stoi(argv[3]);
        const bool unwind = std::string(argv[4]) == "unwind";
        std::ostringstream output;
        try {
            Runtime runtime(output, "/tmp/tacos-wm.sock", "/tmp/tac-os-analyzer-map.sock",
                            CameraOptions{argv[1], argv[2]});
            if (!runtime.has_function("camera.detectMarkers", 0) ||
                runtime.has_function("camera.detectMarkers", 1)) {
                throw std::runtime_error("invalid camera registry entry");
            }
            no_children(); // Construction and function resolution must be lazy.
            try {
                runtime.call("camera.detectMarkers", {Value::int_value_of(1)});
                throw std::runtime_error("wrong camera arity was accepted");
            } catch (const RuntimeError&) {}
            no_children();
            if (count > 0) std::cin.get(); // Let the harness prepare an occupied pathname.
            for (int i = 0; i < count; ++i) {
                try {
                    const auto result = runtime.call("camera.detectMarkers", {});
                    if (result.kind != ValueKind::String) {
                        throw std::runtime_error("camera did not return a String");
                    }
                    std::cout << "OK " << result.string_value << std::endl;
                } catch (const RuntimeError& error) {
                    std::cout << "ERROR " << error.what() << std::endl;
                }
                if (unwind) throw std::runtime_error("test unwind");
            }
        } catch (const std::runtime_error& error) {
            if (!unwind || std::string(error.what()) != "test unwind") throw;
        }
        no_children();
        if (!output.str().empty()) throw std::runtime_error("camera wrote to runtime.log output");
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 1;
    }
}
