package tacc

/**
 * Recursive-descent parser for the grammar in spec/language.md -> "Grammar": one function per
 * grammar rule. Produces an untyped AST; name resolution and type checking are done by [Checker].
 */

class ParseError(message: String, line: Int) : CompileError(message, line)

class Parser(private val tokens: List<Token>) {
    private var pos = 0

    private fun peek(): Token = tokens[pos]

    private fun peekAt(offset: Int): Token = tokens[minOf(pos + offset, tokens.size - 1)]

    private fun advance(): Token = tokens[pos++]

    private fun check(type: TokenType): Boolean = peek().type == type

    private fun match(type: TokenType): Boolean {
        if (!check(type)) return false
        advance()
        return true
    }

    private fun expect(type: TokenType, what: String): Token {
        if (!check(type)) throw ParseError("expected $what, found ${describe(peek())}", peek().line)
        return advance()
    }

    private fun describe(token: Token): String =
        if (token.type == TokenType.EOF) "end of file" else "'${token.text}'"

    // program = { function } ;
    fun parseProgram(): Program {
        val functions = mutableListOf<FunctionDecl>()
        while (!check(TokenType.EOF)) {
            functions.add(parseFunction())
        }
        return Program(functions)
    }

    // function = "fun" ident "(" [ param { "," param } ] ")" [ ":" type ] block ;
    private fun parseFunction(): FunctionDecl {
        val funTok = expect(TokenType.FUN, "'fun'")
        val name = expect(TokenType.IDENT, "function name").text
        expect(TokenType.LPAREN, "'('")
        val params = mutableListOf<Param>()
        if (!check(TokenType.RPAREN)) {
            do {
                val paramTok = expect(TokenType.IDENT, "parameter name")
                expect(TokenType.COLON, "':' and parameter type")
                params.add(Param(paramTok.text, parseType(), paramTok.line))
            } while (match(TokenType.COMMA))
        }
        expect(TokenType.RPAREN, "')'")
        val returnType = if (match(TokenType.COLON)) parseType() else null
        return FunctionDecl(name, params, returnType, parseBlock(), funTok.line)
    }

    // type = "Int" | "Bool" | "String" ;
    private fun parseType(): Type {
        val tok = advance()
        return when (tok.type) {
            TokenType.TYPE_INT -> Type.INT
            TokenType.TYPE_BOOL -> Type.BOOL
            TokenType.TYPE_STRING -> Type.STRING
            else -> throw ParseError("expected type 'Int', 'Bool' or 'String', found ${describe(tok)}", tok.line)
        }
    }

    // block = "{" { statement } "}" ;
    private fun parseBlock(): Block {
        val open = expect(TokenType.LBRACE, "'{'")
        val statements = mutableListOf<Stmt>()
        while (!check(TokenType.RBRACE)) {
            if (check(TokenType.EOF)) throw ParseError("expected '}', found end of file", peek().line)
            statements.add(parseStatement())
        }
        expect(TokenType.RBRACE, "'}'")
        return Block(statements, open.line)
    }

    // statement = var_decl | assignment | if_stmt | while_stmt | return_stmt | call_stmt ;
    private fun parseStatement(): Stmt {
        val tok = peek()
        return when (tok.type) {
            TokenType.LET, TokenType.CONST -> parseVarDecl()
            TokenType.IF -> parseIf()
            TokenType.WHILE -> parseWhile()
            TokenType.RETURN -> parseReturn()
            TokenType.IDENT ->
                if (peekAt(1).type == TokenType.ASSIGN) parseAssignment() else parseCallStatement()
            else -> throw ParseError("expected a statement, found ${describe(tok)}", tok.line)
        }
    }

    // var_decl = ( "let" | "const" ) ident [ ":" type ] "=" expression ";" ;
    private fun parseVarDecl(): Stmt {
        val kw = advance()
        val name = expect(TokenType.IDENT, "variable name").text
        val declaredType = if (match(TokenType.COLON)) parseType() else null
        expect(TokenType.ASSIGN, "'=' and an initializer")
        val init = parseExpression()
        expect(TokenType.SEMI, "';'")
        return VarDecl(kw.type == TokenType.CONST, name, declaredType, init, kw.line)
    }

    // assignment = ident "=" expression ";" ;
    private fun parseAssignment(): Stmt {
        val nameTok = advance()
        advance() // '='
        val value = parseExpression()
        expect(TokenType.SEMI, "';'")
        return Assign(nameTok.text, value, nameTok.line)
    }

    // if_stmt = "if" "(" expression ")" block [ "else" ( if_stmt | block ) ] ;
    private fun parseIf(): If {
        val ifTok = advance()
        expect(TokenType.LPAREN, "'(' after 'if'")
        val condition = parseExpression()
        expect(TokenType.RPAREN, "')'")
        val thenBlock = parseBlock()
        val elseBranch = if (match(TokenType.ELSE)) {
            if (check(TokenType.IF)) parseIf() else parseBlock()
        } else {
            null
        }
        return If(condition, thenBlock, elseBranch, ifTok.line)
    }

