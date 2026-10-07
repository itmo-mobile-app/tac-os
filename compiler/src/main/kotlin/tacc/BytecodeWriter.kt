package tacc

import java.io.ByteArrayOutputStream

/**
 * Binary `.bc` writer per spec/bytecode.md (approved by VM Core).
 *
 * ASSUMPTION — flag with VM Core owner before relying on this for real
 * integration: spec/bytecode.md does not currently state how the element
 * count of `constant_pool` and `function_table` is encoded. This writer
 * assumes a `u16` count immediately before each list, for consistency with
 * the other u16-sized index/count fields already in the format (constant
 * index, function index, `entry_function`). If the VM Core owner specifies
 * a different encoding (e.g. u32, or no explicit count), this writer and
 * the corresponding VM loader must be updated together.
 *
 * ASSUMPTION — `version` is not given a concrete value in spec/bytecode.md;
 * this writer emits `1`.
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
