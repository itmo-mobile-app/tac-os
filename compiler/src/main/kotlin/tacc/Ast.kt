package tacc

/**
 * AST for the minimal subset of `.tc` covered by this increment: a function
 * body is a sequence of single-argument Runtime calls (`ns.func("text");`).
 * Local variables, arithmetic, control flow, and calls to other `.tc`
 * functions are not part of this scenario (see spec/language.md -> Scope).
 */

data class RuntimeCall(
    val namespace: String,
    val function: String,
    val args: List<String>,
    val line: Int,
)

data class FunctionDecl(
    val name: String,
    val body: List<RuntimeCall>,
    val line: Int,
)

data class Program(val functions: List<FunctionDecl>)
