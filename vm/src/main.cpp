#include "tacos/vm/vm.hpp"

#include <exception>
#include <iostream>

int main(int argc, char** argv) {
    if (argc != 2) {
        std::cerr << "usage: " << argv[0] << " <program.bc>\n";
        return 2;
    }

    try {
        tacos::runtime::Runtime runtime(std::cout);
        (void)tacos::vm::run_file(argv[1], runtime);
        return 0;
    } catch (const std::exception& error) {
        std::cerr << "tacvm: " << error.what() << '\n';
        return 1;
    }
}
