package tacc

import kotlin.test.assertFailsWith

/**
 * Test helpers: compile from a string, read back the `.bc` produced by [compile], disassemble
 * function code, and execute a module with a small reference interpreter.
 *
 * The interpreter is a test oracle for the compiler only. It follows spec/bytecode.md so that
 * compiled control flow can be checked before the VM supports these instructions (#62); it is
 * not a replacement for tacvm and is not used outside tests.
 */

fun compileError(source: String): CompileError = assertFailsWith<CompileError> { compile(source) }

/** Compiles [source] and returns everything the program writes with `runtime.log`. */
fun run(source: String): List<String> = TestVm(Module.read(compile(source))).run()

class Module(val constants: List<Any>, val functions: List<Function>, val entry: Int) {
    class Function(val name: String, val paramCount: Int, val localCount: Int, val code: ByteArray)

    companion object {
        fun read(bytes: ByteArray): Module {
            val r = Reader(bytes)
            check(String(r.bytes(4), Charsets.US_ASCII) == "TACB") { "bad magic" }
            check(r.u16() == 1) { "bad version" }
            val constants = List(r.u16()) {
                when (val tag = r.u8()) {
                    0 -> r.i64()
                    1 -> r.string()
                    else -> error("bad constant tag $tag")
                }
            }
            val functions = List(r.u16()) {
                val name = r.string()
                val params = r.u8()
                val locals = r.u8()
                Function(name, params, locals, r.bytes(r.u32()))
            }
            val entry = r.u16()
            check(r.atEnd()) { "trailing bytes" }
            return Module(constants, functions, entry)
        }
    }
}

class Reader(private val bytes: ByteArray, var pos: Int = 0) {
    fun atEnd(): Boolean = pos == bytes.size

    fun u8(): Int = bytes[pos++].toInt() and 0xFF

    fun u16(): Int = u8() or (u8() shl 8)

    fun u32(): Int = u16() or (u16() shl 16)

    fun i64(): Long {
        var v = 0L
        for (i in 0 until 8) v = v or (u8().toLong() shl (8 * i))
        return v
    }

    fun bytes(n: Int): ByteArray = bytes.copyOfRange(pos, pos + n).also { pos += n }

    fun string(): String = String(bytes(u32()), Charsets.UTF_8)
}

private val MNEMONICS = mapOf(
    Opcode.PUSH_CONST to "PUSH_CONST", Opcode.LOAD to "LOAD", Opcode.STORE to "STORE",
    Opcode.ADD to "ADD", Opcode.SUB to "SUB", Opcode.MUL to "MUL", Opcode.DIV to "DIV",
    Opcode.CMP_EQ to "CMP_EQ", Opcode.CMP_LT to "CMP_LT", Opcode.JMP to "JMP",
    Opcode.JMP_IF_FALSE to "JMP_IF_FALSE", Opcode.CALL to "CALL", Opcode.CALL_RUNTIME to "CALL_RUNTIME",
    Opcode.RET to "RET", Opcode.POP to "POP", Opcode.NOT to "NOT",
)

/**
 * One line per instruction: `<offset> <MNEMONIC> <operands>`. Constants are shown by value
 * (strings quoted) so that tests do not depend on constant-pool order.
 */
fun disassemble(module: Module, functionName: String): List<String> {
    val fn = module.functions.first { it.name == functionName }
    val r = Reader(fn.code)
    val lines = mutableListOf<String>()
    while (!r.atEnd()) {
        val offset = r.pos
        val op = r.u8()
        val name = MNEMONICS[op] ?: error("unknown opcode $op at $offset")
        val operands = when (op) {
            Opcode.PUSH_CONST -> " " + show(module.constants[r.u16()])
            Opcode.LOAD, Opcode.STORE -> " ${r.u8()}"
            Opcode.JMP, Opcode.JMP_IF_FALSE -> " ${r.u32()}"
            Opcode.CALL -> " ${module.functions[r.u16()].name}, ${r.u8()}"
            Opcode.CALL_RUNTIME -> " ${module.constants[r.u16()]}, ${r.u8()}"
            else -> ""
        }
        lines.add("$offset $name$operands")
    }
    return lines
}

private fun show(c: Any): String = if (c is String) "\"$c\"" else c.toString()

/** Reference interpreter for tests (see the file comment). */
class TestVm(private val module: Module) {
    private val output = mutableListOf<String>()
    private val stack = ArrayDeque<Any>()

    fun run(): List<String> {
        execute(module.functions[module.entry], emptyList())
        return output
    }

    private fun execute(fn: Module.Function, args: List<Any>): Any {
        check(args.size == fn.paramCount) { "${fn.name}: expected ${fn.paramCount} args" }
        val locals = arrayOfNulls<Any>(fn.paramCount + fn.localCount)
        args.forEachIndexed { i, v -> locals[i] = v }
        val r = Reader(fn.code)
        var steps = 0
        while (true) {
            check(++steps < 1_000_000) { "${fn.name}: step limit exceeded" }
            check(!r.atEnd()) { "${fn.name}: ran off the end of the code without RET" }
            when (val op = r.u8()) {
                Opcode.PUSH_CONST -> stack.addLast(module.constants[r.u16()])
                Opcode.LOAD -> stack.addLast(checkNotNull(locals[r.u8()]) { "read of unset local" })
                Opcode.STORE -> locals[r.u8()] = stack.removeLast()
                Opcode.ADD, Opcode.SUB, Opcode.MUL, Opcode.DIV, Opcode.CMP_LT -> {
                    val b = stack.removeLast() as Long
                    val a = stack.removeLast() as Long
                    stack.addLast(
                        when (op) {
                            Opcode.ADD -> a + b
                            Opcode.SUB -> a - b
                            Opcode.MUL -> a * b
                            Opcode.DIV -> { check(b != 0L) { "division by zero" }; a / b }
                            else -> if (a < b) 1L else 0L
                        },
                    )
                }
                Opcode.CMP_EQ -> {
                    val b = stack.removeLast()
                    val a = stack.removeLast()
                    stack.addLast(if (a == b) 1L else 0L)
                }
                Opcode.NOT -> stack.addLast(1L - (stack.removeLast() as Long))
                Opcode.JMP -> r.pos = jumpTarget(fn, r.u32())
                Opcode.JMP_IF_FALSE -> {
                    val target = jumpTarget(fn, r.u32())
                    if (stack.removeLast() as Long == 0L) r.pos = target
                }
                Opcode.CALL -> {
                    val callee = module.functions[r.u16()]
                    val argc = r.u8()
                    val callArgs = List(argc) { stack.removeLast() }.reversed()
                    stack.addLast(execute(callee, callArgs))
                }
                Opcode.CALL_RUNTIME -> {
                    val name = module.constants[r.u16()]
                    val argc = r.u8()
                    check(name == "runtime.log" && argc == 1) { "unknown Runtime function $name/$argc" }
                    output.add(stack.removeLast() as String)
                    stack.addLast(0L)
                }
                Opcode.RET -> return stack.removeLast()
                Opcode.POP -> stack.removeLast()
                else -> error("unknown opcode $op")
            }
        }
    }

    private fun jumpTarget(fn: Module.Function, target: Int): Int {
        check(target in fn.code.indices) { "${fn.name}: jump target $target is outside the code" }
        return target
    }
}
