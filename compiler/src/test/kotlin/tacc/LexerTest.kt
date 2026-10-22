package tacc

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/** The lexer generated from `Tc.g4`, plus the token checks in `Parse.kt`. */
class LexerTest {
    private fun types(source: String): List<String> =
        tokenize(source).tokens.map { TcLexer.VOCABULARY.getSymbolicName(it.type) }

    @Test
    fun keywordsIdentifiersAndLiterals() {
        assertEquals(
            listOf("LET", "IDENT", "COLON", "INT_TYPE", "ASSIGN", "INT", "SEMI", "CONST", "IDENT", "ASSIGN", "TRUE", "SEMI", "EOF"),
            types("let count: Int = 42; const ok = true;"),
        )
    }

    @Test
    fun keywordPrefixIsAnIdentifier() {
        assertEquals(listOf("IDENT", "IDENT", "EOF"), types("letter fun_name"))
    }

    @Test
    fun oneAndTwoCharacterOperators() {
        assertEquals(
            listOf("EQ", "NE", "LE", "GE", "LT", "GT", "ASSIGN", "BANG", "PLUS", "MINUS", "STAR", "SLASH", "EOF"),
            types("== != <= >= < > = ! + - * /"),
        )
    }

    @Test
    fun commentsAndWindowsLineEndingsAreSkipped() {
        val tokens = tokenize("// comment\r\nlet x = 1; // trailing\r\n").tokens
        assertEquals(TcLexer.LET, tokens[0].type)
        assertEquals(2, tokens[0].line)
        assertEquals(3, tokens.last().line)
    }

    @Test
    fun stringEscapes() {
        val token = tokenize("\"a\\\"b\\\\c\\nd\"").tokens[0]
        assertEquals(TcLexer.STRING, token.type)
        assertEquals("a\"b\\c\nd", stringValue(token))
    }

    @Test
    fun invalidEscapeIsAnError() {
        val e = assertFailsWith<LexError> { tokenize("\"a\\tb\"") }
        assertEquals("invalid escape sequence '\\t'", e.message)
    }

    @Test
    fun unterminatedString() {
        assertEquals("unterminated string literal", assertFailsWith<LexError> { tokenize("\"abc") }.message)
        assertFailsWith<LexError> { tokenize("\"abc\ndef\"") }
        assertFailsWith<LexError> { tokenize("\"abc\\\"") }
    }

    @Test
    fun integerLiteralMustFitIn64Bits() {
        assertEquals(Long.MAX_VALUE, intValue(tokenize("9223372036854775807").tokens[0]))
        assertFailsWith<LexError> { tokenize("9223372036854775808") }
    }

    @Test
    fun digitsFollowedByLettersAreAnError() {
        assertEquals("invalid integer literal '123abc'", assertFailsWith<LexError> { tokenize("123abc") }.message)
    }

    @Test
    fun nonAsciiAndUnknownCharactersAreErrors() {
        val e = assertFailsWith<LexError> { tokenize("let x = 1;\nlet й = 2;") }
        assertEquals("unexpected character 'й'", e.message)
        assertEquals(2, e.line)
        assertFailsWith<LexError> { tokenize("a & b") }
    }

    @Test
    fun lexicalErrorsAreReportedBeforeSyntaxErrors() {
        // Line 1 is a syntax error, line 2 a lexical one: the whole input is lexed first.
        val e = compileError("fun main() { let = 1; }\nlet y = \"open")
        assertEquals("unterminated string literal", e.message)
        assertEquals(2, e.line)
    }
}
