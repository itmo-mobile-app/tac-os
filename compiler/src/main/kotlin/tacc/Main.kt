package tacc

import java.io.File
import kotlin.system.exitProcess

/** Compiles `.tc` source text into the bytes of a `.bc` file. Throws [CompileError] on invalid input. */
fun compile(source: String): ByteArray {
    val program = Parser(Lexer(source).tokenize()).parseProgram()
    Checker().check(program)
    return BytecodeWriter.write(CodeGen().generate(program))
}

fun main(args: Array<String>) {
    if (args.size != 2) {
        System.err.println("usage: tacc <input.tc> <output.bc>")
        exitProcess(1)
    }
    val inputPath = args[0]
    val outputPath = args[1]

    val source =
        try {
            File(inputPath).readText(Charsets.UTF_8)
        } catch (e: Exception) {
            System.err.println("error: cannot read '$inputPath': ${e.message}")
            exitProcess(1)
        }

    val bytes =
        try {
            compile(source)
        } catch (e: CompileError) {
            System.err.println("$inputPath:${e.line}: error: ${e.message}")
            exitProcess(1)
        }

    try {
        File(outputPath).writeBytes(bytes)
    } catch (e: Exception) {
        System.err.println("error: cannot write '$outputPath': ${e.message}")
        exitProcess(1)
    }
}
