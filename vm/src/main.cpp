#include "tacos/runtime/runtime.hpp"

#include <cstdint>
#include <exception>
#include <fstream>
#include <iomanip>
#include <iostream>
#include <limits>
#include <sstream>
#include <stdexcept>
#include <string>
#include <utility>
#include <vector>

namespace {

using tacos::runtime::Runtime;
using tacos::runtime::Value;
using tacos::runtime::ValueKind;

constexpr std::uint16_t kSupportedVersion = 1;

enum class Opcode : std::uint8_t {
    PushConst = 0x01,
    CallRuntime = 0x0D,
    Ret = 0x0E,
    Pop = 0x0F,
};

class VmError : public std::runtime_error {
public:
    explicit VmError(const std::string& message)
        : std::runtime_error(message) {}
};

struct Constant {
    Value value;
};

struct Function {
    std::string name;
    std::uint8_t param_count = 0;
    std::uint8_t local_count = 0;
    std::vector<std::uint8_t> code;
};

struct Program {
    std::vector<Constant> constants;
    std::vector<Function> functions;
    std::uint16_t entry_function = 0;
};

class Reader {
public:
    explicit Reader(std::vector<std::uint8_t> bytes)
        : bytes_(std::move(bytes)) {}

    std::size_t offset() const {
        return offset_;
    }

    std::uint8_t u8(const std::string& field) {
        require(1, field);
        return bytes_[offset_++];
    }

    std::uint16_t u16(const std::string& field) {
        require(2, field);
        const auto value = static_cast<std::uint16_t>(
            bytes_[offset_] | (bytes_[offset_ + 1] << 8));
        offset_ += 2;
        return value;
    }

    std::uint32_t u32(const std::string& field) {
        require(4, field);
        const auto value = static_cast<std::uint32_t>(bytes_[offset_]) |
                           (static_cast<std::uint32_t>(bytes_[offset_ + 1]) << 8) |
                           (static_cast<std::uint32_t>(bytes_[offset_ + 2]) << 16) |
                           (static_cast<std::uint32_t>(bytes_[offset_ + 3]) << 24);
        offset_ += 4;
        return value;
    }

    std::int64_t i64(const std::string& field) {
        require(8, field);
        std::uint64_t raw = 0;
        for (int shift = 0; shift < 64; shift += 8) {
            raw |= static_cast<std::uint64_t>(bytes_[offset_++]) << shift;
        }
        return static_cast<std::int64_t>(raw);
    }

    std::string string(const std::string& field) {
        const auto length = u32(field + " length");
        require(length, field);
        const auto begin = bytes_.begin() + static_cast<std::ptrdiff_t>(offset_);
        const auto end = begin + static_cast<std::ptrdiff_t>(length);
        offset_ += length;
        return std::string(begin, end);
    }

    std::vector<std::uint8_t> bytes(std::uint32_t length, const std::string& field) {
        require(length, field);
        const auto begin = bytes_.begin() + static_cast<std::ptrdiff_t>(offset_);
        const auto end = begin + static_cast<std::ptrdiff_t>(length);
        offset_ += length;
        return std::vector<std::uint8_t>(begin, end);
    }

    void expect_end() const {
        if (offset_ != bytes_.size()) {
            throw VmError("unexpected trailing bytes at offset " + std::to_string(offset_));
        }
    }

private:
    void require(std::size_t count, const std::string& field) const {
        if (count > bytes_.size() - offset_) {
            throw VmError("unexpected end of file while reading " + field +
                          " at offset " + std::to_string(offset_));
        }
    }

