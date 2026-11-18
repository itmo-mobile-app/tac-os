#include "broker_client.hpp"
#include "tacos/runtime/runtime.hpp"

#include <algorithm>
#include <cerrno>
#include <cstring>
#include <iostream>

#include <arpa/inet.h>
#include <poll.h>
#include <sys/socket.h>
#include <unistd.h>

#include <nlohmann/json.hpp>

namespace tacos::runtime::detail {
namespace {

using Clock = std::chrono::steady_clock;
using namespace std::chrono_literals;

constexpr int unavailable = -1;
constexpr int timed_out = -2;
constexpr int malformed = -3;
constexpr std::size_t frame_limit = 16 * 1024;
constexpr auto io_timeout = 1000ms;

enum class Wait { Ready, Timeout, Error };

Wait wait_for(int fd, short events, Clock::time_point deadline) {
    do {
        const auto remaining = std::max(Clock::duration::zero(), deadline - Clock::now());
        const auto ms = std::chrono::ceil<std::chrono::milliseconds>(remaining);
        pollfd socket{fd, events, 0};
        const int result = ::poll(&socket, 1, static_cast<int>(ms.count()));
        if (result > 0) {
            return (socket.revents & POLLNVAL) ? Wait::Error : Wait::Ready;
        }
        if (result == 0) {
            return Wait::Timeout;
        }
        if (errno != EINTR) {
            return Wait::Error;
        }
    } while (Clock::now() < deadline);
    return Wait::Timeout;
}

bool valid_topic(std::string_view topic) {
    if (topic.empty() || topic.find_first_of(" \t\r\n\v\f") != topic.npos ||
        topic.find('\0') != topic.npos) {
        return false;
    }
    try {
        // The strict serializer validates UTF-8 in the topic as well as the payload parser.
        (void)nlohmann::json(std::string(topic)).dump();
        return true;
    } catch (const nlohmann::json::type_error&) {
        return false;
    }
}

bool valid_payload(std::string_view payload) {
    const auto first = payload.find_first_not_of(" \t");
    return first != payload.npos && payload[first] == '{' &&
        payload.find_first_of("\r\n") == payload.npos && payload.find('\0') == payload.npos &&
        nlohmann::json::accept(payload.begin(), payload.end());
}

} // namespace

BrokerClient::~BrokerClient() {
    close();
}

int BrokerClient::connect(const std::string& address, std::int64_t port, std::int64_t timeout_ms) {
    if (fd_ != -1) {
        throw RuntimeError("broker.connect: connection already open");
    }
    if (port < 1 || port > 65535 || timeout_ms < 1 || timeout_ms > io_timeout.count()) {
        throw RuntimeError("broker.connect: expected port 1..65535 and timeout 1..1000 ms");
    }

    sockaddr_in ipv4{};
    sockaddr_in6 ipv6{};
    const sockaddr* endpoint;
    socklen_t endpoint_size;
    int family;
    if (address.find('\0') != address.npos) {
        throw RuntimeError("broker.connect: expected a numeric IP address");
    }
    if (::inet_pton(AF_INET, address.c_str(), &ipv4.sin_addr) == 1) {
        ipv4.sin_family = AF_INET;
        ipv4.sin_port = htons(static_cast<std::uint16_t>(port));
        endpoint = reinterpret_cast<const sockaddr*>(&ipv4);
        endpoint_size = sizeof(ipv4);
        family = AF_INET;
    } else if (::inet_pton(AF_INET6, address.c_str(), &ipv6.sin6_addr) == 1) {
        ipv6.sin6_family = AF_INET6;
        ipv6.sin6_port = htons(static_cast<std::uint16_t>(port));
        endpoint = reinterpret_cast<const sockaddr*>(&ipv6);
        endpoint_size = sizeof(ipv6);
        family = AF_INET6;
    } else {
        throw RuntimeError("broker.connect: expected a numeric IP address");
    }

    const auto deadline = Clock::now() + std::chrono::milliseconds(timeout_ms);
    fd_ = ::socket(family, SOCK_STREAM | SOCK_NONBLOCK | SOCK_CLOEXEC, 0);
    if (fd_ == -1) {
        return fail("connect", unavailable, std::strerror(errno));
    }
    if (::connect(fd_, endpoint, endpoint_size) == 0) {
        if (Clock::now() >= deadline) {
            return fail("connect", timed_out, "connection deadline exceeded");
        }
        return 0;
    }
    if (errno != EINPROGRESS && errno != EINTR) {
        return fail("connect", unavailable, std::strerror(errno));
    }

    const auto readiness = wait_for(fd_, POLLOUT, deadline);
    if (readiness == Wait::Timeout) {
        return fail("connect", timed_out, "connection deadline exceeded");
    }
    if (readiness == Wait::Error) {
        return fail("connect", unavailable, "socket poll failed");
    }
    int error = 0;
    socklen_t error_size = sizeof(error);
    if (::getsockopt(fd_, SOL_SOCKET, SO_ERROR, &error, &error_size) == -1) {
        return fail("connect", unavailable, std::strerror(errno));
    }
    if (error != 0) {
        return fail("connect", unavailable, std::strerror(error));
    }
    if (Clock::now() >= deadline) {
        return fail("connect", timed_out, "connection deadline exceeded");
    }
    return 0;
}

int BrokerClient::subscribe(std::string_view topic) {
    if (topic.size() > frame_limit - sizeof("SUB ") || !valid_topic(topic)) {
        throw RuntimeError("broker.subscribe: invalid topic or oversized frame");
    }
    return send_frame("subscribe", "SUB " + std::string(topic) + '\n');
}

int BrokerClient::publish(std::string_view topic, std::string_view payload) {
    constexpr std::size_t body_limit = frame_limit - sizeof("PUB  ");
    if (topic.size() > body_limit || payload.size() > body_limit - topic.size() ||
        !valid_topic(topic) || !valid_payload(payload)) {
        throw RuntimeError("broker.publish: invalid topic, payload or oversized frame");
    }
    std::string frame = "PUB ";
    frame.append(topic).append(" ").append(payload).append("\n");
    return send_frame("publish", frame);
}

int BrokerClient::send_frame(const char* operation, const std::string& frame) {
    if (fd_ == -1) {
        return fail(operation, unavailable, "not connected");
    }
    const auto deadline = Clock::now() + io_timeout;
    std::size_t written = 0;
    while (written < frame.size()) {
        if (Clock::now() >= deadline) {
            return fail(operation, timed_out, "write deadline exceeded");
        }
        const auto count = ::send(fd_, frame.data() + written, frame.size() - written, MSG_NOSIGNAL);
        if (count > 0) {
            written += static_cast<std::size_t>(count);
            continue;
        }
        if (count < 0 && errno == EINTR) {
            continue;
        }
        if (count == 0 || (errno != EAGAIN && errno != EWOULDBLOCK)) {
            return fail(operation, unavailable, "socket write failed");
        }
        const auto readiness = wait_for(fd_, POLLOUT, deadline);
        if (readiness == Wait::Timeout) {
            return fail(operation, timed_out, "write deadline exceeded");
        }
        if (readiness == Wait::Error) {
            return fail(operation, unavailable, "socket poll failed");
        }
    }
    return 0;
}

int BrokerClient::receive(std::int64_t timeout_ms) {
    message_.reset();
    if (timeout_ms < 0 || timeout_ms > io_timeout.count()) {
        throw RuntimeError("broker.receive: expected timeout 0..1000 ms");
    }
    if (fd_ == -1) {
        return fail("receive", unavailable, "not connected");
    }

    const auto idle_deadline = Clock::now() + std::chrono::milliseconds(timeout_ms);
    for (;;) {
        const auto newline = buffer_.find('\n');
        if (newline != buffer_.npos) {
            return read_frame(newline);
        }
        if (buffer_.size() == frame_limit) {
            return fail("receive", malformed, "frame exceeds 16 KiB");
        }

        const bool assembling = !buffer_.empty();
        const auto deadline = assembling ? std::min(partial_deadline_, idle_deadline) : idle_deadline;
        if (assembling && Clock::now() >= partial_deadline_) {
            return fail("receive", timed_out, "partial frame deadline exceeded");
        }
        if (timeout_ms != 0 && Clock::now() >= idle_deadline) {
            return 0;
        }

        const auto readiness = wait_for(fd_, POLLIN, deadline);
        if (readiness == Wait::Error) {
            return fail("receive", unavailable, "socket poll failed");
        }
        if (assembling && Clock::now() >= partial_deadline_) {
            return fail("receive", timed_out, "partial frame deadline exceeded");
        }
        if (readiness == Wait::Timeout) {
            return 0;
        }

        char bytes[4096];
        const auto count = ::recv(fd_, bytes, std::min(sizeof(bytes), frame_limit - buffer_.size()), 0);
        if (count == 0) {
            return fail("receive", unavailable, "peer disconnected");
        }
        if (count < 0) {
            if (errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK) {
                if (timeout_ms == 0) {
                    return 0;
                }
                continue;
            }
            return fail("receive", unavailable, std::strerror(errno));
        }

        const auto chunk = std::string_view(bytes, static_cast<std::size_t>(count));
        if (!assembling || chunk.find('\n') != chunk.npos) {
            // Any trailing partial frame starts in this chunk, after the last LF.
            partial_deadline_ = Clock::now() + io_timeout;
        }
        buffer_.append(chunk);
    }
}

int BrokerClient::read_frame(std::size_t newline) {
    const std::string_view line(buffer_.data(), newline);
    const auto separator = line.find(' ', 4);
    if (line.substr(0, 4) != "MSG " || separator == line.npos) {
        return fail("receive", malformed, "expected MSG topic and payload");
    }
    const auto topic = line.substr(4, separator - 4);
    const auto payload = line.substr(separator + 1);
    if (!valid_topic(topic) || !valid_payload(payload)) {
        return fail("receive", malformed, "invalid topic or JSON object");
    }

    message_ = Message{std::string(topic), std::string(payload)};
    buffer_.erase(0, newline + 1);
    return 1;
}

const std::string& BrokerClient::topic() const {
    if (!message_) {
        throw RuntimeError("broker.topic: no current message");
    }
    return message_->topic;
}

const std::string& BrokerClient::payload() const {
    if (!message_) {
        throw RuntimeError("broker.payload: no current message");
    }
    return message_->payload;
}

void BrokerClient::close() noexcept {
    if (fd_ != -1) {
        ::close(fd_);
        fd_ = -1;
    }
    buffer_.clear();
    message_.reset();
}

int BrokerClient::fail(const char* operation, int status, const char* reason) {
    close();
    std::cerr << "broker." << operation << ": " << reason << '\n';
    return status;
}

} // namespace tacos::runtime::detail
