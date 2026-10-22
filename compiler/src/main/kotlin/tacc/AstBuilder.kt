package tacc

import org.antlr.v4.runtime.Token

/**
 * Converts the ANTLR parse tree of `Tc.g4` into the AST in `Ast.kt`: one function per grammar
 * rule. Left-associative operator chains (`additive`, `term`) are folded left to right; chained
 * comparisons and equalities are rejected here (see the comment in `Tc.g4`).
 */
class AstBuilder {
    fun program(ctx: TcParser.ProgramContext): Program = Program(ctx.function().map { function(it) })

    private fun function(ctx: TcParser.FunctionContext): FunctionDecl =
        FunctionDecl(
            name = ctx.IDENT().text,
            params = ctx.param().map { Param(it.IDENT().text, type(it.typeName()), it.start.line) },
            returnType = ctx.typeName()?.let { type(it) },
            body = block(ctx.block()),
            line = ctx.start.line,
        )

    private fun type(ctx: TcParser.TypeNameContext): Type = when (ctx.start.type) {
        TcParser.INT_TYPE -> Type.INT
        TcParser.BOOL_TYPE -> Type.BOOL
        else -> Type.STRING
    }

    // ---- Statements ----

    private fun block(ctx: TcParser.BlockContext): Block = Block(ctx.statement().map { statement(it) }, ctx.start.line)

    private fun statement(ctx: TcParser.StatementContext): Stmt {
        ctx.varDecl()?.let { d ->
            return VarDecl(
                isConst = d.kind.type == TcParser.CONST,
                name = d.IDENT().text,
                declaredType = d.typeName()?.let { type(it) },
                initializer = expression(d.expression()),
                line = d.start.line,
            )
        }
        ctx.assignment()?.let { return Assign(it.IDENT().text, expression(it.expression()), it.start.line) }
        ctx.ifStmt()?.let { return ifStmt(it) }
        ctx.whileStmt()?.let { return While(expression(it.expression()), block(it.block()), it.start.line) }
        ctx.returnStmt()?.let { return Return(it.expression()?.let { e -> expression(e) }, it.start.line) }
        val call = call(ctx.callStmt().call())
        return CallStmt(call, call.line)
    }

    private fun ifStmt(ctx: TcParser.IfStmtContext): If {
        val blocks = ctx.block() // the `then` block, and the `else` block if there is one
        val elseBranch: Stmt? = when {
            ctx.ifStmt() != null -> ifStmt(ctx.ifStmt())
            blocks.size > 1 -> block(blocks[1])
            else -> null
        }
        return If(expression(ctx.expression()), block(blocks[0]), elseBranch, ctx.start.line)
    }

    // ---- Expressions ----

    private fun expression(ctx: TcParser.ExpressionContext): Expr = equality(ctx.equality())

    private fun equality(ctx: TcParser.EqualityContext): Expr {
        val operands = ctx.relational().map { relational(it) }
        return nonChaining(operands, ctx.op, "equality") { if (it.type == TcParser.EQ) BinaryOp.EQ else BinaryOp.NE }
    }

    private fun relational(ctx: TcParser.RelationalContext): Expr {
        val operands = ctx.additive().map { additive(it) }
        return nonChaining(operands, ctx.op, "comparison") {
            when (it.type) {
                TcParser.LT -> BinaryOp.LT
                TcParser.LE -> BinaryOp.LE
                TcParser.GT -> BinaryOp.GT
                else -> BinaryOp.GE
            }
        }
    }

    private fun nonChaining(operands: List<Expr>, ops: List<Token>, kind: String, op: (Token) -> BinaryOp): Expr {
        if (ops.size > 1) throw ParseError("$kind operators cannot be chained; use parentheses", ops[1].line)
        if (ops.isEmpty()) return operands[0]
        return Binary(op(ops[0]), operands[0], operands[1], ops[0].line)
    }

    private fun additive(ctx: TcParser.AdditiveContext): Expr =
        leftAssociative(ctx.term().map { term(it) }, ctx.op) {
            if (it.type == TcParser.PLUS) BinaryOp.ADD else BinaryOp.SUB
        }

    private fun term(ctx: TcParser.TermContext): Expr =
        leftAssociative(ctx.unary().map { unary(it) }, ctx.op) {
            if (it.type == TcParser.STAR) BinaryOp.MUL else BinaryOp.DIV
        }

    /** `a op1 b op2 c` -> `(a op1 b) op2 c`. */
    private fun leftAssociative(operands: List<Expr>, ops: List<Token>, op: (Token) -> BinaryOp): Expr {
        var result = operands[0]
        for ((i, token) in ops.withIndex()) result = Binary(op(token), result, operands[i + 1], token.line)
        return result
    }

    private fun unary(ctx: TcParser.UnaryContext): Expr {
        val op = ctx.op ?: return primary(ctx.primary())
        return Unary(if (op.type == TcParser.MINUS) UnaryOp.NEG else UnaryOp.NOT, unary(ctx.unary()), op.line)
    }

    private fun primary(ctx: TcParser.PrimaryContext): Expr {
        val line = ctx.start.line
        return when (ctx) {
            is TcParser.IntLiteralContext -> IntLit(intValue(ctx.INT().symbol), line)
            is TcParser.StringLiteralContext -> StringLit(stringValue(ctx.STRING().symbol), line)
            is TcParser.BoolLiteralContext -> BoolLit(ctx.TRUE() != null, line)
            is TcParser.CallExprContext -> call(ctx.call())
            is TcParser.VarRefContext -> VarRef(ctx.IDENT().text, line)
            is TcParser.ParensContext -> expression(ctx.expression())
            else -> error("internal error: unknown primary ${ctx.javaClass.simpleName}")
        }
    }

    private fun call(ctx: TcParser.CallContext): Call {
        val names = ctx.IDENT()
        val namespace = if (names.size == 2) names[0].text else null
        return Call(namespace, names.last().text, ctx.expression().map { expression(it) }, ctx.start.line)
    }
}
