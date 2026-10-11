#include "tacos/wm/control_server.hpp"
#include "tacos/wm/compositor.hpp"
#include "tacos/wm/protocol.hpp"

#include <cerrno>
#include <csignal>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <deque>
#include <fcntl.h>
#include <sstream>
#include <string>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>
#include <wayland-server-core.h>

namespace tacos::wm {
struct ControlServer::Impl {
    Compositor& compositor;
    std::string socket_path;
    std::string client_path;
    int listen_fd = -1;
    wl_event_source* source = nullptr;
    bool created = false;
    bool visible = false;
    long long window_id = 1;
    int width = 960;
    int height = 640;
    std::string title = "Tactical Map";
    pid_t child = -1;
    std::deque<std::string> events;

    Impl(Compositor& c, std::string socket, std::string client)
        : compositor(c), socket_path(std::move(socket)), client_path(std::move(client)) {}

    static int readable(int fd, uint32_t mask, void* data) {
        auto* self = static_cast<Impl*>(data);
        if (mask & (WL_EVENT_HANGUP | WL_EVENT_ERROR)) return 0;
        self->accept_one(fd);
        return 0;
    }

    void accept_one(int fd) {
        int client = accept4(fd, nullptr, nullptr, SOCK_CLOEXEC);
        if (client < 0) return;
        std::string line;
        char buf[1024];
        while (line.find('\n') == std::string::npos && line.size() < 4096) {
            ssize_t n = recv(client, buf, sizeof(buf), 0);
            if (n <= 0) break;
            line.append(buf, static_cast<size_t>(n));
        }
        const auto end = line.find('\n');
        if (end != std::string::npos) line.resize(end);
        const std::string response = handle(line) + "\n";
        (void)send(client, response.data(), response.size(), MSG_NOSIGNAL);
        close(client);
    }

    static bool parse_id(const std::string& text, long long& id) {
        try {
            size_t used = 0;
            id = std::stoll(text, &used);
            return used == text.size() && id > 0;
        } catch (...) { return false; }
    }

    std::string handle(const std::string& line) {
        std::istringstream input(line);
        std::string command;
        input >> command;
        if (command == "CREATE") {
            int w = 0, h = 0;
            input >> w >> h;
            std::string rest;
            std::getline(input, rest);
            if (!rest.empty() && rest.front() == ' ') rest.erase(0, 1);
            if (w < 1 || h < 1 || w > 4096 || h > 4096 || rest.empty()) return "ERROR invalid CREATE arguments";
            if (created) return "ERROR only one window is supported";
            created = true; width = w; height = h; title = rest; window_id = 1;
            compositor.set_window_size(width, height);
            compositor.set_window_title(title);
            return "OK " + std::to_string(window_id);
        }
        if (command == "SHOW") {
            std::string id_text; input >> id_text;
            long long id = 0;
            if (!parse_id(id_text, id) || !created || id != window_id) return "ERROR unknown window id";
            if (visible) return "OK";
            visible = true;
            compositor.set_window_visible(true);
            child = fork();
            if (child == 0) {
                setenv("WAYLAND_DISPLAY", compositor.wayland_socket_name(), 1);
                execl(client_path.c_str(), client_path.c_str(), std::to_string(width).c_str(),
                      std::to_string(height).c_str(), title.c_str(), std::to_string(window_id).c_str(),
                      socket_path.c_str(), static_cast<char*>(nullptr));
                std::perror("exec tac_wm_window_client");
                _exit(127);
            }
            if (child < 0) { visible = false; compositor.set_window_visible(false); return "ERROR failed to start window client"; }
            return "OK";
        }
        if (command == "CLOSE") {
            std::string id_text; input >> id_text;
            long long id = 0;
            if (!parse_id(id_text, id) || !created || id != window_id) return "ERROR unknown window id";
            if (child > 0) { kill(child, SIGTERM); (void)waitpid(child, nullptr, WNOHANG); child = -1; }
            visible = false;
            compositor.set_window_visible(false);
            created = false;
            return "OK";
        }
        if (command == "USER_CLOSE") {
            std::string id_text; input >> id_text;
            long long id = 0;
            if (!parse_id(id_text, id) || !created || id != window_id) return "ERROR unknown window id";
            events.push_back("EVENT window_close " + std::to_string(window_id));
            if (child > 0) { kill(child, SIGTERM); (void)waitpid(child, nullptr, WNOHANG); child = -1; }
            visible = false;
            created = false;
            compositor.set_window_visible(false);
            return "OK";
        }
        if (command == "POLL_EVENT") {
            if (events.empty()) return "NONE";
            std::string event = events.front(); events.pop_front(); return event;
        }
        if (command == "PING") return "PONG";
        return "ERROR unknown command";
    }
};

ControlServer::ControlServer(Compositor& c, std::string path, std::string client)
    : impl_(new Impl(c, std::move(path), std::move(client))) {}
ControlServer::~ControlServer() {
    if (impl_->source) wl_event_source_remove(impl_->source);
    if (impl_->listen_fd >= 0) close(impl_->listen_fd);
    unlink(impl_->socket_path.c_str());
    delete impl_;
}
bool ControlServer::initialize() {
    auto& s = *impl_;
    s.listen_fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC | SOCK_NONBLOCK, 0);
    if (s.listen_fd < 0) return false;
    sockaddr_un address{};
    address.sun_family = AF_UNIX;
    if (s.socket_path.size() >= sizeof(address.sun_path)) return false;
    std::strncpy(address.sun_path, s.socket_path.c_str(), sizeof(address.sun_path) - 1);
    unlink(s.socket_path.c_str());
    if (bind(s.listen_fd, reinterpret_cast<sockaddr*>(&address), sizeof(address)) < 0) return false;
    if (chmod(s.socket_path.c_str(), 0600) < 0) return false;
    if (listen(s.listen_fd, 16) < 0) return false;
    s.source = wl_event_loop_add_fd(impl_->compositor.event_loop(), s.listen_fd,
                                    WL_EVENT_READABLE, Impl::readable, &s);
    if (!s.source) return false;
    return true;
}
void ControlServer::on_readable(int) {}
int ControlServer::fd() const { return impl_->listen_fd; }
void ControlServer::emit_user_close(long long) {}
} // namespace tacos::wm
