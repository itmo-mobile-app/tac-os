package tacc

/**
 * Opcode values per spec/bytecode.md (approved by VM Core).
 */
object Opcode {
    const val PUSH_CONST: Int = 0x01
    const val LOAD: Int = 0x02
    const val STORE: Int = 0x03
    const val ADD: Int = 0x04
    const val SUB: Int = 0x05
    const val MUL: Int = 0x06
    const val DIV: Int = 0x07
    const val CMP_EQ: Int = 0x08
    const val CMP_LT: Int = 0x09
    const val JMP: Int = 0x0A
    const val JMP_IF_FALSE: Int = 0x0B
    const val CALL: Int = 0x0C
    const val CALL_RUNTIME: Int = 0x0D
    const val RET: Int = 0x0E
    const val POP: Int = 0x0F
}

sealed class Constant {
    data class IntConst(val value: Long) : Constant()
    data class StringConst(val value: String) : Constant()
}

/** De-duplicating constant pool builder (same literal value -> same index). */
class ConstantPool {
    private val constants = mutableListOf<Constant>()
    private val stringIndex = mutableMapOf<String, Int>()
    private val intIndex = mutableMapOf<Long, Int>()

    fun stringConstant(value: String): Int =
        stringIndex.getOrPut(value) {
            constants.add(Constant.StringConst(value))
            constants.size - 1
        }

    fun intConstant(value: Long): Int =
        intIndex.getOrPut(value) {
            constants.add(Constant.IntConst(value))
            constants.size - 1
        }

    fun all(): List<Constant> = constants
}

class CodeBuilder {
    private val bytes = mutableListOf<Byte>()

    fun pushConst(constIndex: Int): CodeBuilder {
        bytes.add(Opcode.PUSH_CONST.toByte())
        appendU16(constIndex)
        return this
    }

    fun callRuntime(nameConstIndex: Int, argCount: Int): CodeBuilder {
        bytes.add(Opcode.CALL_RUNTIME.toByte())
        appendU16(nameConstIndex)
        bytes.add(argCount.toByte())
        return this
    }

    fun ret(): CodeBuilder {
        bytes.add(Opcode.RET.toByte())
        return this
    }

    fun pop(): CodeBuilder {
        bytes.add(Opcode.POP.toByte())
        return this
    }

    private fun appendU16(value: Int) {
        bytes.add((value and 0xFF).toByte())
        bytes.add(((value shr 8) and 0xFF).toByte())
    }

    fun toByteArray(): ByteArray = bytes.toByteArray()
}

class CompiledFunction(
    val name: String,
    val paramCount: Int,
    val localCount: Int,
    val code: ByteArray,
)

class BytecodeModule(
    val constants: List<Constant>,
    val functions: List<CompiledFunction>,
    val entryFunction: Int,
)
