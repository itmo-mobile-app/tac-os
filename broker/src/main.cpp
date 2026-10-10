#include <iostream>
#include <string>

int main(int argc, char* argv[]) {
    if (argc != 2) {
        std::cerr << "Usage: " << argv[0] << " <port>\n";
        return 1;
    }

    int port;

    try {
        port = std::stoi(argv[1]);
    } catch (...) {
        std::cerr << "Invalid port\n";
        return 1;
    }

    if (port < 1 || port > 65535) {
        std::cerr << "Port must be between 1 and 65535\n";
        return 1;
    }

    std::cout << "Broker starting on port " << port << '\n';

    return 0;
}