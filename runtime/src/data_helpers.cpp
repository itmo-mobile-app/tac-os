#include "data_helpers.hpp"

#include <nlohmann/json.hpp>

#include <cmath>
#include <cstdint>
#include <limits>
#include <set>
#include <string>
#include <vector>

// String/JSON helpers for Observation and Target (spec/runtime-api.md ->
// "Minimal String data helpers"). They are pure: no I/O, no state shared
// between calls or Runtime instances. Every result is a new owned Value.

namespace tacos::runtime::detail {

namespace {

using json = nlohmann::json;

constexpr std::size_t kMaxBytes = 65536;
constexpr std::size_t kMaxArrayElements = 72;

const std::string& string_arg(const std::vector<Value>& args, std::size_t index,
                              const char* helper) {
    if (args[index].kind != ValueKind::String) {
        throw RuntimeError(std::string(helper) + ": argument " +
                           std::to_string(index + 1) + " must be a String");
    }
    return args[index].string_value;
}

void check_size(std::size_t size, const char* helper, const char* what) {
    if (size > kMaxBytes) {
        throw RuntimeError(std::string(helper) + ": " + what + " exceeds " +
                           std::to_string(kMaxBytes) + " bytes");
    }
}

// Parses the whole source: one root value, valid UTF-8, no duplicate keys.
json parse(const std::string& source, const char* helper) {
    check_size(source.size(), helper, "input");
    std::vector<std::set<std::string>> keys;
    std::string duplicate;
    const json::parser_callback_t callback =
        [&](int, json::parse_event_t event, json& parsed) {
            switch (event) {
            case json::parse_event_t::object_start:
                keys.emplace_back();
                break;
            case json::parse_event_t::object_end:
                keys.pop_back();
                break;
            case json::parse_event_t::key:
                if (!keys.back().insert(parsed.get<std::string>()).second &&
                    duplicate.empty()) {
                    duplicate = parsed.get<std::string>();
                }
                break;
            default:
                break;
            }
            return true;
        };
    json document;
    try {
        document = json::parse(source, callback);
    } catch (const json::exception& error) {
        throw RuntimeError(std::string(helper) + ": malformed JSON: " + error.what());
    }
    if (!duplicate.empty()) {
        throw RuntimeError(std::string(helper) + ": duplicate key '" + duplicate + "'");
    }
    return document;
}

// A flat object: String and number values only.
void check_flat_object(const json& object, const char* helper, const std::string& where) {
    if (!object.is_object()) {
        throw RuntimeError(std::string(helper) + ": " + where + " is not a JSON object");
    }
    for (const auto& [key, value] : object.items()) {
        if (!value.is_string() && !value.is_number()) {
            throw RuntimeError(std::string(helper) + ": field '" + key + "' in " + where +
                               " must be a String or a number");
        }
    }
}

json parse_array(const std::string& source, const char* helper) {
    json document = parse(source, helper);
    if (!document.is_array()) {
        throw RuntimeError(std::string(helper) + ": root is not a JSON array");
    }
    if (document.size() > kMaxArrayElements) {
        throw RuntimeError(std::string(helper) + ": array has more than " +
                           std::to_string(kMaxArrayElements) + " elements");
    }
    for (std::size_t i = 0; i < document.size(); ++i) {
        check_flat_object(document[i], helper, "element " + std::to_string(i));
    }
    return document;
}

const json& field(const json& object, const std::string& key, const char* helper) {
    const auto it = object.find(key);
    if (it == object.end()) {
        throw RuntimeError(std::string(helper) + ": missing key '" + key + "'");
    }
    return *it;
}

json parse_object(const std::string& source, const char* helper) {
    json document = parse(source, helper);
    check_flat_object(document, helper, "root");
    return document;
}

Value checked_string_result(std::string text, const char* helper) {
    check_size(text.size(), helper, "result");
    return Value::string_value_of(std::move(text));
}

} // namespace

Value json_array_length(Runtime&, std::ostream&, const std::vector<Value>& args) {
    const json array = parse_array(string_arg(args, 0, "json.arrayLength"), "json.arrayLength");
    return Value::int_value_of(static_cast<std::int64_t>(array.size()));
}

Value json_array_object(Runtime&, std::ostream&, const std::vector<Value>& args) {
    const char* helper = "json.arrayObject";
    const json array = parse_array(string_arg(args, 0, helper), helper);
    if (args[1].kind != ValueKind::Int) {
        throw RuntimeError(std::string(helper) + ": argument 2 must be an Int");
    }
    const std::int64_t index = args[1].int_value;
    if (index < 0 || static_cast<std::uint64_t>(index) >= array.size()) {
        throw RuntimeError(std::string(helper) + ": index " + std::to_string(index) +
                           " is outside [0, " + std::to_string(array.size()) + ")");
    }
    return checked_string_result(array[static_cast<std::size_t>(index)].dump(), helper);
}

Value json_get_int(Runtime&, std::ostream&, const std::vector<Value>& args) {
    const char* helper = "json.getInt";
    const json object = parse_object(string_arg(args, 0, helper), helper);
    const std::string& key = string_arg(args, 1, helper);
    const json& value = field(object, key, helper);
    if (value.is_number_unsigned()) {
        const auto number = value.get<std::uint64_t>();
        if (number > static_cast<std::uint64_t>(std::numeric_limits<std::int64_t>::max())) {
            throw RuntimeError(std::string(helper) + ": key '" + key + "' overflows Int");
        }
        return Value::int_value_of(static_cast<std::int64_t>(number));
    }
    if (value.is_number_integer()) {
        return Value::int_value_of(value.get<std::int64_t>());
    }
    throw RuntimeError(std::string(helper) + ": key '" + key + "' is not an integer");
}

Value json_get_float(Runtime&, std::ostream&, const std::vector<Value>& args) {
    const char* helper = "json.getFloat";
    const json object = parse_object(string_arg(args, 0, helper), helper);
    const std::string& key = string_arg(args, 1, helper);
    const json& value = field(object, key, helper);
    double number = 0;
    if (value.is_number_unsigned()) {
        number = static_cast<double>(value.get<std::uint64_t>());
    } else if (value.is_number_integer()) {
        number = static_cast<double>(value.get<std::int64_t>());
    } else if (value.is_number_float()) {
        number = value.get<double>();
    } else {
        throw RuntimeError(std::string(helper) + ": key '" + key + "' is not a number");
    }
    if (!std::isfinite(number)) {
        throw RuntimeError(std::string(helper) + ": key '" + key + "' is not finite");
    }
    return Value::float_value_of(number);
}

Value json_get_string(Runtime&, std::ostream&, const std::vector<Value>& args) {
    const char* helper = "json.getString";
    const json object = parse_object(string_arg(args, 0, helper), helper);
    const std::string& key = string_arg(args, 1, helper);
    const json& value = field(object, key, helper);
    if (!value.is_string()) {
        throw RuntimeError(std::string(helper) + ": key '" + key + "' is not a String");
    }
    return Value::string_value_of(value.get<std::string>());
}

Value json_quote(Runtime&, std::ostream&, const std::vector<Value>& args) {
    const char* helper = "json.quote";
    const std::string& text = string_arg(args, 0, helper);
    check_size(text.size(), helper, "input");
    std::string quoted;
    try {
        quoted = json(text).dump();
    } catch (const json::exception&) {
        throw RuntimeError(std::string(helper) + ": argument is not valid UTF-8");
    }
    return checked_string_result(std::move(quoted), helper);
}

Value text_concat(Runtime&, std::ostream&, const std::vector<Value>& args) {
    const char* helper = "text.concat";
    const std::string& left = string_arg(args, 0, helper);
    const std::string& right = string_arg(args, 1, helper);
    // Checked before allocating the result.
    check_size(left.size() + right.size(), helper, "result");
    return Value::string_value_of(left + right);
}

Value text_from_int(Runtime&, std::ostream&, const std::vector<Value>& args) {
    if (args[0].kind != ValueKind::Int) {
        throw RuntimeError("text.fromInt: argument 1 must be an Int");
    }
    return Value::string_value_of(std::to_string(args[0].int_value));
}

Value text_from_float(Runtime&, std::ostream&, const std::vector<Value>& args) {
    if (args[0].kind != ValueKind::Float) {
        throw RuntimeError("text.fromFloat: argument 1 must be a Float");
    }
    if (!std::isfinite(args[0].float_value)) {
        throw RuntimeError("text.fromFloat: argument 1 must be finite");
    }
    // Shortest round-trip decimal, locale-independent, always a JSON number.
    return Value::string_value_of(json(args[0].float_value).dump());
}

} // namespace tacos::runtime::detail
