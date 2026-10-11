#include "tacos/wm/protocol.hpp"

#include <cstdio>
#include <cstring>
#include <iostream>
#include <poll.h>
#include <unistd.h>
#include <string>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

static std::string request(const std::string& path, const std::string& command) {
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) { perror("socket"); return "ERROR socket"; }
    sockaddr_un address{}; address.sun_family = AF_UNIX;
    if (path.size() >= sizeof(address.sun_path)) { close(fd); return "ERROR socket path too long"; }
    std::strncpy(address.sun_path, path.c_str(), sizeof(address.sun_path) - 1);
    if (connect(fd, reinterpret_cast<sockaddr*>(&address), sizeof(address)) < 0) { perror("connect"); close(fd); return "ERROR connect"; }
    const std::string line = command + "\n";
    if (write(fd, line.data(), line.size()) < 0) { perror("write"); close(fd); return "ERROR write"; }
    char response[1024]{};
    ssize_t n = read(fd, response, sizeof(response) - 1);
    close(fd);
    return n > 0 ? std::string(response, static_cast<size_t>(n)) : "ERROR no response";
}

int main(int argc, char** argv) {
    std::string socket_path = tacos::wm::kDefaultSocket;
    int arg = 1;
    if (argc > 2 && std::strcmp(argv[1], "--socket") == 0) { socket_path = argv[2]; arg = 3; }
    if (argc <= arg || std::strcmp(argv[arg], "demo") == 0) {
        auto created = request(socket_path, "CREATE 960 640 Tactical Map");
        std::cout << created;
        if (created.rfind("OK ", 0) != 0) return 1;
        const std::string id = created.substr(3);
        auto shown = request(socket_path, "SHOW " + id);
        std::cout << shown;
        if (shown.rfind("OK", 0) != 0) return 1;
        std::cout << "Window should now be visible. Click the red X to test window_close, or press Enter to close programmatically.\n";
        for (;;) {
            const auto event = request(socket_path, "POLL_EVENT");
            if (event.rfind("EVENT window_close ", 0) == 0) {
                std::cout << event;
                return 0;
            }
            pollfd input_fd{STDIN_FILENO, POLLIN, 0};
            const int ready = poll(&input_fd, 1, 200);
            if (ready > 0 && (input_fd.revents & POLLIN)) {
                std::string ignored; std::getline(std::cin, ignored);
                std::cout << request(socket_path, "CLOSE " + id);
                return 0;
            }
        }
    }
    const std::string command = argv[arg++];
    if (command == "create" && argc >= arg + 3) {
        std::string line = "CREATE " + std::string(argv[arg]) + " " + argv[arg + 1] + " ";
        for (int i = arg + 2; i < argc; ++i) { if (i > arg + 2) line += " "; line += argv[i]; }
        std::cout << request(socket_path, line); return 0;
    }
    if ((command == "show" || command == "close" || command == "user-close") && argc > arg) {
        const char* op = command == "show" ? "SHOW" : command == "close" ? "CLOSE" : "USER_CLOSE";
        std::cout << request(socket_path, std::string(op) + " " + argv[arg]); return 0;
    }
    if (command == "poll") { std::cout << request(socket_path, "POLL_EVENT"); return 0; }
    if (command == "ping") { std::cout << request(socket_path, "PING"); return 0; }
    std::cerr << "Usage: tac_wm_test_client [--socket PATH] demo | create WIDTH HEIGHT TITLE | show ID | close ID | user-close ID | poll | ping\n";
    return 2;
}
