package tacc

/**
 * Semantic analysis: name resolution, block scopes, type checking, and local slot allocation,
 * per spec/language.md -> "Semantic rules" and "Compilation notes".
 *
 * Annotates the AST in place (expression types, local slots, call targets, `localCount`) for
 * [CodeGen]. Stops at the first error.
 */

class SemanticError(message: String, line: Int) : CompileError(message, line)

/** Runtime functions the compiler can call, with their signatures from spec/runtime-api.md. */
object RuntimeFunctions {
    /** `returnType == null`: the function has no result usable as a value (it pushes `Int 0`). */
    data class Signature(val params: List<Type>, val returnType: Type?)

    val all: Map<String, Signature> = mapOf(
        "runtime.log" to Signature(listOf(Type.STRING), null),
    )
}

class Checker {
    private enum class VarKind { LET, CONST, PARAM }

    private class Variable(val type: Type, val kind: VarKind, val slot: Int)

    private val functionIndex = mutableMapOf<String, Int>()
    private lateinit var functions: List<FunctionDecl>

    // Per-function state.
    private lateinit var current: FunctionDecl
    private val scopes = ArrayDeque<MutableMap<String, Variable>>()
    private var nextSlot = 0

    fun check(program: Program) {
        functions = program.functions
        for ((index, fn) in functions.withIndex()) {
            if (functionIndex.containsKey(fn.name)) {
                throw SemanticError("function '${fn.name}' is already declared", fn.line)
            }
            functionIndex[fn.name] = index
        }
        val main = functions.find { it.name == "main" } ?: throw SemanticError("missing 'main' function", 1)
        if (main.params.isNotEmpty()) throw SemanticError("'main' must not have parameters", main.line)

        for (fn in functions) checkFunction(fn)
    }

    private fun checkFunction(fn: FunctionDecl) {
        current = fn
        scopes.clear()
        nextSlot = 0
        if (fn.params.size > MAX_U8) {
            throw SemanticError("function '${fn.name}' has more than $MAX_U8 parameters", fn.line)
        }

        // Parameters and the top level of the body share one scope: a body-level `let` cannot
        // redeclare a parameter.
        scopes.addLast(mutableMapOf())
        for (param in fn.params) declare(param.name, param.type, VarKind.PARAM, param.line)
        for (stmt in fn.body.statements) checkStmt(stmt)
        scopes.removeLast()

        if (fn.returnType != null && !fn.body.alwaysReturns()) {
            throw SemanticError(
                "function '${fn.name}' must return a value of type ${fn.returnType} on every path",
                fn.line,
            )
        }
        fn.localCount = nextSlot - fn.params.size
    }

    // ---- Statements ----

    private fun checkStmt(stmt: Stmt) {
        when (stmt) {
            is Block -> checkBlock(stmt)
            is VarDecl -> {
                // The initializer is checked before the name is declared: a variable is not
                // visible in its own initializer.
                val type = valueType(stmt.initializer)
                if (stmt.declaredType != null && stmt.declaredType != type) {
                    throw SemanticError(
                        "cannot initialize '${stmt.name}' of type ${stmt.declaredType} with a value of type $type",
                        stmt.line,
                    )
                }
                val kind = if (stmt.isConst) VarKind.CONST else VarKind.LET
                stmt.slot = declare(stmt.name, type, kind, stmt.line).slot
            }
            is Assign -> {
                val variable = lookup(stmt.name) ?: throw SemanticError("undefined variable '${stmt.name}'", stmt.line)
                when (variable.kind) {
                    VarKind.CONST -> throw SemanticError("cannot assign to const '${stmt.name}'", stmt.line)
                    VarKind.PARAM -> throw SemanticError("cannot assign to parameter '${stmt.name}'", stmt.line)
                    VarKind.LET -> Unit
                }
                val type = valueType(stmt.value)
                if (type != variable.type) {
                    throw SemanticError(
                        "cannot assign a value of type $type to '${stmt.name}' of type ${variable.type}",
                        stmt.line,
                    )
                }
                stmt.slot = variable.slot
            }
            is If -> {
                checkCondition(stmt.condition, "if")
                checkBlock(stmt.thenBlock)
                stmt.elseBranch?.let { checkStmt(it) }
            }
            is While -> {
                checkCondition(stmt.condition, "while")
                checkBlock(stmt.body)
            }
            is Return -> checkReturn(stmt)
            is CallStmt -> checkCall(stmt.call)
        }
    }

    private fun checkBlock(block: Block) {
        scopes.addLast(mutableMapOf())
        for (stmt in block.statements) checkStmt(stmt)
        scopes.removeLast()
    }

    private fun checkCondition(condition: Expr, keyword: String) {
        val type = valueType(condition)
        if (type != Type.BOOL) {
            throw SemanticError("condition of '$keyword' must be Bool, found $type", condition.line)
        }
    }

