package tacc

/**
 * Minimal recursive-descent parser for the first Compiler -> VM scenario.
 *
 * Grammar covered (a strict subset of the full grammar in spec/language.md ->
 * "Grammar"; only what is needed for the first Compiler -> VM scenario):
 *   program  := function*
 *   function := "fun" IDENT "(" ")" "{" call* "}"
 *   call     := IDENT "." IDENT "(" ( STRING ( "," STRING )* )? ")" ";"
 *
 * Function parameters, return types, local variables, arithmetic, control
 * flow, and calls between `.tc` functions are defined in spec/language.md
 * but intentionally not implemented yet — out of scope for this increment
 * (remainder of Sprint 1).
 */

class ParseError(message: String, val line: Int) : Exception("line $line: $message")

class Parser(private val tokens: List<Token>) {
    private var pos = 0

    private fun peek(): Token = tokens[pos]

    private fun advance(): Token = tokens[pos++]

    private fun check(type: TokenType): Boolean = peek().type == type

    private fun expect(type: TokenType, what: String): Token {
        if (!check(type)) throw ParseError("expected $what, found '${peek().text}'", peek().line)
        return advance()
    }

    fun parseProgram(): Program {
        val functions = mutableListOf<FunctionDecl>()
        while (!check(TokenType.EOF)) {
            functions.add(parseFunction())
        }
        val mainCount = functions.count { it.name == "main" }
        if (mainCount == 0) throw ParseError("missing 'main' function", peek().line)
        if (mainCount > 1) throw ParseError("multiple 'main' functions", peek().line)
        return Program(functions)
    }

    private fun parseFunction(): FunctionDecl {
        val fnTok = expect(TokenType.FUN, "'fun'")
        val name = expect(TokenType.IDENT, "function name").text
        expect(TokenType.LPAREN, "'('")
        expect(TokenType.RPAREN, "')' (parameters are not supported in this minimal version)")
        // Note: a return type (": Int" etc.) is not accepted yet either — every
        // function in this increment is implicitly void (see CodeGen's implicit
        // "return 0" per spec/language.md -> Compilation notes -> Return values).
        expect(TokenType.LBRACE, "'{'")
        val body = mutableListOf<RuntimeCall>()
        while (!check(TokenType.RBRACE)) {
            body.add(parseRuntimeCallStatement())
        }
        expect(TokenType.RBRACE, "'}'")
        return FunctionDecl(name, body, fnTok.line)
    }

    private fun parseRuntimeCallStatement(): RuntimeCall {
        val nsTok = expect(TokenType.IDENT, "runtime namespace (e.g. 'runtime')")
        expect(TokenType.DOT, "'.'")
        val fnTok = expect(TokenType.IDENT, "runtime function name")
        expect(TokenType.LPAREN, "'('")
        val args = mutableListOf<String>()
        if (!check(TokenType.RPAREN)) {
            args.add(expect(TokenType.STRING, "string literal argument").text)
            while (check(TokenType.COMMA)) {
                advance()
                args.add(expect(TokenType.STRING, "string literal argument").text)
            }
        }
        expect(TokenType.RPAREN, "')'")
        expect(TokenType.SEMI, "';'")
        return RuntimeCall(nsTok.text, fnTok.text, args, nsTok.line)
    }
}
