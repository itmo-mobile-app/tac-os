#include "camera_detector_client.hpp"

#include <nlohmann/json.hpp>

#include <poll.h>
#include <signal.h>
#include <spawn.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>

#include <algorithm>
#include <cerrno>
#include <chrono>
#include <cmath>
#include <cstring>
#include <unordered_set>
#include <utility>
#include <vector>

extern char** environ;

namespace tacos::runtime::detail {
namespace {

using Clock = std::chrono::steady_clock;
using Deadline = Clock::time_point;
using Json = nlohmann::ordered_json;
constexpr std::size_t max_frame_bytes = 64 * 1024;
constexpr auto io_timeout = std::chrono::seconds(5);

[[noreturn]] void fail(const std::string& message) {
    throw RuntimeError("camera.detectMarkers: " + message);
}

int remaining_ms(Deadline deadline) {
    const auto remaining = deadline - Clock::now();
    if (remaining <= Clock::duration::zero()) {
        fail("helper timeout");
    }
    return static_cast<int>(
        std::chrono::ceil<std::chrono::milliseconds>(remaining).count());
}

void wait_ready(int fd, short events, Deadline deadline) {
    for (;;) {
        pollfd descriptor{fd, events, 0};
        const int result = ::poll(&descriptor, 1, remaining_ms(deadline));
        if (result > 0) {
            if (descriptor.revents & POLLNVAL) {
                fail("invalid helper socket");
            }
            // HUP/ERR are handled by the following nonblocking send/recv.
            return;
        }
        if (result == 0) {
            fail("helper timeout");
        }
        if (errno != EINTR) {
            fail("socket poll failed: " + std::string(std::strerror(errno)));
        }
    }
}

struct SocketCloser {
    int fd;
    ~SocketCloser() { if (fd >= 0) ::close(fd); }
};

void send_detect(int fd, Deadline deadline) {
    const std::string request = "{\"command\":\"DETECT\"}\n";
    std::size_t sent = 0;
    while (sent < request.size()) {
        remaining_ms(deadline);
        const auto count = ::send(fd, request.data() + sent, request.size() - sent,
                                  MSG_NOSIGNAL | MSG_DONTWAIT);
        if (count > 0) {
            sent += static_cast<std::size_t>(count);
        } else if (count < 0 && errno == EINTR) {
            continue;
        } else if (count < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
            wait_ready(fd, POLLOUT, deadline);
        } else {
            fail("cannot send DETECT to helper");
        }
    }
}

std::string read_response(int fd, Deadline deadline) {
    std::string response;
    char buffer[4096];
    while (response.size() < max_frame_bytes) {
        remaining_ms(deadline);
        const auto count = ::recv(fd, buffer,
            std::min(sizeof(buffer), max_frame_bytes - response.size()), MSG_DONTWAIT);
        if (count > 0) {
            response.append(buffer, static_cast<std::size_t>(count));
            const auto newline = response.find('\n');
            if (newline != std::string::npos) {
                if (newline + 1 != response.size()) {
                    fail("unexpected data after helper response");
                }
                response.resize(newline);
                return response;
            }
        } else if (count == 0) {
            fail("helper closed connection before a complete response");
        } else if (errno == EINTR) {
            continue;
        } else if (errno == EAGAIN || errno == EWOULDBLOCK) {
            wait_ready(fd, POLLIN, deadline);
        } else {
            fail("cannot read helper response: " + std::string(std::strerror(errno)));
        }
    }
    fail("helper frame exceeds 64 KiB");
}

struct Reply {
    std::string detections;
    std::string error;
};

Reply decode_response(const std::string& frame) {
    if (frame.find('\0') != std::string::npos) {
        fail("NUL byte in helper response");
    }
    Json response;
    std::vector<std::unordered_set<std::string>> object_keys;
    bool duplicate = false;
    try {
        response = Json::parse(frame,
            [&](int depth, Json::parse_event_t event, Json& value) {
                if (depth > 8) {
                    fail("helper JSON nesting is too deep");
                }
                if (event == Json::parse_event_t::object_start) {
                    object_keys.emplace_back();
                } else if (event == Json::parse_event_t::key) {
                    if (!object_keys.back().insert(value.get<std::string>()).second) {
                        duplicate = true;
                    }
                } else if (event == Json::parse_event_t::object_end) {
                    object_keys.pop_back();
                }
                return true;
            });
    } catch (const Json::exception&) {
        fail("invalid JSON in helper response");
    }
    if (duplicate || !response.is_object() || response.size() != 2 ||
        !response.contains("ok") || !response["ok"].is_boolean()) {
        fail("invalid helper response fields");
    }
    if (!response["ok"].get<bool>()) {
        if (!response.contains("error") || !response["error"].is_string()) {
            fail("invalid helper error response");
        }
        const auto code = response["error"].get<std::string>();
        if (code != "CAMERA_UNAVAILABLE" && code != "DETECTION_FAILED" &&
            code != "BAD_REQUEST") {
            fail("unknown helper error code");
        }
        return {"", code};
    }
    if (!response.contains("detections") || !response["detections"].is_array()) {
        fail("invalid detections array");
    }
    int previous_id = -1;
    for (const auto& detection : response["detections"]) {
        if (!detection.is_object() || detection.size() != 3 ||
            !detection.contains("id") || !detection["id"].is_number_integer() ||
            !detection.contains("relativeAngle") || !detection["relativeAngle"].is_number() ||
            !detection.contains("distance") || !detection["distance"].is_number()) {
            fail("invalid detection fields");
        }
        const auto& id_value = detection["id"];
        if ((id_value.is_number_unsigned() && id_value.get<std::uint64_t>() > 71) ||
            (!id_value.is_number_unsigned() &&
             (id_value.get<std::int64_t>() < 0 || id_value.get<std::int64_t>() > 71))) {
            fail("marker id must be in 0..71");
        }
        const int id = id_value.get<int>();
        const double angle = detection["relativeAngle"].get<double>();
        const double distance = detection["distance"].get<double>();
        if (id <= previous_id || !std::isfinite(angle) ||
            !std::isfinite(distance) || distance <= 0) {
            fail("detections require ascending unique ids and finite positive distances");
        }
        previous_id = id;
    }
    return {response["detections"].dump(), ""};
}

} // namespace

CameraDetectorClient::CameraDetectorClient(CameraOptions options)
    : options_(std::move(options)) {}

CameraDetectorClient::~CameraDetectorClient() { stop(); }

void CameraDetectorClient::check_child() {
    if (child_ <= 0) {
        fail("helper is not running");
    }
    int status = 0;
    pid_t result;
    do {
        result = ::waitpid(child_, &status, WNOHANG);
    } while (result < 0 && errno == EINTR);
    if (result == 0) {
        return;
    }
    if (result < 0 && errno != ECHILD) {
        fail("cannot check helper process: " + std::string(std::strerror(errno)));
    }
    child_ = -1;
    if (result > 0 && WIFEXITED(status)) {
        fail("helper exited with status " + std::to_string(WEXITSTATUS(status)));
    }
    fail("helper terminated unexpectedly or child ownership was lost");
}

void CameraDetectorClient::start() {
    if (socket_ >= 0) {
        check_child();
        return;
    }
    if (options_.helper_path.empty() || options_.helper_path.find('\0') != std::string::npos) {
        fail("invalid helper executable path");
    }
    const auto& mode = options_.fake_mode;
    if (!mode.empty() && mode != "two" && mode != "empty" &&
        mode != "camera-unavailable" && mode != "detection-failed") {
        fail("invalid fake mode");
    }
    socket_path_ = "/tmp/tac-os-detector-" + std::to_string(::getpid()) + ".sock";
    struct stat info{};
    if (::lstat(socket_path_.c_str(), &info) == 0) {
        if (!S_ISSOCK(info.st_mode) || info.st_uid != ::geteuid()) {
            fail("detector socket path is occupied by an unrelated file");
        }
    } else if (errno != ENOENT) {
        fail("cannot inspect detector socket path");
    }

    const auto deadline = Clock::now() + io_timeout;
    std::vector<char*> argv{options_.helper_path.data(),
                           const_cast<char*>("-socket"), socket_path_.data()};
    if (!mode.empty()) {
        argv.push_back(const_cast<char*>("-fake"));
        argv.push_back(options_.fake_mode.data());
    }
    argv.push_back(nullptr);

    posix_spawnattr_t attributes;
    int error = ::posix_spawnattr_init(&attributes);
    if (error != 0) {
        fail("cannot initialize helper launch: " + std::string(std::strerror(error)));
    }
    sigset_t empty_mask, default_signals;
    ::sigemptyset(&empty_mask);
    ::sigemptyset(&default_signals);
    ::sigaddset(&default_signals, SIGTERM);
    ::sigaddset(&default_signals, SIGINT);
    ::sigaddset(&default_signals, SIGPIPE);
    error = ::posix_spawnattr_setsigmask(&attributes, &empty_mask);
    if (error == 0) error = ::posix_spawnattr_setsigdefault(&attributes, &default_signals);
    if (error == 0) error = ::posix_spawnattr_setflags(&attributes,
        POSIX_SPAWN_SETSIGMASK | POSIX_SPAWN_SETSIGDEF);
    if (error == 0) error = ::posix_spawnp(&child_, options_.helper_path.c_str(),
        nullptr, &attributes, argv.data(), environ);
    ::posix_spawnattr_destroy(&attributes);
    if (error != 0) {
        child_ = -1;
        fail("cannot start helper: " + std::string(std::strerror(error)));
    }

    sockaddr_un address{};
    address.sun_family = AF_UNIX;
    std::memcpy(address.sun_path, socket_path_.c_str(), socket_path_.size() + 1);
    for (;;) {
        remaining_ms(deadline);
        check_child();
        SocketCloser candidate{::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC | SOCK_NONBLOCK, 0)};
        if (candidate.fd < 0) {
            fail("cannot create helper socket");
        }
        int result = ::connect(candidate.fd, reinterpret_cast<sockaddr*>(&address), sizeof(address));
        int connect_error = result < 0 ? errno : 0;
        if (connect_error == EINPROGRESS) {
            wait_ready(candidate.fd, POLLOUT, deadline);
            socklen_t length = sizeof(connect_error);
            if (::getsockopt(candidate.fd, SOL_SOCKET, SO_ERROR, &connect_error, &length) != 0) {
                fail("cannot check helper connection");
            }
        }
        if (connect_error == 0) {
            // Never use another Runtime's active socket, even if its path collided.
            ucred peer{};
            socklen_t length = sizeof(peer);
            if (::getsockopt(candidate.fd, SOL_SOCKET, SO_PEERCRED, &peer, &length) != 0 ||
                peer.pid != child_ || peer.uid != ::geteuid()) {
                fail("detector socket does not belong to the spawned helper");
            }
            check_child();
            socket_ = candidate.fd;
            candidate.fd = -1;
            return;
        }
        if (connect_error != ENOENT && connect_error != ECONNREFUSED &&
            connect_error != EAGAIN && connect_error != EINTR) {
            fail("cannot connect to helper: " + std::string(std::strerror(connect_error)));
        }
        ::poll(nullptr, 0, std::min(20, remaining_ms(deadline)));
    }
}