    std::vector<std::uint8_t> bytes_;
    std::size_t offset_ = 0;
};

std::string hex_byte(std::uint8_t value) {
    std::ostringstream out;
    out << "0x" << std::hex << std::uppercase << std::setw(2) << std::setfill('0')
        << static_cast<int>(value);
    return out.str();
}

std::vector<std::uint8_t> read_file(const std::string& path) {
    std::ifstream input(path, std::ios::binary);
    if (!input) {
        throw VmError("cannot open bytecode file: " + path);
    }

    input.seekg(0, std::ios::end);
    const auto size = input.tellg();
    if (size < 0) {
        throw VmError("cannot read bytecode file size: " + path);
    }
    input.seekg(0, std::ios::beg);

    std::vector<std::uint8_t> bytes(static_cast<std::size_t>(size));
    if (!bytes.empty()) {
        input.read(reinterpret_cast<char*>(bytes.data()), size);
        if (!input) {
            throw VmError("cannot read bytecode file: " + path);
        }
    }
    return bytes;
}

Program load_program(const std::string& path, Runtime& runtime) {
    Reader reader(read_file(path));

    const char expected_magic[] = {'T', 'A', 'C', 'B'};
    for (char expected : expected_magic) {
        const auto actual = reader.u8("magic");
        if (actual != static_cast<std::uint8_t>(expected)) {
            throw VmError("invalid bytecode magic at offset " +
                          std::to_string(reader.offset() - 1));
        }
    }

    const auto version = reader.u16("version");
    if (version != kSupportedVersion) {
        throw VmError("unsupported bytecode version " + std::to_string(version));
    }

    Program program;

    const auto constant_count = reader.u16("constant_count");
    program.constants.reserve(constant_count);
    for (std::uint16_t i = 0; i < constant_count; ++i) {
        const auto tag_offset = reader.offset();
        const auto tag = reader.u8("constant tag");
        if (tag == 0) {
            program.constants.push_back({Value::int_value_of(reader.i64("Int constant"))});
        } else if (tag == 1) {
            program.constants.push_back({Value::string_value_of(reader.string("String constant"))});
        } else {
            throw VmError("unknown constant tag " + std::to_string(tag) +
                          " at offset " + std::to_string(tag_offset));
        }
    }

    const auto function_count = reader.u16("function_count");
    program.functions.reserve(function_count);
    for (std::uint16_t i = 0; i < function_count; ++i) {
        Function function;
        function.name = reader.string("function name");
        function.param_count = reader.u8("param_count");
        function.local_count = reader.u8("local_count");
        const auto code_length = reader.u32("code_length");
        function.code = reader.bytes(code_length, "function code");
        program.functions.push_back(std::move(function));
    }

    program.entry_function = reader.u16("entry_function");
    reader.expect_end();

    if (program.entry_function >= program.functions.size()) {
        throw VmError("entry_function index is out of range: " +
                      std::to_string(program.entry_function));
    }

    // Resolve Runtime calls at load time, as required by spec/runtime-api.md.
    for (const auto& function : program.functions) {
        for (std::size_t ip = 0; ip < function.code.size();) {
            const auto opcode_offset = ip;
            const auto opcode = function.code[ip++];
            switch (opcode) {
            case static_cast<std::uint8_t>(Opcode::PushConst):
                if (ip + 2 > function.code.size()) {
                    throw VmError("truncated PUSH_CONST operand at offset " +
                                  std::to_string(opcode_offset));
                }
                ip += 2;
                break;
            case static_cast<std::uint8_t>(Opcode::CallRuntime): {
                if (ip + 3 > function.code.size()) {
                    throw VmError("truncated CALL_RUNTIME operand at offset " +
                                  std::to_string(opcode_offset));
                }
                const auto name_index = static_cast<std::uint16_t>(
                    function.code[ip] | (function.code[ip + 1] << 8));
                const auto arg_count = function.code[ip + 2];
                ip += 3;
                if (name_index >= program.constants.size()) {
                    throw VmError("CALL_RUNTIME constant index is out of range at offset " +
                                  std::to_string(opcode_offset));
                }
                const auto& name = program.constants[name_index].value;
                if (name.kind != ValueKind::String) {
                    throw VmError("CALL_RUNTIME name is not a String at offset " +
                                  std::to_string(opcode_offset));
                }
                if (!runtime.has_function(name.string_value, arg_count)) {
                    throw VmError("unknown Runtime function '" + name.string_value +
                                  "' at offset " + std::to_string(opcode_offset));
                }
                break;
            }
            case static_cast<std::uint8_t>(Opcode::Ret):
            case static_cast<std::uint8_t>(Opcode::Pop):
                break;
            default:
                throw VmError("unknown opcode " + hex_byte(opcode) + " at offset " +
                              std::to_string(opcode_offset));
            }
        }
    }

    return program;
}

class Vm {
public:
    Vm(Program program, Runtime& runtime)
        : program_(std::move(program)), runtime_(runtime) {}

