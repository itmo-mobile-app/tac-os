
#pragma once

#include <cstdint>
#include <iosfwd>
#include <stdexcept>
#include <string>
#include <vector>

namespace tacos::runtime {

enum class ValueKind {
    Int,
    String,
};

struct Value {
    ValueKind kind;
    std::int64_t int_value = 0;
    std::string string_value;

    static Value int_value_of(std::int64_t value);
    static Value string_value_of(std::string value);
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

    // IPC: Map
    void ipc_listen();
    std::string ipc_receive();

    // IPC: Analyzer
    void ipc_send(const std::string& json);


private:
    std::ostream& output_;
    std::string wm_socket_path_;
    std::string ipc_socket_path_;

    // -1 означает, что сокет ещё не открыт.
    int ipc_listen_fd_ = -1;
    int ipc_connection_fd_ = -1;

    // Данные, полученные после очередного '\n'.
    std::string ipc_buffer_;
};

} // namespace tacos::runtime
