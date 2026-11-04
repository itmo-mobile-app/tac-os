#pragma once

#include "tacos/runtime/runtime.hpp"

#include <stdexcept>
#include <string>

namespace tacos::vm {

class VmError : public std::runtime_error {
public:
    explicit VmError(const std::string& message) : std::runtime_error(message) {}
};

// One isolated execution per call. Concurrent calls require distinct Runtime
// instances, output streams and external resources. See spec/vm-execution.md.
runtime::Value run_file(const std::string& path, runtime::Runtime& runtime);

} // namespace tacos::vm
