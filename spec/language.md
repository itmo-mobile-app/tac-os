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
- Arithmetic: `+ - * /` and unary `-` on `Int`.
- Comparison: `< <= > >=` on `Int`; `== !=` on two values of the same type.
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
type         = "Int" | "Bool" | "String" ;

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
primary      = int_lit | string_lit | "true" | "false" | call | ident | "(" expression ")" ;
call         = ident [ "." ident ] "(" [ expression { "," expression } ] ")" ;
```

Lexical tokens:

```text
ident        = ( letter | "_" ) { letter | digit | "_" } ;   (* not a keyword *)
int_lit      = digit { digit } ;                            (* must fit in a signed 64-bit integer *)
string_lit   = '"' { string_char | escape } '"' ;           (* no raw newlines inside *)
string_char  = ? any character except '"', "\" and newline ? ;
escape       = "\" ( '"' | "\" | "n" ) ;
letter       = "a" … "z" | "A" … "Z" ;
digit        = "0" … "9" ;
```

- Keywords: `fun`, `let`, `const`, `if`, `else`, `while`, `return`, `true`, `false`, `Int`, `Bool`,
  `String`.
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
  in declaration order. Locals from `param_count` onward hold the function's own `let`- and
  `const`-declared variables, in order of declaration.
- **Comparison operators beyond `CMP_EQ` / `CMP_LT`**: `spec/bytecode.md` defines `CMP_EQ` and
  `CMP_LT` directly, plus a `NOT` opcode (pop Int 0/1, push the inverted 0/1) added specifically to
  support lowering the remaining comparison operators. The compiler lowers them as follows:
  - `a > b` compiles as `b < a` (operands swapped, using `CMP_LT`).
  - `a != b` compiles as `CMP_EQ` followed by `NOT`.
  - `a <= b` compiles as `b < a` (`CMP_LT`, operands swapped) followed by `NOT`.
  - `a >= b` compiles as `a < b` (`CMP_LT`) followed by `NOT`.
- **Unary operators**: `!x` compiles as `x` followed by `NOT`; `-x` compiles as `PUSH_CONST` of `0`,
  then `x`, then `SUB`.
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

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "Language":

- minimal type system beyond `Int` / `Bool` / `String`;
- whether classes/structs are required;
- whether inheritance is required;
- exact representation of structs or objects;
- error handling model;
- standard library scope beyond the Runtime API.