std::string CameraDetectorClient::detect_markers() {
    if (failed_) {
        fail("helper failed; automatic restart is disabled");
    }
    Reply reply;
    try {
        start();
        const auto deadline = Clock::now() + io_timeout;
        send_detect(socket_, deadline);
        reply = decode_response(read_response(socket_, deadline));
        check_child();
    } catch (...) {
        failed_ = true;
        stop();
        throw;
    }
    // A well-formed wire error leaves the stream usable for the next DETECT.
    if (!reply.error.empty()) {
        fail("helper reported " + reply.error);
    }
    return reply.detections;
}

void CameraDetectorClient::stop() noexcept {
    if (socket_ >= 0) {
        ::close(socket_);
        socket_ = -1;
    }
    if (child_ <= 0) return;
    int status = 0;
    auto reap = [&] {
        pid_t result;
        do {
            result = ::waitpid(child_, &status, WNOHANG);
        } while (result < 0 && errno == EINTR);
        return result == child_ || (result < 0 && errno == ECHILD);
    };
    if (!reap()) {
        ::kill(child_, SIGTERM);
        // The Go helper allows up to three seconds for a request in progress.
        const auto deadline = Clock::now() + std::chrono::milliseconds(3500);
        while (!reap()) {
            if (Clock::now() >= deadline) {
                ::kill(child_, SIGKILL);
                while (::waitpid(child_, &status, 0) < 0 && errno == EINTR) {}
                break;
            }
            ::poll(nullptr, 0, 10);
        }
    }
    child_ = -1;
    // The helper owns socket removal; never unlink an unrelated endpoint here.
}

} // namespace tacos::runtime::detail
