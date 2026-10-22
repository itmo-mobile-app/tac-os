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
    const val NOT: Int = 0x10
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

/** Emits the code of one function. Multi-byte operands are little-endian (spec/bytecode.md). */
class CodeBuilder {
    private val bytes = mutableListOf<Byte>()

    /** Current offset from the start of the function's code: the target of a jump emitted next. */
    val offset: Int get() = bytes.size

    fun op(opcode: Int): CodeBuilder {
        bytes.add(opcode.toByte())
        return this
    }

    fun pushConst(constIndex: Int): CodeBuilder = op(Opcode.PUSH_CONST).u16(constIndex)

    fun load(slot: Int): CodeBuilder = op(Opcode.LOAD).u8(slot)

    fun store(slot: Int): CodeBuilder = op(Opcode.STORE).u8(slot)

    fun call(functionIndex: Int, argCount: Int): CodeBuilder = op(Opcode.CALL).u16(functionIndex).u8(argCount)

    fun callRuntime(nameConstIndex: Int, argCount: Int): CodeBuilder =
        op(Opcode.CALL_RUNTIME).u16(nameConstIndex).u8(argCount)

    /** Jump to an already known (backward) [target]. */
    fun jump(opcode: Int, target: Int): CodeBuilder = op(opcode).u32(target)

    /**
     * Forward jump whose target is not known yet: emits a placeholder offset and returns its
     * position for [patch].
     */
    fun jumpForward(opcode: Int): Int {
        op(opcode)
        val operandPos = bytes.size
        u32(0)
        return operandPos
    }

    /** Writes [target] into the jump operand at [operandPos] returned by [jumpForward]. */
    fun patch(operandPos: Int, target: Int) {
        for (i in 0 until 4) bytes[operandPos + i] = ((target shr (8 * i)) and 0xFF).toByte()
    }

    private fun u8(value: Int): CodeBuilder {
        bytes.add(value.toByte())
        return this
    }

    private fun u16(value: Int): CodeBuilder {
        bytes.add((value and 0xFF).toByte())
        bytes.add(((value shr 8) and 0xFF).toByte())
        return this
    }

    private fun u32(value: Int): CodeBuilder {
        for (i in 0 until 4) bytes.add(((value shr (8 * i)) and 0xFF).toByte())
        return this
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