    Value run() {
        return execute(program_.functions[program_.entry_function]);
    }

private:
    Value execute(const Function& function) {
        std::size_t ip = 0;
        while (ip < function.code.size()) {
            const auto opcode_offset = ip;
            const auto opcode = function.code[ip++];
            switch (opcode) {
            case static_cast<std::uint8_t>(Opcode::PushConst): {
                const auto index = read_u16(function, ip, opcode_offset, "PUSH_CONST");
                if (index >= program_.constants.size()) {
                    throw VmError("PUSH_CONST constant index is out of range at offset " +
                                  std::to_string(opcode_offset));
                }
                stack_.push_back(program_.constants[index].value);
                break;
            }
            case static_cast<std::uint8_t>(Opcode::CallRuntime): {
                const auto name_index = read_u16(function, ip, opcode_offset, "CALL_RUNTIME");
                const auto arg_count = read_u8(function, ip, opcode_offset, "CALL_RUNTIME");
                if (name_index >= program_.constants.size()) {
                    throw VmError("CALL_RUNTIME constant index is out of range at offset " +
                                  std::to_string(opcode_offset));
                }
                const auto& name = program_.constants[name_index].value;
                if (name.kind != ValueKind::String) {
                    throw VmError("CALL_RUNTIME name is not a String at offset " +
                                  std::to_string(opcode_offset));
                }
                if (stack_.size() < arg_count) {
                    throw VmError("stack underflow in CALL_RUNTIME at offset " +
                                  std::to_string(opcode_offset));
                }
                std::vector<Value> args(arg_count);
                for (std::size_t i = arg_count; i > 0; --i) {
                    args[i - 1] = stack_.back();
                    stack_.pop_back();
                }
                stack_.push_back(runtime_.call(name.string_value, args));
                break;
            }
            case static_cast<std::uint8_t>(Opcode::Pop):
                if (stack_.empty()) {
                    throw VmError("stack underflow in POP at offset " +
                                  std::to_string(opcode_offset));
                }
                stack_.pop_back();
                break;
            case static_cast<std::uint8_t>(Opcode::Ret):
                if (stack_.empty()) {
                    throw VmError("stack underflow in RET at offset " +
                                  std::to_string(opcode_offset));
                }
                return stack_.back();
            default:
                throw VmError("unknown opcode " + hex_byte(opcode) + " at offset " +
                              std::to_string(opcode_offset));
            }
        }

        throw VmError("function '" + function.name + "' ended without RET");
    }

    static std::uint8_t read_u8(
        const Function& function,
        std::size_t& ip,
        std::size_t opcode_offset,
        const std::string& instruction) {
        if (ip >= function.code.size()) {
            throw VmError("truncated " + instruction + " operand at offset " +
                          std::to_string(opcode_offset));
        }
        return function.code[ip++];
    }

    static std::uint16_t read_u16(
        const Function& function,
        std::size_t& ip,
        std::size_t opcode_offset,
        const std::string& instruction) {
        if (ip + 2 > function.code.size()) {
            throw VmError("truncated " + instruction + " operand at offset " +
                          std::to_string(opcode_offset));
        }
        const auto value = static_cast<std::uint16_t>(
            function.code[ip] | (function.code[ip + 1] << 8));
        ip += 2;
        return value;
    }

    Program program_;
    Runtime& runtime_;
    std::vector<Value> stack_;
};

} // namespace

int main(int argc, char** argv) {
    if (argc != 2) {
        std::cerr << "usage: " << argv[0] << " <program.bc>\n";
        return 2;
    }

    try {
        Runtime runtime(std::cout);
        Vm vm(load_program(argv[1], runtime), runtime);
        (void)vm.run();
        return 0;
    } catch (const std::exception& error) {
        std::cerr << "tacvm: " << error.what() << '\n';
        return 1;
    }
}
