package tacc

/**
 * Lexer for the `.tc` language: turns source text into tokens per spec/language.md -> "Grammar"
 * -> "Lexical tokens".
 */

data class Token(val type: TokenType, val text: String, val line: Int)

enum class TokenType {
    // Literals and names.
    IDENT,
    INT,
    STRING,

    // Keywords.
    FUN,
    LET,
    CONST,
    IF,
    ELSE,
    WHILE,
    RETURN,
    TRUE,
    FALSE,
    TYPE_INT,
    TYPE_BOOL,
    TYPE_STRING,

    // Punctuation.
    LPAREN,
    RPAREN,
    LBRACE,
    RBRACE,
    DOT,
    SEMI,
    COMMA,
    COLON,

    // Operators.
    ASSIGN,
    PLUS,
    MINUS,
    STAR,
    SLASH,
    BANG,
    EQ,
    NE,
    LT,
    LE,
    GT,
    GE,

    EOF,
}

/** Base class for every error reported as `<file>:<line>: error: <message>`. */
open class CompileError(message: String, val line: Int) : Exception(message)

class LexError(message: String, line: Int) : CompileError(message, line)

class Lexer(private val source: String) {
    private var pos = 0
    private var line = 1

    fun tokenize(): List<Token> {
        val tokens = mutableListOf<Token>()
        while (true) {
            skipWhitespaceAndComments()
            if (pos >= source.length) {
                tokens.add(Token(TokenType.EOF, "", line))
                return tokens
            }
            val c = source[pos]
            tokens.add(
                when {
                    c == '"' -> readString()
                    isDigit(c) -> readInt()
                    isLetter(c) || c == '_' -> readIdentOrKeyword()
                    else -> readOperator()
                },
            )
        }
    }

    private fun skipWhitespaceAndComments() {
        while (pos < source.length) {
            val c = source[pos]
            when {
                c == '\n' -> { line++; pos++ }
                c == ' ' || c == '\t' || c == '\r' -> pos++
                c == '/' && peekNext() == '/' -> {
                    while (pos < source.length && source[pos] != '\n') pos++
                }
                else -> return
            }
        }
    }

    private fun readOperator(): Token {
        val c = source[pos]
        if (peekNext() == '=') {
            val twoCharType = when (c) {
                '=' -> TokenType.EQ
                '!' -> TokenType.NE
                '<' -> TokenType.LE
                '>' -> TokenType.GE
                else -> null
            }
            if (twoCharType != null) {
                pos += 2
                return Token(twoCharType, "$c=", line)
            }
        }
        val type = when (c) {
            '(' -> TokenType.LPAREN
            ')' -> TokenType.RPAREN
            '{' -> TokenType.LBRACE
            '}' -> TokenType.RBRACE
            '.' -> TokenType.DOT
            ';' -> TokenType.SEMI
            ',' -> TokenType.COMMA
            ':' -> TokenType.COLON
            '=' -> TokenType.ASSIGN
            '+' -> TokenType.PLUS
            '-' -> TokenType.MINUS
            '*' -> TokenType.STAR
            '/' -> TokenType.SLASH
            '!' -> TokenType.BANG
            '<' -> TokenType.LT
            '>' -> TokenType.GT
            else -> throw LexError("unexpected character '$c'", line)
        }
        pos++
        return Token(type, c.toString(), line)
    }

    private fun readInt(): Token {
        val start = pos
        while (pos < source.length && isDigit(source[pos])) pos++
        if (pos < source.length && (isLetter(source[pos]) || source[pos] == '_')) {
            throw LexError("invalid integer literal '${source.substring(start, pos + 1)}'", line)
        }
        val text = source.substring(start, pos)
        // Must fit in a signed 64-bit integer (spec/language.md -> "Lexical tokens").
        if (text.toLongOrNull() == null) {
            throw LexError("integer literal $text does not fit in a signed 64-bit integer", line)
        }
        return Token(TokenType.INT, text, line)
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
            if (c == '\\') {
                if (pos + 1 >= source.length) throw LexError("unterminated string literal", startLine)
                when (val e = source[pos + 1]) {
                    '"' -> sb.append('"')
                    '\\' -> sb.append('\\')
                    'n' -> sb.append('\n')
                    else -> throw LexError("invalid escape sequence '\\$e'", line)
                }
                pos += 2
                continue
            }
            sb.append(c)
            pos++
        }
        return Token(TokenType.STRING, sb.toString(), startLine)
    }

    private fun readIdentOrKeyword(): Token {
        val start = pos
        while (pos < source.length && (isLetter(source[pos]) || isDigit(source[pos]) || source[pos] == '_')) pos++
        val text = source.substring(start, pos)
        return Token(KEYWORDS[text] ?: TokenType.IDENT, text, line)
    }

    private fun peekNext(): Char? = if (pos + 1 < source.length) source[pos + 1] else null

    // ASCII only, per spec/language.md -> "Lexical tokens" (`letter`, `digit`).
    private fun isLetter(c: Char): Boolean = c in 'a'..'z' || c in 'A'..'Z'

    private fun isDigit(c: Char): Boolean = c in '0'..'9'

    private companion object {
        val KEYWORDS = mapOf(
            "fun" to TokenType.FUN,
            "let" to TokenType.LET,
            "const" to TokenType.CONST,
            "if" to TokenType.IF,
            "else" to TokenType.ELSE,
            "while" to TokenType.WHILE,
            "return" to TokenType.RETURN,
            "true" to TokenType.TRUE,
            "false" to TokenType.FALSE,
            "Int" to TokenType.TYPE_INT,
            "Bool" to TokenType.TYPE_BOOL,
            "String" to TokenType.TYPE_STRING,
        )
    }
}
