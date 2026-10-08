package tacc

import java.io.ByteArrayOutputStream

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
object BytecodeWriter {
    private const val MAGIC = "TACB"
    private const val VERSION: Int = 1

    fun write(module: BytecodeModule): ByteArray {
        val out = ByteArrayOutputStream()
        out.write(MAGIC.toByteArray(Charsets.US_ASCII))
        writeU16(out, VERSION)
        writeConstantPool(out, module.constants)
        writeFunctionTable(out, module.functions)
        writeU16(out, module.entryFunction)
        return out.toByteArray()
    }

    private fun writeConstantPool(out: ByteArrayOutputStream, constants: List<Constant>) {
        writeU16(out, constants.size)
        for (c in constants) {
            when (c) {
                is Constant.IntConst -> {
                    out.write(0) // tag = Int
                    writeI64(out, c.value)
                }
                is Constant.StringConst -> {
                    out.write(1) // tag = String
                    writeU32LengthPrefixedString(out, c.value)
                }
            }
        }
    }

    private fun writeFunctionTable(out: ByteArrayOutputStream, functions: List<CompiledFunction>) {
        writeU16(out, functions.size)
        for (f in functions) {
            writeU32LengthPrefixedString(out, f.name)
            out.write(f.paramCount)
            out.write(f.localCount)
            writeU32(out, f.code.size)
            out.write(f.code)
        }
    }

    private fun writeU32LengthPrefixedString(out: ByteArrayOutputStream, value: String) {
        val bytes = value.toByteArray(Charsets.UTF_8)
        writeU32(out, bytes.size)
        out.write(bytes)
    }

    private fun writeU16(out: ByteArrayOutputStream, value: Int) {
        out.write(value and 0xFF)
        out.write((value shr 8) and 0xFF)
    }

    private fun writeU32(out: ByteArrayOutputStream, value: Int) {
        out.write(value and 0xFF)
        out.write((value shr 8) and 0xFF)
        out.write((value shr 16) and 0xFF)
        out.write((value shr 24) and 0xFF)
    }

    private fun writeI64(out: ByteArrayOutputStream, value: Long) {
        for (i in 0 until 8) {
            out.write(((value shr (8 * i)) and 0xFF).toInt())
        }
    }
}
