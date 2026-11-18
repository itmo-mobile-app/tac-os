#pragma once

#include <chrono>
#include <cstddef>
#include <cstdint>
#include <optional>
#include <string>
#include <string_view>

namespace tacos::runtime::detail {

class BrokerClient {
public:
    BrokerClient() = default;
    ~BrokerClient();
    BrokerClient(const BrokerClient&) = delete;
    BrokerClient& operator=(const BrokerClient&) = delete;

    int connect(const std::string& address, std::int64_t port, std::int64_t timeout_ms);
    int subscribe(std::string_view topic);
    int publish(std::string_view topic, std::string_view payload);
    int receive(std::int64_t timeout_ms);
    const std::string& topic() const;
    const std::string& payload() const;
    void close() noexcept;

private:
    struct Message {
        std::string topic;
        std::string payload;
    };

    int send_frame(const char* operation, const std::string& frame);
    int read_frame(std::size_t newline);
    int fail(const char* operation, int status, const char* reason);

    int fd_ = -1;
    std::string buffer_;
    std::chrono::steady_clock::time_point partial_deadline_{};
    std::optional<Message> message_;
};

} // namespace tacos::runtime::detail
