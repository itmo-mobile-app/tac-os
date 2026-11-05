#pragma once

#include "tacos/runtime/runtime.hpp"

#include <sys/types.h>

#include <string>

namespace tacos::runtime::detail {

class CameraDetectorClient {
public:
    explicit CameraDetectorClient(CameraOptions options);
    ~CameraDetectorClient();
    CameraDetectorClient(const CameraDetectorClient&) = delete;
    CameraDetectorClient& operator=(const CameraDetectorClient&) = delete;

    std::string detect_markers();

private:
    void start();
    void check_child();
    void stop() noexcept;

    CameraOptions options_;
    std::string socket_path_;
    pid_t child_ = -1;
    int socket_ = -1;
    bool failed_ = false;
};

} // namespace tacos::runtime::detail
