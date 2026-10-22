package tacc

class CodeGenError(message: String, line: Int) : CompileError(message, line)

/**
 * Lowers a checked AST (see [Checker]) to a [BytecodeModule], following spec/language.md ->
 * "Compilation notes":
 *
 * - every value is one slot; `Bool` is `Int` 0/1;
 * - `>`, `<=`, `>=`, `!=`, `!x`, and `-x` are lowered onto `CMP_LT`, `CMP_EQ`, `NOT`, and `SUB`;
 * - every call leaves exactly one value on the stack, so a call used as a statement is followed
 *   by `POP`;
 * - a function without a return type returns `Int 0` (`PUSH_CONST 0; RET`) on `return;` and at
 *   the end of its body.
 *
 * Jumps use absolute offsets from the start of the function's code (spec/bytecode.md). No jump
 * ever targets the end of the code: a branch that always returns is not followed by a jump.
 */
class CodeGen {
    private val pool = ConstantPool()
    private lateinit var code: CodeBuilder

    fun generate(program: Program): BytecodeModule {
        if (program.functions.size > MAX_U16 + 1) {
            throw CodeGenError("too many functions", program.functions[MAX_U16 + 1].line)
        }
        val functions = program.functions.map { fn ->
            code = CodeBuilder()
            genStatements(fn.body.statements)
            if (fn.returnType == null) emitReturnZero(fn.line)
            CompiledFunction(fn.name, fn.params.size, fn.localCount, code.toByteArray())
        }
        val entry = program.functions.indexOfFirst { it.name == "main" }
        check(entry >= 0) { "internal error: 'main' presence should have been checked by Checker" }
        return BytecodeModule(pool.all(), functions, entry)
    }

    // ---- Statements ----

    private fun genStatements(statements: List<Stmt>) {
        for (stmt in statements) genStmt(stmt)
    }

    private fun genStmt(stmt: Stmt) {
        when (stmt) {
            is Block -> genStatements(stmt.statements)
            is VarDecl -> {
                genExpr(stmt.initializer)
                code.store(stmt.slot)
            }
            is Assign -> {
                genExpr(stmt.value)
                code.store(stmt.slot)
            }
            is If -> genIf(stmt)
            is While -> genWhile(stmt)
            is Return -> {
                val value = stmt.value
                if (value == null) emitReturnZero(stmt.line) else { genExpr(value); code.op(Opcode.RET) }
            }
            is CallStmt -> {
                genExpr(stmt.call)
                code.op(Opcode.POP) // discard the one value every call leaves on the stack
            }
        }
    }

    //     <condition>
    //     JMP_IF_FALSE else      (to end when there is no else branch)
    //     <then>
    //     JMP end                (omitted when there is no else, or <then> always returns)
    // else:
    //     <else>
    // end:
    private fun genIf(stmt: If) {
        genExpr(stmt.condition)
        val toElse = code.jumpForward(Opcode.JMP_IF_FALSE)
        genStmt(stmt.thenBlock)
        val elseBranch = stmt.elseBranch
        if (elseBranch == null) {
            code.patch(toElse, code.offset)
            return
        }
        val toEnd = if (stmt.thenBlock.alwaysReturns()) null else code.jumpForward(Opcode.JMP)
        code.patch(toElse, code.offset)
        genStmt(elseBranch)
        toEnd?.let { code.patch(it, code.offset) }
    }

    // start:
    //     <condition>
    //     JMP_IF_FALSE end
    //     <body>
    //     JMP start
    // end:
    private fun genWhile(stmt: While) {
        val start = code.offset
        genExpr(stmt.condition)
        val toEnd = code.jumpForward(Opcode.JMP_IF_FALSE)
        genStmt(stmt.body)
        code.jump(Opcode.JMP, start)
        code.patch(toEnd, code.offset)
    }

    private fun emitReturnZero(line: Int) {
        code.pushConst(intConstant(0, line))
        code.op(Opcode.RET)
    }

    // ---- Expressions ----

    private fun genExpr(expr: Expr) {
        when (expr) {
            is IntLit -> code.pushConst(intConstant(expr.value, expr.line))
            is BoolLit -> code.pushConst(intConstant(if (expr.value) 1 else 0, expr.line))
            is StringLit -> code.pushConst(stringConstant(expr.value, expr.line))
            is VarRef -> code.load(expr.slot)
            is Unary -> when (expr.op) {
                UnaryOp.NEG -> {
                    code.pushConst(intConstant(0, expr.line))
                    genExpr(expr.operand)
                    code.op(Opcode.SUB)
                }
                UnaryOp.NOT -> {
                    genExpr(expr.operand)
                    code.op(Opcode.NOT)
                }
            }
            is Binary -> genBinary(expr)
            is Call -> {
                for (arg in expr.args) genExpr(arg) // left to right (spec/runtime-api.md)
                if (expr.namespace == null) {
                    code.call(expr.functionIndex, expr.args.size)
                } else {
                    code.callRuntime(stringConstant(expr.qualifiedName, expr.line), expr.args.size)
                }
            }
        }
    }

    private fun genBinary(expr: Binary) {
        val (first, second) = when (expr.op) {
            // `a > b` is `b < a`; `a <= b` is `!(b < a)`: operands are swapped.
            BinaryOp.GT, BinaryOp.LE -> expr.right to expr.left
            else -> expr.left to expr.right
        }
        genExpr(first)
        genExpr(second)
        when (expr.op) {
            BinaryOp.ADD -> code.op(Opcode.ADD)
            BinaryOp.SUB -> code.op(Opcode.SUB)
            BinaryOp.MUL -> code.op(Opcode.MUL)
            BinaryOp.DIV -> code.op(Opcode.DIV)
            BinaryOp.EQ -> code.op(Opcode.CMP_EQ)
            BinaryOp.NE -> code.op(Opcode.CMP_EQ).op(Opcode.NOT)
            BinaryOp.LT, BinaryOp.GT -> code.op(Opcode.CMP_LT)
            BinaryOp.LE, BinaryOp.GE -> code.op(Opcode.CMP_LT).op(Opcode.NOT)
        }
    }

    // ---- Constants ----

    private fun intConstant(value: Long, line: Int): Int = checkPoolSize(pool.intConstant(value), line)

    private fun stringConstant(value: String, line: Int): Int = checkPoolSize(pool.stringConstant(value), line)

    // Constant indexes are u16 operands (spec/bytecode.md).
    private fun checkPoolSize(index: Int, line: Int): Int {
        if (index > MAX_U16) throw CodeGenError("too many distinct constants", line)
        return index
    }

    private companion object {
        const val MAX_U16 = 0xFFFF
    }
}
