# Language specification

Status: approved by the Language and Compiler owner. Compilation notes added following confirmation
from the VM Core owner.

This document defines the minimal syntax of the `.tc` language required for the first Compiler → VM
integration scenario (see `docs/ai-context/02-open-questions.md` → "Language").

## Scope

Only what is needed to compile and run a program that calls one Runtime function. Classes, inheritance,
and a standard library remain open questions and are intentionally not covered here.

## Source files

- Extension: `.tc`.
- A source file contains zero or more function declarations.
- Exactly one function named `main` with no parameters is the program entry point.

## Types

- `Int` — signed integer.
- `Bool` — `true` / `false`.
- `String` — UTF-8 text literal.

No user-defined types (structs/objects/classes) in this minimal version.

## Functions

```text
fn <name>(<param>: <type>, ...) -> <type> {
    <statements>
}
```

A function with no return value omits `-> <type>`.

## Variables

```text
let <name>: <type> = <expression>;
```

Variables are mutable and block-scoped.

## Expressions

- Integer literals: `123`.
- String literals: `"text"`.
- Boolean literals: `true`, `false`.
- Arithmetic: `+ - * /` on `Int`.
- Comparison: `== != < <= > >=`.
- Function call: `name(arg, ...)`.
- Runtime call: `<namespace>.<function>(arg, ...)` (see `spec/runtime-api.md`).

## Statements

- Expression statement: `<expression>;`
- Variable declaration: see above.
- Assignment: `<name> = <expression>;`
- `if (<expr>) { ... } else { ... }` (`else` optional).
- `while (<expr>) { ... }`.
- `return <expression>;` / `return;`

## Example

```text
fn main() {
    runtime.log("hello");
}
```

## Compilation notes

Confirmed with the VM Core owner against `spec/bytecode.md`. These notes describe how language-level
constructs map onto the bytecode format; they do not change the syntax above.

- **Value representation**: every value — `Int`, `Bool`, and `String` — occupies a single 64-bit slot
  in locals and on the operand stack.
- **`Bool` representation**: `Bool` has no distinct runtime type. It is an `Int` slot holding `0`
  (false) or `1` (true), consistent with `CMP_EQ` / `CMP_LT` in `spec/bytecode.md`.
- **`String` representation**: at runtime, a `String` value is a reference (pointer) to a string object
  on the heap. Locals and stack slots holding a `String` hold this reference, not the bytes themselves.
  String constants are resolved to heap objects at load time, per `spec/bytecode.md`.
- **Function frame layout**: on `CALL`, locals `0` to `param_count - 1` hold the function's parameters,
  in declaration order. Locals from `param_count` onward hold the function's own `let`-declared
  variables, in order of declaration.
- **Comparison operators beyond `CMP_EQ` / `CMP_LT`**: `spec/bytecode.md` defines `CMP_EQ` and
  `CMP_LT` directly, plus a `NOT` opcode (pop Int 0/1, push the inverted 0/1) added specifically to
  support lowering the remaining comparison operators. The compiler lowers them as follows:
  - `a > b` compiles as `b < a` (operands swapped, using `CMP_LT`).
  - `a != b` compiles as `CMP_EQ` followed by `NOT`.
  - `a <= b` compiles as `b < a` (`CMP_LT`, operands swapped) followed by `NOT`.
  - `a >= b` compiles as `a < b` (`CMP_LT`) followed by `NOT`.
- **Return values**: every call — to a function in the same file (`CALL`) or to a Runtime function
  (`CALL_RUNTIME`) — leaves exactly one value on the operand stack (see `spec/bytecode.md` and
  `spec/runtime-api.md`). Consequently:
  - a call used as an expression statement (e.g. `runtime.log("hello");`) is followed by `POP`;
  - a function declared without `-> <type>` returns `Int 0`: the compiler emits `PUSH_CONST` of `0`
    followed by `RET` for `return;` and at the end of the function body.

For example, `fn main() { runtime.log("hello"); }` compiles to:

```text
PUSH_CONST   "hello"
CALL_RUNTIME "runtime.log", 1
POP
PUSH_CONST   0
RET
```

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Language":

- minimal type system beyond `Int` / `Bool` / `String`;
- whether classes/structs are required;
- whether inheritance is required;
- exact representation of structs or objects;
- error handling model;
- standard library scope beyond the Runtime API.
