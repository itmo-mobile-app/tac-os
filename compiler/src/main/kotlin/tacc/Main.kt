package tacc

import java.io.File
import kotlin.system.exitProcess

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

    try {
        val tokens = Lexer(source).tokenize()
        val program = Parser(tokens).parseProgram()
        val module = CodeGen().generate(program)
        val bytes = BytecodeWriter.write(module)
        File(outputPath).writeBytes(bytes)
    } catch (e: LexError) {
        System.err.println("$inputPath:${e.line}: error: ${e.message}")
        exitProcess(1)
    } catch (e: ParseError) {
        System.err.println("$inputPath:${e.line}: error: ${e.message}")
        exitProcess(1)
    } catch (e: CodeGenError) {
        System.err.println("$inputPath:${e.line}: error: ${e.message}")
        exitProcess(1)
    }
}
