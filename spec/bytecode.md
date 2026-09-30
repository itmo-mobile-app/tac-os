# Bytecode specification

Status: Approved by VM Core

This document defines the contract between the Compiler and the VM: the `.bc` file format and the
minimal instruction set required for the first Compiler → VM integration scenario.

## File format

```text
magic: 4 bytes, ASCII "TACB"
version: u16
constant_pool: [Constant]
function_table: [Function]
entry_function: u16 (index into function_table)
```

All multi-byte integers in the file are little-endian.

### Constant

```text
tag: u8 (0 = Int, 1 = String)
value: varies (Int: i64; String: u32 length + UTF-8 bytes)
```

### Function

```text
name: u32 length + UTF-8 bytes
param_count: u8
local_count: u8          # number of *additional* locals (not including parameters)
code_length: u32
code: [Instruction]
```

### Calling convention

When a function is called:

- Arguments are placed into locals `[0 .. param_count-1]`
- Additional local variables occupy locals `[param_count .. param_count + local_count - 1]`
- The total number of local slots required by the function is `param_count + local_count`

## Value representation (runtime)

All values on the operand stack and in local variables occupy a single 64-bit slot.

- `Int` — i64
- `Bool` — represented as `Int` (0 = false, 1 = true). No separate physical type.
- `String` — reference (pointer) to a heap-allocated string object.

Strings from the constant pool are resolved to heap objects at load time.
The exact string object layout and interning strategy are left to the VM implementation
(see Open questions).

## Instruction set

One-byte opcode, followed by a fixed number of operands.

| Opcode         | Value | Operands                                            | Meaning                                                                  |
|----------------|-------|-----------------------------------------------------|--------------------------------------------------------------------------|
| `PUSH_CONST`   | 0x01  | u16 (constant index)                                | Push constant onto operand stack                                         |
| `LOAD`         | 0x02  | u8 (local index)                                    | Push local variable                                                      |
| `STORE`        | 0x03  | u8 (local index)                                    | Pop into local variable                                                  |
| `ADD`          | 0x04  | —                                                   | Pop 2 ints, push result                                                  |
| `SUB`          | 0x05  | —                                                   | Pop 2 ints, push result                                                  |
| `MUL`          | 0x06  | —                                                   | Pop 2 ints, push result                                                  |
| `DIV`          | 0x07  | —                                                   | Pop 2 ints, push result                                                  |
| `CMP_EQ`       | 0x08  | —                                                   | Pop 2, push `Bool` (as Int 0/1)                                          |
| `CMP_LT`       | 0x09  | —                                                   | Pop 2, push `Bool` (as Int 0/1)                                          |
| `JMP`          | 0x0A  | u32 (absolute offset)                               | Unconditional jump                                                       |
| `JMP_IF_FALSE` | 0x0B  | u32 (absolute offset)                               | Pop `Bool`, jump if false                                                |
| `CALL`         | 0x0C  | u16 (function index), u8 (arg count)                | Call a function in this file                                             |
| `CALL_RUNTIME` | 0x0D  | u16 (constant index: function name), u8 (arg count) | Call a Runtime function (see `spec/runtime-api.md`)                      |
| `RET`          | 0x0E  | —                                                   | Return from current function (top of stack, if any, is the return value) |
| `POP`          | 0x0F  | —                                                   | Discard top of operand stack                                             |

Notes:

- `JMP` / `JMP_IF_FALSE`: the `u32` offset is absolute from the beginning of the current function's `code` array.
- A function may return zero or one value.

This is the minimal set for the first integration test (arithmetic, one branch, one call, one Runtime
call). Additional instructions are added only when a concrete scenario needs them.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Bytecode" and "VM":

- heap/object representation and any instructions for it;
- string interning / lifetime strategy;
- versioning/compatibility strategy beyond the `version` field;
- metadata (debug info, source locations);
- stack overflow / underflow behaviour.
```
