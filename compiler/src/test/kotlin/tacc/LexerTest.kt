package tacc

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class LexerTest {
    private fun types(source: String): List<TokenType> = Lexer(source).tokenize().map { it.type }

    @Test
    fun keywordsIdentifiersAndLiterals() {
        assertEquals(
            listOf(
                TokenType.LET, TokenType.IDENT, TokenType.COLON, TokenType.TYPE_INT, TokenType.ASSIGN,
                TokenType.INT, TokenType.SEMI, TokenType.CONST, TokenType.IDENT, TokenType.ASSIGN,
                TokenType.TRUE, TokenType.SEMI, TokenType.EOF,
            ),
            types("let count: Int = 42; const ok = true;"),
        )
    }

    @Test
    fun keywordPrefixIsAnIdentifier() {
        assertEquals(listOf(TokenType.IDENT, TokenType.IDENT, TokenType.EOF), types("letter fun_name"))
    }

    @Test
    fun oneAndTwoCharacterOperators() {
        assertEquals(
            listOf(
                TokenType.EQ, TokenType.NE, TokenType.LE, TokenType.GE, TokenType.LT, TokenType.GT,
                TokenType.ASSIGN, TokenType.BANG, TokenType.PLUS, TokenType.MINUS, TokenType.STAR,
                TokenType.SLASH, TokenType.EOF,
            ),
            types("== != <= >= < > = ! + - * /"),
        )
    }

    @Test
    fun commentsAndWindowsLineEndingsAreSkipped() {
        val tokens = Lexer("// comment\r\nlet x = 1; // trailing\r\n").tokenize()
        assertEquals(TokenType.LET, tokens[0].type)
        assertEquals(2, tokens[0].line)
        assertEquals(3, tokens.last().line)
    }

    @Test
    fun stringEscapes() {
        val token = Lexer("\"a\\\"b\\\\c\\nd\"").tokenize()[0]
        assertEquals(TokenType.STRING, token.type)
        assertEquals("a\"b\\c\nd", token.text)
    }

    @Test
    fun invalidEscapeIsAnError() {
        val e = assertFailsWith<LexError> { Lexer("\"a\\tb\"").tokenize() }
        assertEquals("invalid escape sequence '\\t'", e.message)
    }

    @Test
    fun unterminatedString() {
        assertFailsWith<LexError> { Lexer("\"abc").tokenize() }
        assertFailsWith<LexError> { Lexer("\"abc\ndef\"").tokenize() }
    }

    @Test
    fun integerLiteralMustFitIn64Bits() {
        assertEquals("9223372036854775807", Lexer("9223372036854775807").tokenize()[0].text)
        assertFailsWith<LexError> { Lexer("9223372036854775808").tokenize() }
    }

    @Test
    fun digitsFollowedByLettersAreAnError() {
        assertFailsWith<LexError> { Lexer("123abc").tokenize() }
    }

    @Test
    fun nonAsciiAndUnknownCharactersAreErrors() {
        val e = assertFailsWith<LexError> { Lexer("let x = 1;\nlet й = 2;").tokenize() }
        assertEquals(2, e.line)
        assertFailsWith<LexError> { Lexer("a & b").tokenize() }
    }
}
