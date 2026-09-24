# Language specification

Status: draft — proposed by Architecture & Integration ahead of Sprint 0, pending review and approval by
the Language and Compiler owner (see `docs/ai-context/03-team-ownership.md`) in a follow-up Pull Request.

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

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Language":

- whether classes/structs are required;
- error handling model;
- standard library scope beyond the Runtime API.
