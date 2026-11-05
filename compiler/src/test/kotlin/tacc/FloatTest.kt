package tacc

import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class FloatTest {
    private val coordinateProgram = """
        fun calculateX(longitude: Float, lonMin: Float, lonMax: Float, width: Float): Float {
            return (longitude - lonMin) / (lonMax - lonMin) * width;
        }
        fun main() {
            let x: Float = calculateX(30.31, 30.308, 30.314, 960.0);
            if (x >= 319.0) {
                if (x <= 321.0) { runtime.log("float-ok"); }
            }
        }
    """.trimIndent()

    @Test
    fun coordinateExampleCompilesAndRuns() {
        val bytes = compile(coordinateProgram)
        assertEquals(2, (bytes[4].toInt() and 255))
        assertEquals(listOf("float-ok"), run(coordinateProgram))
    }

    @Test
    fun legacySourcesRemainV1() {
        val bytes = compile("fun main() { let x = 1 + 2; }")
        assertEquals(1, bytes[4].toInt() and 255)
    }

    @Test
    fun floatConstIsBinary64LittleEndian() {
        val module = compile("fun main() { let a = 1.5; }")
        // First constant: pool count at 6..7, tag at 8, payload at 9..16.
        assertEquals(2, module[8].toInt() and 255)
        assertContentEquals(
            byteArrayOf(0, 0, 0, 0, 0, 0, 0xF8.toByte(), 0x3F),
            module.copyOfRange(9, 17),
        )
    }

    @Test
    fun allFloatArithmeticAndComparisons() {
        val source = """
            fun main() {
                let x: Float = -2.5 * 2.0 + 7.0 / 2.0 - 1.0;
                if (x == -2.5 && false) { }
            }
        """.trimIndent()
        // There is no && operator in the language.
        assertFailsWith<CompileError> { compile(source) }
        val good = """
            fun main() {
                let x: Float = -2.5 * 2.0 + 7.0 / 2.0 - 1.0;
                if (x < 0.0) { if (x != 0.0) { runtime.log("ok"); } }
            }
        """.trimIndent()
        assertEquals(listOf("ok"), run(good))
        val ops = disassemble(Module.read(compile(good)), "main").joinToString(" ")
        for (op in listOf("FNEG", "FMUL", "FADD", "FDIV", "FSUB", "FCMP_LT", "FCMP_EQ")) {
            assertTrue(op in ops, "missing $op")
        }
    }

    @Test
    fun mixedNumericTypesAreRejected() {
        val lines = listOf(
            "let x: Float = 1;",
            "let x = 1 + 2.0;",
            "let x = 1.0 < 2;",
            "let x: Int = 1.0;",
            "let x = -true;",
        )
        for (line in lines) assertFailsWith<CompileError> { compile("fun main() { $line }") }
        assertFailsWith<CompileError> {
            compile("fun f(x: Float): Float { return x; } fun main() { let x = f(1); }")
        }
        assertFailsWith<CompileError> {
            compile("fun f(): Float { return 1; } fun main() {}")
        }
    }

    @Test
    fun signedZeroAndNonFiniteLiterals() {
        assertEquals(listOf("ok"), run("""
            fun main() { let n = -0.0; if (n == 0.0) { runtime.log("ok"); } }
        """))
        val massive = "9".repeat(400) + ".0"
        assertFailsWith<CompileError> { compile("fun main() { let x = $massive; }") }
    }

    @Test
    fun swappedComparisonsPreserveEvaluationOrder() {
        val source = """
            fun sideA(): Float { runtime.log("A"); return 3.0; }
            fun sideB(): Float { runtime.log("B"); return 2.0; }
            fun main() {
                if (sideA() > sideB()) { runtime.log("gt"); }
                if (sideA() <= sideB()) { runtime.log("le"); }
            }
        """.trimIndent()
        assertEquals(listOf("A", "B", "gt", "A", "B"), run(source))
    }
}