    // while_stmt = "while" "(" expression ")" block ;
    private fun parseWhile(): Stmt {
        val whileTok = advance()
        expect(TokenType.LPAREN, "'(' after 'while'")
        val condition = parseExpression()
        expect(TokenType.RPAREN, "')'")
        return While(condition, parseBlock(), whileTok.line)
    }

    // return_stmt = "return" [ expression ] ";" ;
    private fun parseReturn(): Stmt {
        val returnTok = advance()
        val value = if (check(TokenType.SEMI)) null else parseExpression()
        expect(TokenType.SEMI, "';'")
        return Return(value, returnTok.line)
    }

    // call_stmt = call ";" ;
    private fun parseCallStatement(): Stmt {
        val call = parseCall()
        expect(TokenType.SEMI, "';'")
        return CallStmt(call, call.line)
    }

    // expression = equality ;
    private fun parseExpression(): Expr = parseEquality()

    // equality = relational [ ( "==" | "!=" ) relational ] ;
    private fun parseEquality(): Expr {
        val left = parseRelational()
        val op = when (peek().type) {
            TokenType.EQ -> BinaryOp.EQ
            TokenType.NE -> BinaryOp.NE
            else -> return left
        }
        val opTok = advance()
        val result = Binary(op, left, parseRelational(), opTok.line)
        if (check(TokenType.EQ) || check(TokenType.NE)) {
            throw ParseError("equality operators cannot be chained; use parentheses", peek().line)
        }
        return result
    }

    // relational = additive [ ( "<" | "<=" | ">" | ">=" ) additive ] ;
    private fun parseRelational(): Expr {
        val left = parseAdditive()
        val op = relationalOp(peek().type) ?: return left
        val opTok = advance()
        val result = Binary(op, left, parseAdditive(), opTok.line)
        if (relationalOp(peek().type) != null) {
            throw ParseError("comparison operators cannot be chained; use parentheses", peek().line)
        }
        return result
    }

    private fun relationalOp(type: TokenType): BinaryOp? = when (type) {
        TokenType.LT -> BinaryOp.LT
        TokenType.LE -> BinaryOp.LE
        TokenType.GT -> BinaryOp.GT
        TokenType.GE -> BinaryOp.GE
        else -> null
    }

    // additive = term { ( "+" | "-" ) term } ;   (left-associative)
    private fun parseAdditive(): Expr {
        var left = parseTerm()
        while (true) {
            val op = when (peek().type) {
                TokenType.PLUS -> BinaryOp.ADD
                TokenType.MINUS -> BinaryOp.SUB
                else -> return left
            }
            val opTok = advance()
            left = Binary(op, left, parseTerm(), opTok.line)
        }
    }

    // term = unary { ( "*" | "/" ) unary } ;   (left-associative)
    private fun parseTerm(): Expr {
        var left = parseUnary()
        while (true) {
            val op = when (peek().type) {
                TokenType.STAR -> BinaryOp.MUL
                TokenType.SLASH -> BinaryOp.DIV
                else -> return left
            }
            val opTok = advance()
            left = Binary(op, left, parseUnary(), opTok.line)
        }
    }

    // unary = ( "-" | "!" ) unary | primary ;
    private fun parseUnary(): Expr {
        val op = when (peek().type) {
            TokenType.MINUS -> UnaryOp.NEG
            TokenType.BANG -> UnaryOp.NOT
            else -> return parsePrimary()
        }
        val opTok = advance()
        return Unary(op, parseUnary(), opTok.line)
    }

    // primary = int_lit | string_lit | "true" | "false" | call | ident | "(" expression ")" ;
    private fun parsePrimary(): Expr {
        val tok = peek()
        return when (tok.type) {
            TokenType.INT -> { advance(); IntLit(tok.text.toLong(), tok.line) }
            TokenType.STRING -> { advance(); StringLit(tok.text, tok.line) }
            TokenType.TRUE -> { advance(); BoolLit(true, tok.line) }
            TokenType.FALSE -> { advance(); BoolLit(false, tok.line) }
            TokenType.IDENT -> {
                val next = peekAt(1).type
                if (next == TokenType.LPAREN || next == TokenType.DOT) {
                    parseCall()
                } else {
                    advance()
                    VarRef(tok.text, tok.line)
                }
            }
            TokenType.LPAREN -> {
                advance()
                val inner = parseExpression()
                expect(TokenType.RPAREN, "')'")
                inner
            }
            else -> throw ParseError("expected an expression, found ${describe(tok)}", tok.line)
        }
    }

    // call = ident [ "." ident ] "(" [ expression { "," expression } ] ")" ;
    private fun parseCall(): Call {
        val first = expect(TokenType.IDENT, "function name")
        var namespace: String? = null
        var name = first.text
        if (match(TokenType.DOT)) {
            namespace = first.text
            name = expect(TokenType.IDENT, "Runtime function name after '.'").text
        }
        expect(TokenType.LPAREN, "'(' to call '${if (namespace == null) name else "$namespace.$name"}'")
        val args = mutableListOf<Expr>()
        if (!check(TokenType.RPAREN)) {
            do {
                args.add(parseExpression())
            } while (match(TokenType.COMMA))
        }
        expect(TokenType.RPAREN, "')'")
        return Call(namespace, name, args, first.line)
    }
}
