#include <iostream>

// Usage: tacvm <program.bc>
int main(int argc, char** argv) {
    if (argc != 2) {
        std::cerr << "usage: " << argv[0] << " <program.bc>\n";
        return 2;
    }
    std::cerr << "tacvm: not implemented yet\n";
    return 1;
}
