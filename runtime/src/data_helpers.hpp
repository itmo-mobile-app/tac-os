#pragma once

#include "tacos/runtime/runtime.hpp"

#include <iosfwd>
#include <vector>

// Runtime handlers for the json.* and text.* data helpers (spec/runtime-api.md).
namespace tacos::runtime::detail {

Value json_array_length(Runtime&, std::ostream&, const std::vector<Value>&);
Value json_array_object(Runtime&, std::ostream&, const std::vector<Value>&);
Value json_get_int(Runtime&, std::ostream&, const std::vector<Value>&);
Value json_get_float(Runtime&, std::ostream&, const std::vector<Value>&);
Value json_get_string(Runtime&, std::ostream&, const std::vector<Value>&);
Value json_quote(Runtime&, std::ostream&, const std::vector<Value>&);
Value text_concat(Runtime&, std::ostream&, const std::vector<Value>&);
Value text_from_int(Runtime&, std::ostream&, const std::vector<Value>&);
Value text_from_float(Runtime&, std::ostream&, const std::vector<Value>&);

} // namespace tacos::runtime::detail
