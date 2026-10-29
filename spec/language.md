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
- `Float` — IEEE 754 binary64 (proposed extension #106; not yet implemented).

No user-defined types (structs/objects/classes) in this minimal version.

## Syntax style

The syntax follows the part shared by Kotlin and TypeScript: `fun` for functions, `name: Type` for type
annotations, `let` / `const` for variables, and `//` comments. Unlike Kotlin and TypeScript,
semicolons are mandatory.

## Lexical rules

- Comments: `//` starts a comment that runs to the end of the line.
- Every simple statement ends with `;` (see "Statements"). Block statements (`if`, `while`) and function
  declarations end with `}` and take no `;`.
- Newlines are ordinary whitespace: a statement or expression may span several lines.

## Functions

```text
fun <name>(<param>: <type>, ...): <type> {
    <statements>
}
```

- Parameter types are required.
- A function with no return value omits `: <type>` after the parameter list.

## Variables

```text
let <name>[: <type>] = <expression>;
const <name>[: <type>] = <expression>;
```

- `let` declares a mutable variable; `const` declares one that cannot be reassigned (reassignment is a
  compile-time error).
- The initializer is required. The type annotation is optional: without it, the variable takes the type
  of the initializer expression.
- Variables are block-scoped.

## Expressions

- Integer literals: `123` (decimal; negative values are written with unary `-`).
- String literals: `"text"`, with escapes `\"`, `\\`, `\n`.
- Boolean literals: `true`, `false`.
- Arithmetic: `+ - * /` and unary `-` on `Int`, and on `Float` in the proposed extension.
- Comparison: `< <= > >=` on `Int` or `Float`; `== !=` on two values of the same type.
- Logical negation: unary `!` on `Bool`.
- Parentheses: `( <expression> )`.
- Function call: `name(arg, ...)`.
- Runtime call: `<namespace>.<function>(arg, ...)` (see `spec/runtime-api.md`).

Operator precedence and associativity are defined in "Grammar".

## Statements

- Call statement: `<call>;` — only a function or Runtime call may be used as a statement on its own.
- Variable declaration: see above.
- Assignment: `<name> = <expression>;` (only for `let` variables).
- `if (<expr>) { ... } else { ... }` (`else` optional; `else if` chains are allowed).
- `while (<expr>) { ... }`.
- `return <expression>;` / `return;`

## Grammar

The grammar below (EBNF) is authoritative; the sections above explain it. `{ x }` means zero or more,
`[ x ]` means optional, quoted text is a literal token.

```text
program      = { function } ;
function     = "fun" ident "(" [ param { "," param } ] ")" [ ":" type ] block ;
param        = ident ":" type ;
type         = "Int" | "Bool" | "String" | "Float" ;

block        = "{" { statement } "}" ;
statement    = var_decl | assignment | if_stmt | while_stmt | return_stmt | call_stmt ;
var_decl     = ( "let" | "const" ) ident [ ":" type ] "=" expression ";" ;
assignment   = ident "=" expression ";" ;
if_stmt      = "if" "(" expression ")" block [ "else" ( if_stmt | block ) ] ;
while_stmt   = "while" "(" expression ")" block ;
return_stmt  = "return" [ expression ] ";" ;
call_stmt    = call ";" ;

expression   = equality ;
equality     = relational [ ( "==" | "!=" ) relational ] ;
relational   = additive [ ( "<" | "<=" | ">" | ">=" ) additive ] ;
additive     = term { ( "+" | "-" ) term } ;
term         = unary { ( "*" | "/" ) unary } ;
unary        = ( "-" | "!" ) unary | primary ;
primary      = int_lit | float_lit | string_lit | "true" | "false" | call | ident | "(" expression ")" ;
call         = ident [ "." ident ] "(" [ expression { "," expression } ] ")" ;
```

Lexical tokens:

```text
ident        = ( letter | "_" ) { letter | digit | "_" } ;   (* not a keyword *)
int_lit      = digit { digit } ;                            (* must fit in a signed 64-bit integer *)
float_lit    = digit { digit } "." digit { digit } ;          (* finite binary64 *)
string_lit   = '"' { string_char | escape } '"' ;           (* no raw newlines inside *)
string_char  = ? any character except '"', "\" and newline ? ;
escape       = "\" ( '"' | "\" | "n" ) ;
letter       = "a" … "z" | "A" … "Z" ;
digit        = "0" … "9" ;
```

- Keywords: `fun`, `let`, `const`, `if`, `else`, `while`, `return`, `true`, `false`, `Int`, `Bool`,
  `String`, `Float`.
- Whitespace (space, tab, `\r`, `\n`) and `//` comments may appear between any two tokens and are
  otherwise ignored.
- Binary operators within one precedence level are left-associative. Comparison and equality operators
  do not chain: `a < b < c` and `a == b == c` are syntax errors.
- Precedence, from lowest to highest: equality; comparison; `+` `-`; `*` `/`; unary `-` `!`; calls and
  parentheses.

## Examples

```text
// hello world
fun main() {
    runtime.log("hello");
}
```

```text
fun add(a: Int, b: Int): Int {
    return a + b;
}

fun main() {
    let count = 0;            // mutable, type inferred as Int
    const limit: Int = 3;     // immutable, explicit type

    while (count < limit) {
        count = add(count, 1);
    }

    if (count == limit) {
        runtime.log("done");
    } else {
        runtime.log("unexpected");
    }
}
```

## Semantic rules

Checked by the compiler; a program that breaks any of them is rejected with a compile-time error.

- **Types**: `+ - * /`, unary `-`, and `< <= > >=` accept `Int`, or `Float`
  in the proposed extension, but both operands must have the same type.
  `!` takes `Bool`; `==` and `!=` require matching types. There are no
  implicit conversions, particularly between `Int` and `Float`.
  Conditions of `if` and `while` must be `Bool`.
- **Variables**: an annotated type must equal the type of the initializer; an assignment must keep
  the variable's type. Only `let` variables can be assigned; `const` variables and function
  parameters cannot.
- **Scopes**: every `{ ... }` of `if`, `else`, and `while` opens a new scope; a variable is visible from
  its declaration to the end of its block. A name cannot be declared twice in the same scope.
  Function parameters and the top level of the function body are one scope. A variable in a nested
  block may shadow a variable of an enclosing block. A variable is not visible in its own
  initializer.
- **Functions**: function names are unique; a function can be called before its declaration. A call
  must pass as many arguments as the function has parameters, with matching types.
- **Return values**: a function with `: <type>` returns a value of that type, and every path through
  its body must end in `return <expression>;` (a `while` body does not count, since it may not run).
  A function without `: <type>` uses only `return;`, and a call to it is a statement, not a value.
- **Runtime calls**: the compiler accepts only Runtime functions listed in `spec/runtime-api.md`, with
  their argument types. `runtime.log(message: String)` has no result usable as a value.
- **Limits** (from `spec/bytecode.md`): at most 255 parameters and 255 `let`/`const` variables per
  function, and at most 65 536 distinct constants and functions per file.

## Compilation notes

Confirmed with the VM Core owner against `spec/bytecode.md`. These notes describe how language-level
constructs map onto the bytecode format; they do not change the syntax above.

- **Value representation**: each `Int`, `Bool`, `String` or proposed `Float`
  has a 64-bit payload; the VM tracks its runtime kind separately.
- **`Bool` representation**: `Bool` has no distinct runtime type. It is an `Int` slot holding `0`
  (false) or `1` (true), consistent with `CMP_EQ` / `CMP_LT` in `spec/bytecode.md`.
- **`String` representation**: at runtime, a `String` value is a reference (pointer) to a string object
  on the heap. Locals and stack slots holding a `String` hold this reference, not the bytes themselves.
  String constants are resolved to heap objects at load time, per `spec/bytecode.md`.
- **Function frame layout**: on `CALL`, locals `0` to `param_count - 1` hold the function's parameters,
  in declaration order. Locals from `param_count` onward hold the function's own `let`- and
  `const`-declared variables, in order of declaration.
- **Comparison operators beyond `CMP_EQ` / `CMP_LT`**: `spec/bytecode.md` defines `CMP_EQ` and
  `CMP_LT` directly, plus a `NOT` opcode (pop Int 0/1, push the inverted 0/1) added specifically to
  support lowering the remaining comparison operators. The compiler lowers them as follows:
  - `a > b` compiles as `b < a` (operands swapped, using `CMP_LT`).
  - `a != b` compiles as `CMP_EQ` followed by `NOT`.
  - `a <= b` compiles as `b < a` (`CMP_LT`, operands swapped) followed by `NOT`.
  - `a >= b` compiles as `a < b` (`CMP_LT`) followed by `NOT`.
- **Unary operators**: `!x` compiles as `x` followed by `NOT`; Int `-x`
  compiles as `PUSH_CONST` of `0`, then `x`, then `SUB`; Float `-x`
  compiles as `x` followed by `FNEG`. Float comparisons use `FCMP_EQ`,
  `FCMP_LT` and existing `NOT` as specified in `spec/bytecode.md`.
- **Return values**: every call — to a function in the same file (`CALL`) or to a Runtime function
  (`CALL_RUNTIME`) — leaves exactly one value on the operand stack (see `spec/bytecode.md` and
  `spec/runtime-api.md`). Consequently:
  - a call used as an expression statement (e.g. `runtime.log("hello");`) is followed by `POP`;
  - a function declared without `: <type>` after its parameter list returns `Int 0`: the compiler emits
    `PUSH_CONST` of `0` followed by `RET` for `return;` and at the end of the function body.

For example, `fun main() { runtime.log("hello"); }` compiles to:

```text
PUSH_CONST   "hello"
CALL_RUNTIME "runtime.log", 1
POP
PUSH_CONST   0
RET
```

## Float extension (proposed in #106)

This is a **contract proposal**, not an implemented feature. Compiler #107 and
VM #108 will implement it after review.

- The source type `Float` is IEEE 754 binary64. Decimal literals have at
  least one digit on either side of the dot: `0.0`, `12.5`, `960.0`.
  Negative literals use the existing unary minus.
- Numeric literal conversion uses correctly rounded binary64, round to nearest
  with ties to even. Overflow to infinity is a compile-time error. Finite
  subnormals and underflow to zero are permitted.
- `1` is `Int`; `1.0` is `Float`. No implicit conversion between them
  occurs in expressions, assignments, function arguments or returns.
- Float supports `+ - * /`, unary `-`, `< <= > >= == !=`,
  `let`, `const`, parameters and function return values. Both operands
  of a Float binary operation must be Float, and comparisons yield `Bool`.
- The compiler generates the new Float opcodes of `spec/bytecode.md` and
  retains all existing rules for `Int`, `Bool`, `String` and Runtime calls.
- Scientific notation (`1e3`), leading/trailing-dot literals (`.5`,
  `5.`), casts, arrays, JSON, trigonometry and new Runtime APIs are excluded.

The grammar additions are `Float` in `type` and the keyword list,
`float_lit` in `primary`, and this lexical rule:

```text
float_lit = digit { digit } "." digit { digit } ;
```

A complete example for Map coordinate calculation:

```text
fun calculateX(longitude: Float, lonMin: Float,
               lonMax: Float, width: Float): Float {
    return (longitude - lonMin) / (lonMax - lonMin) * width;
}

fun main() {
    let x: Float = calculateX(30.31, 30.308, 30.314, 960.0);
    if (x >= 319.0) {
        if (x <= 321.0) {
            runtime.log("float-ok");
        }
    }
}
```

The expected output is `float-ok` (x is approximately 320.0).
A compliant compiler rejects each of `let x: Float = 1;`,
`let x = 1 + 2.0;`, `let x = 1.0 < 2;`, and
`let x: Int = 1.0;`. VM arithmetic errors are specified in
`spec/bytecode.md`.

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Language":

- additional type-system features beyond `Int` / `Bool` / `String` / proposed `Float`;
- whether classes/structs are required;
- whether inheritance is required;
- exact representation of structs or objects;
- error handling model;
- standard library scope beyond the Runtime API.
