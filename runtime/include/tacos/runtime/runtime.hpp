
#pragma once

#include <cstdint>
#include <iosfwd>
#include <memory>
#include <stdexcept>
#include <string>
#include <vector>

namespace tacos::runtime {

namespace detail { class CameraDetectorClient; }

// Host configuration; the VM-facing camera function still has zero arguments.
struct CameraOptions {
    std::string helper_path = "marker-detect";
    std::string fake_mode;
};

enum class ValueKind {
    Int,
    String,
    Float,
};

struct Value {
    ValueKind kind;
    std::int64_t int_value = 0;
    std::string string_value;
    double float_value = 0;

    static Value int_value_of(std::int64_t value);
    static Value string_value_of(std::string value);
    static Value float_value_of(double value);
};

class RuntimeError : public std::runtime_error {
public:
    explicit RuntimeError(const std::string& message);
};

class Runtime {
public:
    explicit Runtime(
        std::ostream& output,
        std::string wm_socket_path = "/tmp/tacos-wm.sock",
        std::string ipc_socket_path =
            "/tmp/tac-os-analyzer-map.sock"
    );

    Runtime(
        std::ostream& output,
        std::string wm_socket_path,
        std::string ipc_socket_path,
        CameraOptions camera_options
    );

    // Деструктор: освобождает ресурсы сокетов
    ~Runtime();

    // Запрещаем копирование Runtime
    Runtime(const Runtime&) = delete;
    Runtime& operator=(const Runtime&) = delete;

    bool has_function(
        const std::string& name,
        std::uint8_t arg_count
    ) const;

    Value call(
        const std::string& name,
        const std::vector<Value>& args
    );

    const std::string& wm_socket_path() const;
    const std::string& ipc_socket_path() const;

    void set_wm_event_socket_path(std::string path);
    std::string window_poll_event(int timeout_ms);
    void connect_wm_events();
    std::string camera_detect_markers();
    void mark_wm_window_open();
    bool is_wm_window_open() const;
    void disconnect_wm_events();

    // IPC: Map
    void ipc_listen();
    std::string ipc_receive();

    // IPC: Analyzer
    void ipc_send(const std::string& json);


private:
    std::ostream& output_;
    std::string wm_socket_path_;
    std::string ipc_socket_path_;

    std::string wm_event_socket_path_;
    int wm_event_fd_ = -1;
    std::string wm_event_buffer_;
    bool wm_window_open_ = false;

    // -1 означает, что сокет ещё не открыт.
    int ipc_listen_fd_ = -1;
    int ipc_connection_fd_ = -1;

    // Данные, полученные после очередного '\n'.
    std::string ipc_buffer_;
    std::unique_ptr<detail::CameraDetectorClient> camera_;
};

} // namespace tacos::runtime