    private fun checkReturn(stmt: Return) {
        val expected = current.returnType
        val value = stmt.value
        if (expected == null) {
            if (value != null) {
                throw SemanticError(
                    "function '${current.name}' has no return type and cannot return a value",
                    stmt.line,
                )
            }
            return
        }
        if (value == null) {
            throw SemanticError("function '${current.name}' must return a value of type $expected", stmt.line)
        }
        val type = valueType(value)
        if (type != expected) {
            throw SemanticError("function '${current.name}' must return $expected, found $type", stmt.line)
        }
    }

    // ---- Expressions ----

    /** Type of an expression that must produce a value. */
    private fun valueType(expr: Expr): Type =
        checkExpr(expr) ?: throw SemanticError(
            "'${(expr as Call).qualifiedName}' does not return a value",
            expr.line,
        )

    /** Returns the expression's type, or `null` for a call to a function without a return value. */
    private fun checkExpr(expr: Expr): Type? {
        val type: Type? = when (expr) {
            is IntLit -> Type.INT
            is BoolLit -> Type.BOOL
            is StringLit -> Type.STRING
            is VarRef -> {
                val variable = lookup(expr.name) ?: throw SemanticError("undefined variable '${expr.name}'", expr.line)
                expr.slot = variable.slot
                variable.type
            }
            is Unary -> {
                val operand = valueType(expr.operand)
                val required = if (expr.op == UnaryOp.NEG) Type.INT else Type.BOOL
                if (operand != required) {
                    throw SemanticError("operator '${expr.op.symbol}' requires $required, found $operand", expr.line)
                }
                required
            }
            is Binary -> checkBinary(expr)
            is Call -> checkCall(expr)
        }
        expr.type = type
        return type
    }

    private fun checkBinary(expr: Binary): Type {
        val left = valueType(expr.left)
        val right = valueType(expr.right)
        return when (expr.op) {
            BinaryOp.ADD, BinaryOp.SUB, BinaryOp.MUL, BinaryOp.DIV -> {
                requireInts(expr, left, right)
                Type.INT
            }
            BinaryOp.LT, BinaryOp.LE, BinaryOp.GT, BinaryOp.GE -> {
                requireInts(expr, left, right)
                Type.BOOL
            }
            BinaryOp.EQ, BinaryOp.NE -> {
                if (left != right) {
                    throw SemanticError(
                        "operator '${expr.op.symbol}' requires operands of the same type, found $left and $right",
                        expr.line,
                    )
                }
                Type.BOOL
            }
        }
    }

    private fun requireInts(expr: Binary, left: Type, right: Type) {
        if (left != Type.INT || right != Type.INT) {
            throw SemanticError(
                "operator '${expr.op.symbol}' requires Int operands, found $left and $right",
                expr.line,
            )
        }
    }

    private fun checkCall(call: Call): Type? {
        val paramTypes: List<Type>
        val returnType: Type?
        if (call.namespace == null) {
            val index = functionIndex[call.name] ?: throw SemanticError("undefined function '${call.name}'", call.line)
            val fn = functions[index]
            call.functionIndex = index
            paramTypes = fn.params.map { it.type }
            returnType = fn.returnType
        } else {
            val signature = RuntimeFunctions.all[call.qualifiedName]
                ?: throw SemanticError("unknown Runtime function '${call.qualifiedName}'", call.line)
            paramTypes = signature.params
            returnType = signature.returnType
        }

        if (call.args.size != paramTypes.size) {
            throw SemanticError(
                "'${call.qualifiedName}' expects ${paramTypes.size} argument(s), found ${call.args.size}",
                call.line,
            )
        }
        for ((i, arg) in call.args.withIndex()) {
            val type = valueType(arg)
            if (type != paramTypes[i]) {
                throw SemanticError(
                    "argument ${i + 1} of '${call.qualifiedName}' must be ${paramTypes[i]}, found $type",
                    arg.line,
                )
            }
        }
        call.type = returnType
        return returnType
    }

    // ---- Scopes and slots ----

    private fun declare(name: String, type: Type, kind: VarKind, line: Int): Variable {
        val scope = scopes.last()
        if (scope.containsKey(name)) throw SemanticError("'$name' is already declared in this scope", line)
        // Slot indexes are u8 operands of LOAD/STORE; param_count and local_count are u8 (spec/bytecode.md).
        val localsSoFar = nextSlot - current.params.size
        if (nextSlot > MAX_SLOT || (kind != VarKind.PARAM && localsSoFar >= MAX_U8)) {
            throw SemanticError("too many variables in function '${current.name}'", line)
        }
        val variable = Variable(type, kind, nextSlot++)
        scope[name] = variable
        return variable
    }

    /** Innermost declaration wins: a nested block may shadow an outer variable. */
    private fun lookup(name: String): Variable? {
        for (scope in scopes.asReversed()) scope[name]?.let { return it }
        return null
    }

    private companion object {
        const val MAX_U8 = 255
        const val MAX_SLOT = 255
    }
}
