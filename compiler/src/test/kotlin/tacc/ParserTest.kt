package tacc

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs
import kotlin.test.assertNull

class ParserTest {
    private fun parse(source: String): Program = parseProgram(source)

    /** Parses `fun main() { let x = <expr>; }` and renders the initializer with explicit parentheses. */
    private fun expr(source: String): String {
        val decl = parse("fun main() { let x = $source; }").functions[0].body.statements[0] as VarDecl
        return render(decl.initializer)
    }

    private fun render(e: Expr): String = when (e) {
        is IntLit -> e.value.toString()
        is BoolLit -> e.value.toString()
        is StringLit -> "\"${e.value}\""
        is VarRef -> e.name
        is Unary -> "(${e.op.symbol}${render(e.operand)})"
        is Binary -> "(${render(e.left)} ${e.op.symbol} ${render(e.right)})"
        is Call -> "${e.qualifiedName}(${e.args.joinToString(", ") { render(it) }})"
    }

    private fun parseError(source: String): ParseError = assertFailsWith<ParseError> { parse(source) }

    @Test
    fun multiplicationBindsTighterThanAddition() {
        assertEquals("(a + (b * c))", expr("a + b * c"))
        assertEquals("((a * b) + c)", expr("a * b + c"))
    }

    @Test
    fun binaryOperatorsAreLeftAssociative() {
        assertEquals("((a - b) - c)", expr("a - b - c"))
        assertEquals("((a / b) / c)", expr("a / b / c"))
    }

    @Test
    fun precedenceFromLowestToHighest() {
        assertEquals("((a + 1) == (b < c))", expr("a + 1 == b < c"))
        assertEquals("(((-a) * b) < c)", expr("-a * b < c"))
        assertEquals("(!(!ok))", expr("!!ok"))
        assertEquals("(a * (b + c))", expr("a * (b + c)"))
    }

    @Test
    fun callsInExpressions() {
        assertEquals("(add(a, (b * 2)) + 1)", expr("add(a, b * 2) + 1"))
        assertEquals("runtime.log(\"x\")", expr("runtime.log(\"x\")"))
        assertEquals("f()", expr("f()"))
    }

    @Test
    fun comparisonsDoNotChain() {
        assertEquals("comparison operators cannot be chained; use parentheses", parseError("fun main() { let x = a < b < c; }").message)
        assertEquals("equality operators cannot be chained; use parentheses", parseError("fun main() { let x = a == b == c; }").message)
        // Parentheses make the intent explicit and are accepted by the parser.
        assertEquals("((a < b) == c)", expr("(a < b) == c"))
    }

    @Test
    fun functionWithParametersAndReturnType() {
        val fn = parse("fun add(a: Int, b: Bool): String { return \"x\"; }").functions[0]
        assertEquals(listOf("a" to Type.INT, "b" to Type.BOOL), fn.params.map { it.name to it.type })
        assertEquals(Type.STRING, fn.returnType)
    }

    @Test
    fun elseIfChain() {
        val stmt = parse("fun main() { if (a) { } else if (b) { } else { } }").functions[0].body.statements[0]
        val first = assertIs<If>(stmt)
        val second = assertIs<If>(first.elseBranch)
        assertIs<Block>(second.elseBranch)
    }

    @Test
    fun returnWithAndWithoutValue() {
        val body = parse("fun main() { return; } fun f(): Int { return 1; }").functions
        assertNull((body[0].body.statements[0] as Return).value)
        assertIs<IntLit>((body[1].body.statements[0] as Return).value)
    }

    @Test
    fun syntaxErrors() {
        assertEquals("expected ';', found '}'", parseError("fun main() { let x = 1 }").message)
        assertEquals("expected ':' or '=', found ';'", parseError("fun main() { let x; }").message)
        assertEquals("expected ':', found ')'", parseError("fun f(a) { }").message)
        assertEquals("expected a statement or '}', found '1'", parseError("fun main() { 1 + 2; }").message)
        assertEquals("expected a statement or '}', found end of file", parseError("fun main() { let x = 1;").message)
        assertEquals("expected an expression, found ';'", parseError("fun main() { let x = ; }").message)
        assertEquals("expected ';', found ')'", parseError("fun main() { let x = 1); }").message)
        assertEquals("expected 'fun' or end of file, found 'let'", parseError("let x = 1;").message)
        assertEquals(
            "expected type 'Int', 'Bool' or 'String', found 'Float'",
            parseError("fun main() { let x: Float = 1; }").message,
        )
        // A bare block is not a statement in spec/language.md -> "Grammar".
        assertEquals("expected a statement or '}', found '{'", parseError("fun main() { { } }").message)
    }

    @Test
    fun errorLineIsReported() {
        assertEquals(3, parseError("fun main() {\n  let x = 1;\n  x = ;\n}").line)
    }
}
