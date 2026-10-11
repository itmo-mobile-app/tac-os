#pragma once

#include <string>

namespace tacos::wm {
class Compositor;

class ControlServer {
public:
    ControlServer(Compositor& compositor, std::string socket_path, std::string client_path);
    ~ControlServer();
    ControlServer(const ControlServer&) = delete;
    ControlServer& operator=(const ControlServer&) = delete;
    bool initialize();
    void on_readable(int fd);
    int fd() const;
    void emit_user_close(long long window_id);

private:
    struct Impl;
    Impl* impl_;
};
} // namespace tacos::wm
