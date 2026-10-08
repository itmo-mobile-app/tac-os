# hello world

The first Compiler → VM → Runtime integration scenario.

| File | Purpose |
| --- | --- |
| `hello.tc` | Source program (`spec/language.md`) |
| `hello.bc` | Hand-assembled reference bytecode (`spec/bytecode.md`), 73 bytes |
| `expected.txt` | Expected standard output of the program |

## How the files are used

- VM and Runtime: run the reference `hello.bc` and compare standard output with `expected.txt`. This does
  not depend on the compiler.
- Compiler: compile `hello.tc`, run the result in the VM, and compare standard output with
  `expected.txt`. The compiled file is not compared with `hello.bc` byte for byte: the order of entries
  in the constant pool is not fixed by `spec/bytecode.md`, so a correct compiler may produce different
  bytes.

`spec/runtime-api.md` does not yet say whether `runtime.log` appends a newline. Until it does, the
comparison with `expected.txt` ignores a single trailing newline.

## Layout of `hello.bc`

All multi-byte integers are little-endian.

```text
54 41 43 42                      magic "TACB"
01 00                            version = 1
03 00                            constant_count = 3
  01 05 00 00 00 68 65 6c 6c 6f    [0] String "hello"
  01 0b 00 00 00 72 75 6e 74 69    [1] String "runtime.log"
     6d 65 2e 6c 6f 67
  00 00 00 00 00 00 00 00 00       [2] Int 0
01 00                            function_count = 1
  04 00 00 00 6d 61 69 6e          name "main"
  00                               param_count = 0
  00                               local_count = 0
  0c 00 00 00                      code_length = 12
  01 00 00                         PUSH_CONST   0      ; "hello"
  0d 01 00 01                      CALL_RUNTIME 1, 1   ; runtime.log, 1 argument
  0f                               POP                 ; discard Int 0 returned by runtime.log
  01 02 00                         PUSH_CONST   2      ; Int 0
  0e                               RET
00 00                            entry_function = 0
```
