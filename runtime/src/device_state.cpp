#include "device_state.hpp"

#include <algorithm>
#include <cmath>
#include <ctime>
#include <iostream>
#include <limits>
#include <thread>
#include <utility>

namespace tacos::runtime::detail {
namespace {

using Clock = std::chrono::steady_clock;
using Milliseconds = std::chrono::milliseconds;

std::int64_t sensor_failure(const char* name, std::int64_t status, const char* reason) {
    std::cerr << name << ": " << reason << '\n';
    return status;
}

template<typename Sample, typename Validator>
std::int64_t acquire(const char* name, std::int64_t timeout_ms, Milliseconds delay,
                     const std::optional<Sample>& source, std::optional<Sample>& snapshot,
                     Validator valid) {
    snapshot.reset();
    if (timeout_ms < 1 || timeout_ms > 1000) {
        throw RuntimeError(std::string(name) + ": expected timeout 1..1000 ms");
    }
    const auto timeout = Milliseconds(timeout_ms);
    const auto start = Clock::now();
    const auto deadline = start + timeout;
    if (!source) {
        return sensor_failure(name, -1, "source unavailable");
    }
    if (delay > Milliseconds::zero()) {
        std::this_thread::sleep_until(start + std::min(delay, timeout));
    }
    if (Clock::now() >= deadline) {
        return sensor_failure(name, -2, "source deadline exceeded");
    }
    if (!valid(*source)) {
        return sensor_failure(name, -3, "invalid source reading");
    }
    snapshot = *source;
    return 0;
}

} // namespace

DeviceState::DeviceState(DeviceOptions options) : options_(std::move(options)) {
    if (options_.position_delay < Milliseconds::zero() ||
        options_.heading_delay < Milliseconds::zero()) {
        throw RuntimeError("device source delay must be nonnegative");
    }
}

std::int64_t DeviceState::read_position(std::int64_t timeout_ms) {
    return acquire("location.read", timeout_ms, options_.position_delay, options_.position, position_,
                   [](const DevicePosition& value) {
                       return std::isfinite(value.latitude) &&
                              std::isfinite(value.longitude) &&
                              value.latitude >= -90.0 && value.latitude <= 90.0 &&
                              value.longitude >= -180.0 && value.longitude <= 180.0;
                   });
}

std::int64_t DeviceState::read_heading(std::int64_t timeout_ms) {
    return acquire("orientation.read", timeout_ms, options_.heading_delay, options_.heading, heading_,
                   [](double value) {
                       return std::isfinite(value) && value >= 0.0 && value < 360.0;
                   });
}

const DevicePosition& DeviceState::position() const {
    if (!position_) {
        throw RuntimeError("location: no current position");
    }
    return *position_;
}

double DeviceState::heading() const {
    if (!heading_) {
        throw RuntimeError("orientation.heading: no current heading");
    }
    return *heading_;
}

std::int64_t DeviceState::now_ms() const {
    if (options_.fixed_now_ms) {
        return *options_.fixed_now_ms;
    }

    timespec timestamp{};
    if (::clock_gettime(CLOCK_REALTIME, &timestamp) != 0) {
        throw RuntimeError("time.nowMs: cannot read the system wall clock");
    }

    constexpr auto min = std::numeric_limits<std::int64_t>::min();
    constexpr auto max = std::numeric_limits<std::int64_t>::max();
    auto fraction_ms = timestamp.tv_nsec / 1000000;
    // Move the fraction toward the epoch so the seconds product fits whenever possible.
    if (timestamp.tv_sec < 0 && fraction_ms > 0) {
        ++timestamp.tv_sec;
        fraction_ms -= 1000;
    }
    if (timestamp.tv_sec < min / 1000 || timestamp.tv_sec > max / 1000) {
        throw RuntimeError("time.nowMs: system clock is outside the Int range");
    }
    const auto seconds_ms = static_cast<std::int64_t>(timestamp.tv_sec) * 1000;
    if ((fraction_ms > 0 && seconds_ms > max - fraction_ms) ||
        (fraction_ms < 0 && seconds_ms < min - fraction_ms)) {
        throw RuntimeError("time.nowMs: system clock is outside the Int range");
    }
    return seconds_ms + fraction_ms;
}

} // namespace tacos::runtime::detail
