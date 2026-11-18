// Acceptance vectors of docs/data-contract-review.md for the json.* / text.*
// Runtime helpers. The tacc -> tacvm path is covered by tests/data-tc.

#include "tacos/runtime/runtime.hpp"

#include <cmath>
#include <cstdint>
#include <cstring>
#include <iostream>
#include <limits>
#include <sstream>
#include <stdexcept>
#include <string>
#include <thread>
#include <vector>

using tacos::runtime::Runtime;
using tacos::runtime::RuntimeError;
using tacos::runtime::Value;
using tacos::runtime::ValueKind;

namespace {

int failures = 0;

void check(bool condition, const std::string& what) {
    if (!condition) {
        std::cerr << "FAIL: " << what << '\n';
        ++failures;
    }
}

Value S(std::string text) { return Value::string_value_of(std::move(text)); }
Value I(std::int64_t number) { return Value::int_value_of(number); }
Value F(double number) { return Value::float_value_of(number); }

// Expects a RuntimeError whose message names the helper and contains `detail`.
void rejects(Runtime& runtime, const std::string& name, const std::vector<Value>& args,
             const std::string& detail, const std::string& what) {
    try {
        runtime.call(name, args);
    } catch (const RuntimeError& error) {
        const std::string message = error.what();
        check(message.rfind(name + ":", 0) == 0 && message.find(detail) != std::string::npos,
              what + " (message: " + message + ")");
        return;
    }
    check(false, what + ": no RuntimeError");
}

std::int64_t get_int(Runtime& r, const std::string& object, const std::string& key) {
    const Value v = r.call("json.getInt", {S(object), S(key)});
    check(v.kind == ValueKind::Int, "getInt returns Int");
    return v.int_value;
}

double get_float(Runtime& r, const std::string& object, const std::string& key) {
    const Value v = r.call("json.getFloat", {S(object), S(key)});
    check(v.kind == ValueKind::Float, "getFloat returns Float");
    return v.float_value;
}

std::string str(Runtime& r, const std::string& name, const std::vector<Value>& args) {
    const Value v = r.call(name, args);
    check(v.kind == ValueKind::String, name + " returns String");
    return v.string_value;
}

void arrays(Runtime& r) {
    check(r.call("json.arrayLength", {S("[]")}).int_value == 0, "empty array");
    check(r.call("json.arrayLength", {S(" [ {\"id\":1} ] ")}).int_value == 1, "one element");
    const std::string two = "[{\"id\":17,\"distance\":10.0},{\"distance\":5,\"id\":3}]";
    check(r.call("json.arrayLength", {S(two)}).int_value == 2, "two elements");
    check(get_int(r, str(r, "json.arrayObject", {S(two), I(0)}), "id") == 17, "order kept, 0");
    check(get_int(r, str(r, "json.arrayObject", {S(two), I(1)}), "id") == 3, "order kept, 1");
    rejects(r, "json.arrayObject", {S(two), I(-1)}, "index -1", "negative index");
    rejects(r, "json.arrayObject", {S(two), I(2)}, "index 2", "index == length");
    rejects(r, "json.arrayLength", {S("{}")}, "not a JSON array", "object root");
    rejects(r, "json.arrayLength", {S("[1]")}, "element 0", "non-object element");
    rejects(r, "json.arrayLength", {S("[{\"a\":{}}]")}, "field 'a'", "nested object");

    std::string many = "[";
    for (int i = 0; i < 73; ++i) many += std::string(i ? "," : "") + "{\"id\":" + std::to_string(i) + "}";
    many += "]";
    rejects(r, "json.arrayLength", {S(many)}, "more than 72", "73 elements");
    const std::string seventy_two = many.substr(0, many.rfind(",{")) + "]";
    check(r.call("json.arrayLength", {S(seventy_two)}).int_value == 72, "72 elements accepted");
}

void objects(Runtime& r) {
    const std::string target =
        "{\"targetId\":\"17\",\"latitude\":59.9505,\"longitude\":30.311,"
        "\"probability\":0.85,\"lastSeenAt\":1800000000100}";
    const std::string reordered =
        " {\"lastSeenAt\":1800000000100,\n\"probability\":0.85,\"longitude\":30.311,"
        "\t\"latitude\":59.9505,\"targetId\":\"17\"} ";
    for (const auto& payload : {target, reordered}) {
        check(str(r, "json.getString", {S(payload), S("targetId")}) == "17", "targetId");
        check(get_float(r, payload, "latitude") == 59.9505, "latitude");
        check(get_float(r, payload, "longitude") == 30.311, "longitude");
        check(get_float(r, payload, "probability") == 0.85, "probability");
        check(get_int(r, payload, "lastSeenAt") == 1800000000100, "lastSeenAt");
    }

    check(get_int(r, "{\"t\":9007199254740993}", "t") == 9007199254740993, "exact above 2^53");
    check(get_int(r, "{\"t\":-9223372036854775808}", "t") == std::numeric_limits<std::int64_t>::min(), "i64 min");
    check(get_int(r, "{\"t\":9223372036854775807}", "t") == std::numeric_limits<std::int64_t>::max(), "i64 max");
    check(get_int(r, "{\"t\":-0}", "t") == 0, "-0 is Int 0");
    check(str(r, "text.fromInt", {I(std::numeric_limits<std::int64_t>::min())}) == "-9223372036854775808", "fromInt min");
    check(str(r, "text.fromInt", {I(std::numeric_limits<std::int64_t>::max())}) == "9223372036854775807", "fromInt max");
    check(str(r, "text.fromInt", {I(17)}) == "17", "fromInt 17");
    rejects(r, "json.getInt", {S("{\"t\":9223372036854775808}"), S("t")}, "overflows", "i64 max + 1");
    rejects(r, "json.getInt", {S("{\"t\":-9223372036854775809}"), S("t")}, "not an integer", "i64 min - 1");
    rejects(r, "json.getInt", {S("{\"id\":17.0}"), S("id")}, "not an integer", "17.0");
    rejects(r, "json.getInt", {S("{\"id\":17e0}"), S("id")}, "not an integer", "17e0");
    rejects(r, "json.getInt", {S("{\"id\":\"17\"}"), S("id")}, "not an integer", "\"17\"");

    check(get_float(r, "{\"v\":1}", "v") == 1.0, "getFloat 1");
    check(get_float(r, "{\"v\":1.25}", "v") == 1.25, "getFloat 1.25");
    check(get_float(r, "{\"v\":1.25e1}", "v") == 12.5, "getFloat 1.25e1");
    check(std::signbit(get_float(r, "{\"v\":-0.0}", "v")), "signed zero kept");
    rejects(r, "json.getFloat", {S("{\"v\":1e9999}"), S("v")}, "malformed JSON", "overflow 1e9999");
    rejects(r, "json.getFloat", {S("{\"v\":\"1\"}"), S("v")}, "not a number", "String for Float");

    rejects(r, "json.getInt", {S("{\"a\":1}"), S("id")}, "missing key 'id'", "missing key");
    rejects(r, "json.getInt", {S("{\"id\":1,\"id\":2}"), S("id")}, "duplicate key 'id'", "duplicate key");
    rejects(r, "json.getString", {S("{\"id\":1}"), S("id")}, "not a String", "wrong type");
    rejects(r, "json.getString", {S("{\"id\":null}"), S("id")}, "field 'id'", "null");
    rejects(r, "json.getString", {S("{\"id\":true}"), S("id")}, "field 'id'", "boolean");
    rejects(r, "json.getString", {S("{\"id\":{\"x\":1}}"), S("id")}, "field 'id'", "nested object");
    rejects(r, "json.getString", {S("{\"id\":\"1\"} x"), S("id")}, "malformed JSON", "trailing bytes");
    rejects(r, "json.getString", {S("{\"id\":\"\\q\"}"), S("id")}, "malformed JSON", "malformed escape");
    rejects(r, "json.getString", {S("{\"id\":\"\\ud800\"}"), S("id")}, "malformed JSON", "unpaired surrogate");
    rejects(r, "json.getString", {S("{\"id\":\"\xff\"}"), S("id")}, "malformed JSON", "invalid UTF-8");
    rejects(r, "json.getString", {S("[]"), S("id")}, "not a JSON object", "array root");
    check(str(r, "json.getString", {S("{\"id\":\"\\u00e9\\n\"}"), S("id")}) == "\xc3\xa9\n", "escapes decoded");
}

void output(Runtime& r) {
    const std::string tricky = "a\"b\\c\nd\re\x01 \xc3\xa9";
    const std::string quoted = str(r, "json.quote", {S(tricky)});
    check(quoted.find('\n') == std::string::npos && quoted.find('\r') == std::string::npos, "no raw CR/LF");
    check(quoted.front() == '"' && quoted.back() == '"', "quoted token");
    check(str(r, "json.getString", {S("{\"v\":" + quoted + "}"), S("v")}) == tricky, "quote round trip");
    rejects(r, "json.quote", {S("\xff")}, "UTF-8", "quote invalid UTF-8");

    check(str(r, "text.concat", {S("ab"), S("")}) == "ab", "concat");
    for (double value : {59.95050012345678, 8.993216059187306e-05, 0.0, 480.0, -1e-300, 1e300}) {
        const std::string text = str(r, "text.fromFloat", {F(value)});
        check(get_float(r, "{\"v\":" + text + "}", "v") == value, "fromFloat round trip " + text);
        check(text.find(',') == std::string::npos, "no locale separator " + text);
    }
    check(str(r, "text.fromFloat", {F(480.0)}) == "480.0", "integral Float stays a Float token");
    check(std::signbit(get_float(r, "{\"v\":" + str(r, "text.fromFloat", {F(-0.0)}) + "}", "v")),
          "-0.0 round trip");
    rejects(r, "text.fromFloat", {I(1)}, "must be a Float", "Int to fromFloat");
    rejects(r, "text.fromInt", {F(1.0)}, "must be an Int", "Float to fromInt");
    rejects(r, "text.concat", {S("a"), I(1)}, "argument 2", "Int to concat");
    rejects(r, "json.getInt", {I(1), S("k")}, "argument 1", "Int source");
}

void limits(Runtime& r) {
    constexpr std::size_t bound = 65536;
    // A valid object of exactly `bound` bytes.
    const std::string prefix = "{\"k\":\"";
    const std::string suffix = "\"}";
    const std::string at_bound = prefix + std::string(bound - prefix.size() - suffix.size(), 'x') + suffix;
    check(at_bound.size() == bound, "fixture size");
    check(str(r, "json.getString", {S(at_bound), S("k")}).size() == bound - 8, "input at bound");
    rejects(r, "json.getString", {S(at_bound + " "), S("k")}, "exceeds 65536", "input above bound");

    check(str(r, "text.concat", {S(std::string(bound - 1, 'a')), S("b")}).size() == bound, "concat at bound");
    rejects(r, "text.concat", {S(std::string(bound, 'a')), S("b")}, "exceeds 65536", "concat above bound");
    rejects(r, "json.quote", {S(std::string(bound / 2, '"'))}, "exceeds 65536", "escape expansion");
}

void independent_runtimes() {
    // Two independent Runtimes in parallel: no shared parser state or String storage.
    auto worker = [](int seed, bool* ok) {
        std::ostringstream sink;
        Runtime r(sink);
        *ok = true;
        for (int i = 0; i < 2000; ++i) {
            const std::string id = std::to_string(seed * 100000 + i);
            const std::string message = str(r, "text.concat",
                {S("{\"targetId\":" + str(r, "json.quote", {S(id)})), S("}")});
            *ok = *ok && str(r, "json.getString", {S(message), S("targetId")}) == id;
        }
    };
    bool first = false;
    bool second = false;
    std::thread a(worker, 1, &first);
    std::thread b(worker, 2, &second);
    a.join();
    b.join();
    check(first && second, "independent Runtimes");
}

} // namespace

int main() {
    std::ostringstream sink;
    Runtime runtime(sink);
    for (const auto& [name, arity] : std::vector<std::pair<std::string, int>>{
             {"json.arrayLength", 1}, {"json.arrayObject", 2}, {"json.getInt", 2},
             {"json.getFloat", 2}, {"json.getString", 2}, {"json.quote", 1},
             {"text.concat", 2}, {"text.fromInt", 1}, {"text.fromFloat", 1}}) {
        check(runtime.has_function(name, static_cast<std::uint8_t>(arity)), name + " registered");
        check(!runtime.has_function(name, static_cast<std::uint8_t>(arity + 1)), name + " arity");
    }
    arrays(runtime);
    objects(runtime);
    output(runtime);
    limits(runtime);
    independent_runtimes();
    check(sink.str().empty(), "helpers do no I/O");

    if (failures != 0) {
        std::cerr << failures << " data helper check(s) failed\n";
        return 1;
    }
    std::cout << "Runtime data helper tests passed\n";
    return 0;
}
