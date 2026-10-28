# Bytecode specification

Status: Approved by VM Core

This document defines the contract between the Compiler and the VM: the `.bc` file format and the
minimal instruction set required for the first Compiler → VM integration scenario.

## File format

```text
magic:           4 bytes, ASCII "TACB"
version:         u16          # 1 = legacy; 2 = Float extension
constant_count:  u16
constant_pool:   [Constant; constant_count]
function_count:  u16
function_table:  [Function; function_count]
entry_function:  u16          # index into function_table
```

All multi-byte integers in the file are little-endian.

### Constant

```text
tag: u8 (0 = Int, 1 = String; 2 = Float in version 2 only)
value: varies (Int: i64; String: u32 length + UTF-8 bytes;
               Float: 8-byte IEEE 754 binary64 bit pattern, little-endian)
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

Each value on the operand stack and in a local variable has a 64-bit payload.
The VM tracks its runtime kind separately from the payload.

- `Int` — i64
- `Bool` — represented as `Int` (0 = false, 1 = true). No separate physical type.
- `String` — reference (pointer) to a heap-allocated string object.
- `Float` (version 2) — IEEE 754 binary64 bits, with a distinct runtime kind.

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

## Float extension for bytecode version 2 (proposed in #106)

This is a **contract proposal**, not an assertion that the existing VM or
Compiler implements it. Implementation belongs to issues #107 and #108.

### Compatibility and constant representation

- Version 1 retains its exact existing format, constant tags (`0`, `1`)
  and opcodes (`0x01`–`0x10`).
- Version 2 uses the **same** header, constant-pool and function-table layouts,
  index widths, instruction encoding, calling convention and Runtime API.
  It adds constant tag `2` and seven Float instructions below.
- A v2-capable VM accepts v1 and v2 files. A v1 VM must reject an unsupported
  v2 version rather than attempt to decode it as v1. Unknown versions,
  constants, opcodes and truncated operands are loader errors.
- Compiler emits v1 for source programs with no Float-related constructs;
  emits v2 for a program using any Float literal, variable, function
  parameter, return type, expression or comparison.
- A v1 file containing tag `2` or opcodes `0x11`–`0x17` is invalid, even
  if instructions containing these opcodes cannot be reached.

The Float constant is `tag = 0x02`, followed by the **little-endian
8-byte bit pattern** of IEEE 754 binary64, not a decimal string. Example:

```text
1.5 (Float) = 02 00 00 00 00 00 00 F8 3F
```

The loader rejects non-finite Float constants (NaN, +Infinity and -Infinity).
Finite subnormals and both signs of zero are legal. Compiler decimal-literal
conversion rounds to nearest binary64 value, ties to even.

### New opcodes

All new instructions consist of a single opcode byte and no operands.
Each binary operation pops right operand and then left operand.

| Opcode | Hex | Input kinds | Result |
| --- | --- | --- | --- |
| `FADD` | `0x11` | Float, Float | Float addition |
| `FSUB` | `0x12` | Float, Float | Float subtraction |
| `FMUL` | `0x13` | Float, Float | Float multiplication |
| `FDIV` | `0x14` | Float, Float | Float division |
| `FNEG` | `0x15` | Float | Float negation |
| `FCMP_EQ` | `0x16` | Float, Float | Bool (Int 0/1) equality |
| `FCMP_LT` | `0x17` | Float, Float | Bool (Int 0/1) less-than |

Float comparison lowering:

- `a == b`: `FCMP_EQ`; `a != b`: `FCMP_EQ; NOT`.
- `a < b`: `FCMP_LT`; `a >= b`: `FCMP_LT; NOT`.
- `a > b`: compare `b < a`; `a <= b`: compare `b < a`,
  followed by `NOT`.
- `-a`: `FNEG`.

When lowering reversed comparisons, the compiler must **not** reverse the
evaluation order of expressions with side effects. It can preserve normal
left-to-right evaluation using temporary locals and then load operands in
the required stack order. Existing comparison and unary-operator rules for
Int remain unchanged.

### Runtime and error semantics

Values have a 64-bit payload and an independently tracked kind
(`Int`, `String`, or `Float`; `Bool` remains `Int 0/1`). This is a
logical representation, not a new file field. Float bits are never
interpreted as an integer or String pointer.

`PUSH_CONST`, `LOAD`, `STORE`, `CALL`, `RET`, and `POP` preserve or
discard both payload **and** kind. Float parameters and returns use the
existing function layout without function signature metadata; the compiler
enforces declared types and the VM dynamically checks operand kinds.
Existing `CALL_RUNTIME`, `runtime.log(String)`, Int operations and
String comparison behavior are unchanged.

- Binary64 arithmetic uses round to nearest, ties to even.
- `FCMP_EQ(+0.0, -0.0)` is true.
- Signed zero, subnormal finite results and underflow to zero are legal.
- Division by either signed zero is a VM error (including `0.0 / 0.0`).
- NaN or infinity produced by any Float arithmetic is a VM error.
- Wrong runtime operand kinds or stack underflow are VM errors.
- Errors write a `tacvm:` diagnostic on stderr and exit with a nonzero code.
  They must not corrupt a String pointer or silently reinterpret Float
  bits. This addition does not alter existing Int `DIV` behavior.

### Required acceptance vectors

| Case | Expected behavior |
| --- | --- |
| `1.5 + 2.25` | Float `3.75` |
| `5.0 / 2.0` | Float `2.5` |
| `-2.5 * 2.0` | Float `-5.0` |
| `1.0 < 2.0` | Bool true |
| `3.0 == 3.0` | Bool true |
| `0.0 == -0.0` | Bool true |
| `1 + 2.0` | Compile-time error |
| `let x: Float = 10;` | Compile-time error |
| `1.0 / 0.0` | VM execution error |
| `1.0 / -0.0` | VM execution error |
| `FADD` with an Int operand | VM execution error |
| NaN or infinity Float constant | Loader error |
| Truncated Float constant | Loader error |
| Float opcode in v1 | Loader error |
| Old `hello.bc` on v2 VM | Same output as previously |
| v2 bytecode on legacy VM | Unsupported version |
| Float in local, parameter and `RET` | Payload and kind preserved |

An end-to-end test must compile the `calculateX` source from
`spec/language.md` with the real `tacc` and execute it with the real
`tacvm`. It must print `float-ok`. The Compiler and VM implementation
tests belong in #107/#108; this PR only defines their shared contract.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Bytecode" and "VM":

- heap/object representation and any instructions for it;
- string interning / lifetime strategy;
- compatibility strategy for future versions after proposed Float v2;
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
