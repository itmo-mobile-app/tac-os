#pragma once

#include "tacos/runtime/runtime.hpp"

namespace tacos::runtime::detail {

class DeviceState {
public:
    explicit DeviceState(DeviceOptions options);

    std::int64_t read_position(std::int64_t timeout_ms);
    std::int64_t read_heading(std::int64_t timeout_ms);
    const DevicePosition& position() const;
    double heading() const;
    std::int64_t now_ms() const;

private:
    DeviceOptions options_;
    std::optional<DevicePosition> position_;
    std::optional<double> heading_;
};

} // namespace tacos::runtime::detail
