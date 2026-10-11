package tacc

/**
 * Minimal lexer for the `.tc` language.
 *
 * Covers only what is needed for the first Compiler -> VM integration
 * scenario (see spec/language.md): `fun`, identifiers, `( ) { } . ; ,`,
 * and string literals. Variables, arithmetic, control flow, and comments
 * other than `//` line comments are out of scope for this increment, even
 * though spec/language.md now defines the full grammar for them.
 */

data class Token(val type: TokenType, val text: String, val line: Int)

enum class TokenType {
    FUN,
    IDENT,
    STRING,
    LPAREN,
    RPAREN,
    LBRACE,
    RBRACE,
    DOT,
    SEMI,
    COMMA,
    EOF,
}

class LexError(message: String, val line: Int) : Exception("line $line: $message")

class Lexer(private val source: String) {
    private var pos = 0
    private var line = 1

    fun tokenize(): List<Token> {
        val tokens = mutableListOf<Token>()
        while (true) {
            skipWhitespaceAndComments()
            if (pos >= source.length) {
                tokens.add(Token(TokenType.EOF, "", line))
                break
            }
            val c = source[pos]
            when {
                c == '(' -> { tokens.add(Token(TokenType.LPAREN, "(", line)); pos++ }
                c == ')' -> { tokens.add(Token(TokenType.RPAREN, ")", line)); pos++ }
                c == '{' -> { tokens.add(Token(TokenType.LBRACE, "{", line)); pos++ }
                c == '}' -> { tokens.add(Token(TokenType.RBRACE, "}", line)); pos++ }
                c == '.' -> { tokens.add(Token(TokenType.DOT, ".", line)); pos++ }
                c == ';' -> { tokens.add(Token(TokenType.SEMI, ";", line)); pos++ }
                c == ',' -> { tokens.add(Token(TokenType.COMMA, ",", line)); pos++ }
                c == '"' -> tokens.add(readString())
                c.isLetter() || c == '_' -> tokens.add(readIdentOrKeyword())
                else -> throw LexError("unexpected character '$c'", line)
            }
        }
        return tokens
    }

    private fun skipWhitespaceAndComments() {
        while (pos < source.length) {
            val c = source[pos]
            when {
                c == '\n' -> { line++; pos++ }
                c.isWhitespace() -> pos++
                c == '/' && pos + 1 < source.length && source[pos + 1] == '/' -> {
                    while (pos < source.length && source[pos] != '\n') pos++
                }
                else -> return
            }
        }
    }

    private fun readString(): Token {
        val startLine = line
        pos++ // skip opening quote
        val sb = StringBuilder()
        while (true) {
            if (pos >= source.length) throw LexError("unterminated string literal", startLine)
            val c = source[pos]
            if (c == '"') { pos++; break }
            if (c == '\n') throw LexError("unterminated string literal", startLine)
            sb.append(c)
            pos++
        }
        return Token(TokenType.STRING, sb.toString(), startLine)
    }

    private fun readIdentOrKeyword(): Token {
        val start = pos
        val startLine = line
        while (pos < source.length && (source[pos].isLetterOrDigit() || source[pos] == '_')) pos++
        val text = source.substring(start, pos)
        val type = if (text == "fun") TokenType.FUN else TokenType.IDENT
        return Token(type, text, startLine)
    }
}
