package tacc

class CodeGenError(message: String, val line: Int) : Exception("line $line: $message")

/**
 * Lowers the AST to a [BytecodeModule].
 *
 * Per spec/language.md -> "Compilation notes" -> "Return values" and spec/runtime-api.md: every
 * call, including CALL_RUNTIME, leaves exactly one value on the operand stack (a Runtime function
 * that conceptually returns nothing, such as `runtime.log`, pushes `Int 0`), so a call used as a
 * statement is followed by POP. A function with no declared return type (every function in this
 * increment, since return types are not parsed yet) implicitly returns `Int 0`: the compiler
 * emits `PUSH_CONST 0` followed by `RET` at the end of the function body.
 */
class CodeGen {
    fun generate(program: Program): BytecodeModule {
        val pool = ConstantPool()
        val functions = mutableListOf<CompiledFunction>()
        var entryFunction = -1

        for (fn in program.functions) {
            val code = CodeBuilder()
            for (call in fn.body) {
                if (call.args.size != 1) {
                    throw CodeGenError(
                        "only single-argument Runtime calls are supported in this minimal version",
                        call.line,
                    )
                }
                val argIndex = pool.stringConstant(call.args[0])
                val fnNameIndex = pool.stringConstant("${call.namespace}.${call.function}")
                code.pushConst(argIndex)
                code.callRuntime(fnNameIndex, call.args.size)
                code.pop() // discard the one value every call leaves on the stack (statement context)
            }
            // Implicit "return 0" for a function with no declared return type.
            val zeroIndex = pool.intConstant(0)
            code.pushConst(zeroIndex)
            code.ret()

            if (fn.name == "main") entryFunction = functions.size
            functions.add(CompiledFunction(fn.name, paramCount = 0, localCount = 0, code = code.toByteArray()))
        }

        check(entryFunction >= 0) { "internal error: 'main' presence should have been checked by the parser" }
        return BytecodeModule(pool.all(), functions, entryFunction)
    }
}
