# tacc — `.tc` compiler

Minimal compiler for the first Compiler → VM integration scenario: compiles a program that
calls a single Runtime function (e.g. `runtime.log("hello")`) into a `.bc` file per
`spec/bytecode.md`.

## Scope (this increment)

`spec/language.md` now defines the full `.tc` grammar (variables, arithmetic, `if`/`while`,
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

## Build

```sh
./gradlew build
```

## Run

```sh
./gradlew run --args="tests/hello/hello.tc tests/hello/out.bc"
```

or, after building a fat/runnable jar:

```sh
java -jar build/libs/tacc-0.1.0.jar tests/hello/hello.tc tests/hello/out.bc
```

Compare the result against the reference fixture byte-for-byte:

```sh
diff tests/hello/out.bc tests/hello/hello.bc && echo OK
```

## Assumptions to confirm before integration testing

With the VM Core owner — `spec/bytecode.md` does not currently specify:

- **Element count encoding** for `constant_pool` and `function_table`. This implementation
  assumes a `u16` count immediately before each list, for consistency with the other
  u16-sized fields already in the format (constant index, function index, `entry_function`).
- **`version` field value.** This implementation emits `1`.

With the Runtime owner — **this one is a potential spec inconsistency, not just a gap:**
`spec/language.md` → "Compilation notes" now states that `CALL_RUNTIME` always leaves exactly
one value on the stack (hence the `POP` after `runtime.log(...)` in the example trace below).
The last version of `spec/runtime-api.md` seen by this compiler said a Void-returning Runtime
function "pushes nothing". If that hasn't changed, the generated `POP` pops the wrong value (or
underflows). Please confirm with the Runtime owner whether `runtime-api.md` was updated to match,
before relying on `tests/hello/hello.bc` for a real VM/Runtime integration test.

Any of the above changing requires updating `BytecodeWriter.kt` / `CodeGen.kt` and regenerating
`tests/hello/hello.bc` (via `tests/hello/generate_reference.py`) together with the corresponding
change on the other side.

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
