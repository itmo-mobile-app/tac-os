#include <iostream>

// Usage: tacc <input.tc> -o <output.bc>
int main(int argc, char** argv) {
    if (argc != 4) {
        std::cerr << "usage: " << argv[0] << " <input.tc> -o <output.bc>\n";
        return 2;
    }
    std::cerr << "tacc: not implemented yet\n";
    return 1;
}
