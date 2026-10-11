# tacc — `.tc` compiler

Minimal compiler for the first Compiler → VM integration scenario: compiles a program that
calls a single Runtime function (e.g. `runtime.log("hello")`) into a `.bc` file per
`spec/bytecode.md`.

## Scope (this increment)

`spec/language.md` defines the full `.tc` grammar (variables, arithmetic, `if`/`while`,
parameters, typed return values). This compiler still only implements the strict subset needed
for the first Compiler → VM scenario — the rest is the remainder of Sprint 1:

- One function `main`, no parameters, no declared return type.
- Function body: a sequence of single-argument Runtime calls, `ns.func("text");`.
- No variables, arithmetic, control flow, or calls between `.tc` functions yet.

Per `spec/language.md` → "Compilation notes" → "Return values", every call leaves exactly one
value on the stack, so:

- each statement-level call is followed by `POP`;
- a function with no declared return type implicitly returns `Int 0`, so the compiler emits
  `PUSH_CONST 0` + `RET` at the end of the function body.

## Build and run

Requires JDK 17+. From `compiler/`:

```sh
./gradlew installDist
build/install/tacc/bin/tacc ../tests/hello/hello.tc build/hello.bc
```

On Windows use `.\gradlew.bat installDist` and `build\install\tacc\bin\tacc.bat`.

Usage: `tacc <input.tc> <output.bc>`. Write the output under `build/` (ignored by git), not under
`tests/`: `.bc` files in `tests/` are not ignored.

## Testing

The CTest test `hello_e2e` (see `tests/hello/README.md`) compiles `tests/hello/hello.tc` with
`tacc`, runs the result with `tacvm`, and compares standard output with `expected.txt`. The output
is not compared with `tests/hello/hello.bc` byte for byte: `spec/bytecode.md` does not fix the
order of constant-pool entries.

## Example

Input (`tests/hello/hello.tc`):

```text
fun main() {
    runtime.log("hello");
}
```

Compiles to (per `spec/language.md` → "Compilation notes"):

```text
PUSH_CONST   "hello"
CALL_RUNTIME "runtime.log", 1
POP
PUSH_CONST   0
RET
```

## Error reporting

Lexer, parser, and codegen errors are reported as `<file>:<line>: error: <message>` on stderr,
with exit code 1.
