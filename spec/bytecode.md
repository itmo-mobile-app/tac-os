# Bytecode specification

Status: Approved by VM Core

This document defines the contract between the Compiler and the VM: the `.bc` file format and the
minimal instruction set required for the first Compiler → VM integration scenario.

## File format

```text
magic:           4 bytes, ASCII "TACB"
version:         u16          # current version = 1
constant_count:  u16
constant_pool:   [Constant; constant_count]
function_count:  u16
function_table:  [Function; function_count]
entry_function:  u16          # index into function_table
```

All multi-byte integers in the file are little-endian.

### Constant

```text
tag: u8 (0 = Int, 1 = String)
value: varies (Int: i64; String: u32 length + UTF-8 bytes)
```

### Function

```text
name:         u32 length + UTF-8 bytes
param_count:  u8
local_count:  u8              # number of *additional* locals (not including parameters)
code_length:  u32             # length of the code section in bytes
code:         [Instruction]   # exactly code_length bytes
```

### Calling convention

When a function is called:

- Arguments are placed into locals `[0 .. param_count-1]`
- Additional local variables occupy locals `[param_count .. param_count + local_count - 1]`
- The total number of local slots required by the function is `param_count + local_count`

Every function returns exactly one value. Void-like functions push `Int 0` before `RET`.

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
| `CMP_LT`       | 0x09  | —                                                   | Pop 2 ints, push `Bool` (as Int 0/1)                                     |
| `JMP`          | 0x0A  | u32 (absolute offset)                               | Unconditional jump                                                       |
| `JMP_IF_FALSE` | 0x0B  | u32 (absolute offset)                               | Pop `Bool`, jump if false                                                |
| `CALL`         | 0x0C  | u16 (function index), u8 (arg count)                | Call a function in this file                                             |
| `CALL_RUNTIME` | 0x0D  | u16 (constant index: function name), u8 (arg count) | Call a Runtime function (see `spec/runtime-api.md`)                      |
| `RET`          | 0x0E  | —                                                   | Return from current function (top of stack is the return value)          |
| `POP`          | 0x0F  | —                                                   | Discard top of operand stack                                             |
| `NOT`          | 0x10  | —                                                   | Pop Int 0/1, push the inverted 0/1                                       |

Notes:

- `JMP` / `JMP_IF_FALSE`: the `u32` offset is absolute from the beginning of the current function's `code` array.
- Every function returns exactly one value. Functions that conceptually return nothing must push `Int 0` before `RET`.
- `CMP_EQ` compares `Int` values numerically and `String` values by content.
- Behaviour of `DIV` when the divisor is zero is left open (see Open questions).

This is the minimal set for the first integration test (arithmetic, one branch, one call, one Runtime
call). Additional instructions are added only when a concrete scenario needs them.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Bytecode" and "VM":

- heap/object representation and any instructions for it;
- string interning / lifetime strategy;
- versioning/compatibility strategy beyond the `version` field;
- metadata (debug info, source locations);
- stack overflow / underflow behaviour;
- behaviour of `DIV` when the divisor is zero.

## VM safety behavior (proposed in issue #62; Compiler owner review pending)

The implementation in #62 uses the following explicit failure behavior. These rules
are proposed for joint Compiler/VM review, not recorded as owner-approved decisions.
The file format and opcode values remain unchanged.

- Binary instructions pop the right operand first, then the left operand.
- Signed integer division truncates toward zero. Division by zero and signed i64
  overflow (including `INT64_MIN / -1`) terminate execution with a VM error.
- Arithmetic and `CMP_LT` require Int operands. `CMP_EQ` requires matching runtime
  kinds and compares strings by content. `NOT` and `JMP_IF_FALSE` require Int 0/1.
- Each call has independent locals and an operand-stack base. Arguments are consumed
  in declaration order; a callee cannot pop caller operands. `RET` consumes the
  result, discards any remaining callee operands and pushes one result to the caller.
- Entry functions must take no parameters. Calls must match `param_count`.
  Reading an additional local before `STORE` is an error.
- The loader checks all instructions, even unreachable code: operand completeness,
  constant/local/function indices, call arity and Runtime function resolution.
  Jump targets must point to an instruction in the same function, never an operand
  byte or `code_length`. Falling through the end of a function is an execution error.
- Stack underflow, exceeding 65,536 operand values or 1,024 active function frames
  terminates execution with a VM error. These limits are implementation limits,
  not additional fields in the bytecode format.
- Errors produce a `tacvm:` diagnostic on stderr and a nonzero exit status.
