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
    Load = 0x02,
    Store = 0x03,
    Add = 0x04,
    Sub = 0x05,
    Mul = 0x06,
    Div = 0x07,
    CmpEq = 0x08,
    CmpLt = 0x09,
    Jmp = 0x0A,
    JmpIfFalse = 0x0B,
    Call = 0x0C,
    CallRuntime = 0x0D,
    Ret = 0x0E,
    Pop = 0x0F,
    Not = 0x10,
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

    if (program.functions[program.entry_function].param_count != 0) {
        throw VmError("entry function must have no parameters");
    }
    // Decode every function, including unreachable instructions. Jump targets must
    // be instruction boundaries, never operands or the end of the code array.
    for (const auto& function : program.functions) {
        std::vector<bool> boundaries(function.code.size(), false);
        std::vector<std::uint32_t> targets;
        Reader code(function.code);
        while (code.offset() < function.code.size()) {
            const auto offset = code.offset();
            boundaries[offset] = true;
            const auto opcode = static_cast<Opcode>(code.u8("opcode"));
            switch (opcode) {
            case Opcode::PushConst:
                if (code.u16("PUSH_CONST") >= program.constants.size()) {
                    throw VmError("PUSH_CONST constant index is out of range");
                }
                break;
            case Opcode::Load:
            case Opcode::Store:
                if (code.u8("local index") >=
                    static_cast<unsigned>(function.param_count) + function.local_count) {
                    throw VmError("local index is out of range");
                }
                break;
            case Opcode::Jmp:
            case Opcode::JmpIfFalse:
                targets.push_back(code.u32("jump target"));
                break;
            case Opcode::Call: {
                const auto index = code.u16("CALL function");
                const auto argc = code.u8("CALL arguments");
                if (index >= program.functions.size()) {
                    throw VmError("CALL function index is out of range");
                }
                if (argc != program.functions[index].param_count) {
                    throw VmError("CALL argument count mismatch");
                }
                break;
            }
            case Opcode::CallRuntime: {
                const auto index = code.u16("CALL_RUNTIME name");
                const auto argc = code.u8("CALL_RUNTIME arguments");
                if (index >= program.constants.size()) {
                    throw VmError("CALL_RUNTIME constant index is out of range");
                }
                const auto& name = program.constants[index].value;
                if (name.kind != ValueKind::String) {
                    throw VmError("CALL_RUNTIME name is not a String");
                }
                if (!runtime.has_function(name.string_value, argc)) {
                    throw VmError("unknown Runtime function '" + name.string_value + "'");
                }
                break;
            }
            case Opcode::Add:
            case Opcode::Sub:
            case Opcode::Mul:
            case Opcode::Div:
            case Opcode::CmpEq:
            case Opcode::CmpLt:
            case Opcode::Not:
            case Opcode::Ret:
            case Opcode::Pop:
                break;
            default:
                throw VmError("unknown opcode " + hex_byte(static_cast<std::uint8_t>(opcode)) +
                              " at offset " + std::to_string(offset));
            }
        }
        for (const auto target : targets) {
            if (target >= boundaries.size() || !boundaries[target]) {
                throw VmError("invalid jump target " + std::to_string(target));
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
        frames_.push_back(make_frame(program_.entry_function, {}));
        while (!frames_.empty()) {
            auto& frame = frames_.back();
            const auto& function = program_.functions[frame.function];
            if (frame.ip >= function.code.size()) {
                throw VmError("function '" + function.name + "' ended without RET");
            }
            const auto opcode = static_cast<Opcode>(function.code[frame.ip++]);
            // Instructions and operands were validated at load time.
            auto operand = [&](unsigned width) {
                std::uint32_t value = 0;
                for (unsigned i = 0; i < width; ++i) {
                    value |= static_cast<std::uint32_t>(function.code[frame.ip++]) << (8 * i);
                }
                return value;
            };
            switch (opcode) {
            case Opcode::PushConst:
                push(program_.constants[operand(2)].value);
                break;
            case Opcode::Load: {
                const auto index = operand(1);
                if (!frame.initialized[index]) throw VmError("LOAD of uninitialized local");
                push(frame.locals[index]);
                break;
            }
            case Opcode::Store: {
                const auto index = operand(1);
                frame.locals[index] = pop();
                frame.initialized[index] = true;
                break;
            }
            case Opcode::Add:
            case Opcode::Sub:
            case Opcode::Mul:
            case Opcode::Div:
            case Opcode::CmpLt: {
                const auto rhs = integer(pop());
                const auto lhs = integer(pop());
                std::int64_t result = 0;
                bool overflow = false;
                switch (opcode) {
                case Opcode::Add: overflow = __builtin_add_overflow(lhs, rhs, &result); break;
                case Opcode::Sub: overflow = __builtin_sub_overflow(lhs, rhs, &result); break;
                case Opcode::Mul: overflow = __builtin_mul_overflow(lhs, rhs, &result); break;
                case Opcode::Div:
                    if (rhs == 0) throw VmError("division by zero");
                    if (lhs == std::numeric_limits<std::int64_t>::min() && rhs == -1) {
                        overflow = true;
                    } else {
                        result = lhs / rhs;
                    }
                    break;
                case Opcode::CmpLt: result = lhs < rhs; break;
                default: break;
                }
                if (overflow) throw VmError("integer overflow");
                push(Value::int_value_of(result));
                break;
            }
            case Opcode::CmpEq: {
                const auto rhs = pop();
                const auto lhs = pop();
                if (lhs.kind != rhs.kind) throw VmError("CMP_EQ requires matching types");
                const bool equal = lhs.kind == ValueKind::Int
                    ? lhs.int_value == rhs.int_value : lhs.string_value == rhs.string_value;
                push(Value::int_value_of(equal));
                break;
            }
            case Opcode::Not:
                push(Value::int_value_of(!boolean(pop())));
                break;
            case Opcode::Jmp:
                frame.ip = operand(4);
                break;
            case Opcode::JmpIfFalse: {
                const auto target = operand(4);
                if (!boolean(pop())) frame.ip = target;
                break;
            }
            case Opcode::Call: {
                const auto index = operand(2);
                const auto argc = operand(1);
                auto args = arguments(argc);
                if (frames_.size() >= kMaxFrames) throw VmError("call stack overflow");
                frames_.push_back(make_frame(index, args));
                break;
            }
            case Opcode::CallRuntime: {
                const auto index = operand(2);
                const auto argc = operand(1);
                const auto args = arguments(argc);
                push(runtime_.call(program_.constants[index].value.string_value, args));
                break;
            }
            case Opcode::Pop:
                (void)pop();
                break;
            case Opcode::Ret: {
                auto result = pop();
                stack_.resize(frame.base);
                frames_.pop_back();
                if (frames_.empty()) return result;
                push(std::move(result));
                break;
            }
            default:
                throw VmError("invalid decoded opcode");
            }
        }
        throw VmError("missing entry frame");
    }

private:
    // Iterative frames avoid exhausting the host C++ call stack on recursion.
    static constexpr std::size_t kMaxFrames = 1024;
    static constexpr std::size_t kMaxStack = 65536;
    struct Frame {
        std::size_t function;
        std::size_t ip = 0;
        std::size_t base;
        std::vector<Value> locals;
        std::vector<bool> initialized;
    };

    Frame make_frame(std::size_t index, const std::vector<Value>& args) const {
        const auto& fn = program_.functions[index];
        const auto count = static_cast<std::size_t>(fn.param_count) + fn.local_count;
        Frame frame{index, 0, stack_.size(),
                    std::vector<Value>(count, Value::int_value_of(0)),
                    std::vector<bool>(count, false)};
        for (std::size_t i = 0; i < args.size(); ++i) {
            frame.locals[i] = args[i];
            frame.initialized[i] = true;
        }
        return frame;
    }

    void push(Value value) {
        if (stack_.size() >= kMaxStack) throw VmError("operand stack overflow");
        stack_.push_back(std::move(value));
    }

    Value pop() {
        if (stack_.size() <= frames_.back().base) throw VmError("stack underflow");
        auto value = std::move(stack_.back());
        stack_.pop_back();
        return value;
    }

    std::vector<Value> arguments(std::size_t count) {
        if (count > stack_.size() - frames_.back().base) throw VmError("stack underflow in call");
        std::vector<Value> args(count);
        for (std::size_t i = count; i > 0; --i) args[i - 1] = pop();
        return args;
    }

    static std::int64_t integer(const Value& value) {
        if (value.kind != ValueKind::Int) throw VmError("expected Int operand");
        return value.int_value;
    }

    static bool boolean(const Value& value) {
        const auto number = integer(value);
        if (number != 0 && number != 1) throw VmError("expected Bool 0 or 1");
        return number != 0;
    }

    Program program_;
    Runtime& runtime_;
    std::vector<Value> stack_;
    std::vector<Frame> frames_;
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
