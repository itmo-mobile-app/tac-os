package tacc

/**
 * AST for `.tc` per spec/language.md -> "Grammar".
 *
 * The parser builds the tree; [Checker] then fills in the `var` fields marked "set by Checker"
 * (expression types, local slots, call targets), which [CodeGen] reads.
 */

enum class Type(val displayName: String) {
    INT("Int"),
    BOOL("Bool"),
    STRING("String"),
    ;

    override fun toString(): String = displayName
}

// ---- Expressions ----

sealed class Expr {
    abstract val line: Int

    /** Static type of the expression; `null` for a call to a function without a return value. Set by Checker. */
    var type: Type? = null
}

data class IntLit(val value: Long, override val line: Int) : Expr()

data class BoolLit(val value: Boolean, override val line: Int) : Expr()

data class StringLit(val value: String, override val line: Int) : Expr()

data class VarRef(val name: String, override val line: Int) : Expr() {
    /** Local slot of the referenced variable. Set by Checker. */
    var slot: Int = -1
}

enum class UnaryOp(val symbol: String) { NEG("-"), NOT("!") }

data class Unary(val op: UnaryOp, val operand: Expr, override val line: Int) : Expr()

enum class BinaryOp(val symbol: String) {
    ADD("+"),
    SUB("-"),
    MUL("*"),
    DIV("/"),
    EQ("=="),
    NE("!="),
    LT("<"),
    LE("<="),
    GT(">"),
    GE(">="),
}

data class Binary(val op: BinaryOp, val left: Expr, val right: Expr, override val line: Int) : Expr()

/**
 * `name(args)` calls a function in the same file; `namespace.name(args)` calls a Runtime function
 * (spec/runtime-api.md).
 */
data class Call(val namespace: String?, val name: String, val args: List<Expr>, override val line: Int) : Expr() {
    val qualifiedName: String get() = if (namespace == null) name else "$namespace.$name"

    /** Index in the function table for a call to a `.tc` function. Set by Checker. */
    var functionIndex: Int = -1
}

// ---- Statements ----

sealed class Stmt {
    abstract val line: Int
}

data class Block(val statements: List<Stmt>, override val line: Int) : Stmt()

data class VarDecl(
    val isConst: Boolean,
    val name: String,
    val declaredType: Type?,
    val initializer: Expr,
    override val line: Int,
) : Stmt() {
    /** Local slot of the new variable. Set by Checker. */
    var slot: Int = -1
}

data class Assign(val name: String, val value: Expr, override val line: Int) : Stmt() {
    /** Local slot of the assigned variable. Set by Checker. */
    var slot: Int = -1
}

/** `elseBranch` is a [Block] or, for `else if`, another [If]. */
data class If(val condition: Expr, val thenBlock: Block, val elseBranch: Stmt?, override val line: Int) : Stmt()

data class While(val condition: Expr, val body: Block, override val line: Int) : Stmt()

data class Return(val value: Expr?, override val line: Int) : Stmt()

data class CallStmt(val call: Call, override val line: Int) : Stmt()

/** True if every path through this statement ends in `return`. `while` never counts: its body may not run. */
fun Stmt.alwaysReturns(): Boolean = when (this) {
    is Return -> true
    is Block -> statements.any { it.alwaysReturns() }
    is If -> elseBranch != null && thenBlock.alwaysReturns() && elseBranch.alwaysReturns()
    else -> false
}

// ---- Declarations ----

data class Param(val name: String, val type: Type, val line: Int)

data class FunctionDecl(
    val name: String,
    val params: List<Param>,
    val returnType: Type?,
    val body: Block,
    val line: Int,
) {
    /** Number of `let`/`const` locals, not counting parameters (`local_count` in spec/bytecode.md). Set by Checker. */
    var localCount: Int = 0
}

data class Program(val functions: List<FunctionDecl>)
