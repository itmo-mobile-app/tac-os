
#pragma once

#include <chrono>
#include <cstdint>
#include <iosfwd>
#include <memory>
#include <optional>
#include <stdexcept>
#include <string>
#include <vector>

namespace tacos::runtime {

namespace detail {
class CameraDetectorClient;
class DeviceState;
}

// Host configuration; the VM-facing camera function still has zero arguments.
struct CameraOptions {
    std::string helper_path = "marker-detect";
    std::string fake_mode;
};

struct DevicePosition {
    double latitude;
    double longitude;
};

// Explicit, fixed sources selected by the host before running bytecode.
struct DeviceOptions {
    std::optional<DevicePosition> position;
    std::optional<double> heading;
    std::optional<std::int64_t> fixed_now_ms;
    std::chrono::milliseconds position_delay{0};
    std::chrono::milliseconds heading_delay{0};
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

    Runtime(
        std::ostream& output,
        std::string wm_socket_path,
        std::string ipc_socket_path,
        CameraOptions camera_options,
        DeviceOptions device_options
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

    std::string camera_detect_markers();

    // IPC: Map
    void ipc_listen();
    std::string ipc_receive();

    // IPC: Analyzer
    void ipc_send(const std::string& json);


private:
    struct FunctionEntry;
    static const FunctionEntry* find_function(const std::string& name);

    std::ostream& output_;
    std::string wm_socket_path_;
    std::string ipc_socket_path_;

    // -1 означает, что сокет ещё не открыт.
    int ipc_listen_fd_ = -1;
    int ipc_connection_fd_ = -1;

    // Данные, полученные после очередного '\n'.
    std::string ipc_buffer_;
    std::unique_ptr<detail::CameraDetectorClient> camera_;
    std::unique_ptr<detail::DeviceState> device_;
};

} // namespace tacos::runtime
