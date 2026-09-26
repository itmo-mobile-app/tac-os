# Bytecode specification

Status: draft — proposed by Architecture & Integration ahead of Sprint 0, pending review and approval by
the VM Core owner (see `docs/ai-context/03-team-ownership.md`) in a follow-up Pull Request.

This document defines the contract between the Compiler and the VM: the `.bc` file format and the
minimal instruction set required for the first Compiler → VM integration scenario.

## File format

```text
magic:            4 bytes, ASCII "TACB"
version:          u16
constant_pool:    [Constant]
function_table:   [Function]
entry_function:   u16 (index into function_table)
```

### Constant

```text
tag: u8            (0 = Int, 1 = String)
value: varies      (Int: i64; String: u32 length + UTF-8 bytes)
```

### Function

```text
name:        u32 length + UTF-8 bytes
param_count: u8
local_count: u8
code_length: u32
code:        [Instruction]
```

## Instruction set

One-byte opcode, followed by a fixed number of operands.

| Opcode | Operands | Meaning |
| --- | --- | --- |
| `PUSH_CONST` | u16 (constant index) | Push constant onto operand stack |
| `LOAD` | u8 (local index) | Push local variable |
| `STORE` | u8 (local index) | Pop into local variable |
| `ADD` / `SUB` / `MUL` / `DIV` | — | Pop 2 ints, push result |
| `CMP_EQ` / `CMP_LT` | — | Pop 2, push `Bool` (as Int 0/1) |
| `JMP` | u32 (absolute offset) | Unconditional jump |
| `JMP_IF_FALSE` | u32 (absolute offset) | Pop `Bool`, jump if false |
| `CALL` | u16 (function index), u8 (arg count) | Call a function in this file |
| `CALL_RUNTIME` | u16 (constant index: function name), u8 (arg count) | Call a Runtime function (see `spec/runtime-api.md`) |
| `RET` | — | Return from current function (top of stack, if any, is the return value) |
| `POP` | — | Discard top of operand stack |

This is the minimal set for the first integration test (arithmetic, one branch, one call, one Runtime
call). Additional instructions are added only when a concrete scenario needs them.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Bytecode":

- heap/object representation and any instructions for it;
- versioning/compatibility strategy beyond the `version` field;
- metadata (debug info, source locations).
