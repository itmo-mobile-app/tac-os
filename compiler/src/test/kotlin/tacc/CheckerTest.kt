package tacc

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs

/** Semantic analysis: types, scopes, `const`, functions and Runtime calls. */
class CheckerTest {
    private fun accepts(source: String) {
        compile(source)
    }

    private fun rejects(source: String, message: String, line: Int? = null) {
        val e = compileError(source)
        assertIs<SemanticError>(e)
        assertEquals(message, e.message)
        if (line != null) assertEquals(line, e.line)
    }

    private fun inMain(body: String) = "fun main() {\n$body\n}"

    // ---- Types: positive ----

    @Test
    fun typesAreInferredAndAnnotationsMatch() {
        accepts(
            inMain(
                """
                let i = 1 + 2 * 3;
                let b: Bool = i < 10;
                const s: String = "text";
                let same = s == "text";
                let notSame = b != true;
                let neg = -i;
                let flipped = !b;
                """,
            ),
        )
    }

    // ---- Types: negative ----

    @Test
    fun annotationMustMatchInitializer() =
        rejects(inMain("let x: Int = true;"), "cannot initialize 'x' of type Int with a value of type Bool")

    @Test
    fun assignmentMustKeepTheType() =
        rejects(inMain("let x = 1;\nx = \"one\";"), "cannot assign a value of type String to 'x' of type Int", 3)

    @Test
    fun arithmeticRequiresInts() {
        rejects(inMain("let x = 1 + true;"), "operator '+' requires Int operands, found Int and Bool")
        rejects(inMain("let x = \"a\" * 2;"), "operator '*' requires Int operands, found String and Int")
    }

    @Test
    fun comparisonRequiresInts() =
        rejects(inMain("let x = true < false;"), "operator '<' requires Int operands, found Bool and Bool")

    @Test
    fun equalityRequiresTheSameType() =
        rejects(inMain("let x = 1 == true;"), "operator '==' requires operands of the same type, found Int and Bool")

    @Test
    fun unaryOperatorTypes() {
        rejects(inMain("let x = -true;"), "operator '-' requires Int, found Bool")
        rejects(inMain("let x = !1;"), "operator '!' requires Bool, found Int")
    }

    @Test
    fun conditionsMustBeBool() {
        rejects(inMain("if (1) { }"), "condition of 'if' must be Bool, found Int")
        rejects(inMain("while (\"yes\") { }"), "condition of 'while' must be Bool, found String")
    }

    // ---- Scopes and const ----

    @Test
    fun blockScopeAndShadowing() {
        accepts(
            inMain(
                """
                let x = 1;
                if (x == 1) {
                    let x = true;     // shadows the outer x inside this block
                    let y = !x;
                }
                x = x + 1;            // the outer x is an Int again
                if (true) { let y = 2; }   // a sibling block may reuse a name
                """,
            ),
        )
    }

    @Test
    fun variableIsNotVisibleOutsideItsBlock() =
        rejects(inMain("if (true) {\n  let y = 1;\n}\ny = 2;"), "undefined variable 'y'", 5)

    @Test
    fun loopVariableIsNotVisibleAfterTheLoop() =
        rejects(inMain("while (false) { let i = 0; }\nlet j = i;"), "undefined variable 'i'")

    @Test
    fun undefinedVariable() = rejects(inMain("let x = y + 1;"), "undefined variable 'y'")

    @Test
    fun variableIsNotVisibleInItsOwnInitializer() = rejects(inMain("let x = x + 1;"), "undefined variable 'x'")

    @Test
    fun redeclarationInTheSameScope() =
        rejects(inMain("let x = 1;\nlet x = 2;"), "'x' is already declared in this scope", 3)

    @Test
    fun parameterCannotBeRedeclaredAtTheTopOfTheBody() =
        rejects("fun f(a: Int) { let a = 2; }\nfun main() { }", "'a' is already declared in this scope")

    @Test
    fun constCannotBeReassigned() {
        rejects(inMain("const limit = 3;\nlimit = 4;"), "cannot assign to const 'limit'", 3)
        rejects(inMain("const c = 1;\nif (true) { c = 2; }"), "cannot assign to const 'c'")
    }

    @Test
    fun parametersCannotBeReassigned() =
        rejects("fun f(a: Int) { a = 1; }\nfun main() { }", "cannot assign to parameter 'a'")

    // ---- Functions ----

    @Test
    fun functionsMayBeCalledBeforeTheirDeclaration() =
        accepts("fun main() { let x = twice(2); }\nfun twice(n: Int): Int { return n * 2; }")

    @Test
    fun callChecks() {
        val add = "fun add(a: Int, b: Int): Int { return a + b; }\n"
        rejects(add + inMain("let x = add(1);"), "'add' expects 2 argument(s), found 1")
        rejects(add + inMain("let x = add(1, false);"), "argument 2 of 'add' must be Int, found Bool")
        rejects(inMain("let x = missing();"), "undefined function 'missing'")
    }

    @Test
    fun functionWithoutReturnTypeHasNoValue() {
        accepts("fun hello() { runtime.log(\"hi\"); }\n" + inMain("hello();"))
        rejects("fun hello() { }\n" + inMain("let x = hello();"), "'hello' does not return a value")
        rejects(inMain("let x = runtime.log(\"a\");"), "'runtime.log' does not return a value")
    }

    @Test
    fun returnChecks() {
        rejects("fun f(): Int { return true; }\nfun main() { }", "function 'f' must return Int, found Bool")
        rejects("fun f(): Int { return; }\nfun main() { }", "function 'f' must return a value of type Int")
        rejects(inMain("return 1;"), "function 'main' has no return type and cannot return a value")
        accepts(inMain("if (true) { return; }\nruntime.log(\"x\");"))
    }

    @Test
    fun everyPathMustReturn() {
        accepts("fun f(a: Int): Int { if (a < 0) { return 0; } else { return a; } }\nfun main() { }")
        accepts("fun f(a: Int): Int { if (a < 0) { return 0; } else if (a == 0) { return 1; } else { return a; } }\nfun main() { }")
        accepts("fun f(a: Int): Int { if (a < 0) { return 0; }\nreturn a; }\nfun main() { }")
        rejects(
            "fun f(a: Int): Int { if (a < 0) { return 0; } }\nfun main() { }",
            "function 'f' must return a value of type Int on every path",
        )
        rejects(
            "fun f(): Int { while (true) { return 1; } }\nfun main() { }",
            "function 'f' must return a value of type Int on every path",
        )
    }

    @Test
    fun programStructure() {
        rejects("fun f() { }", "missing 'main' function")
        rejects("fun main(a: Int) { }", "'main' must not have parameters")
        rejects("fun main() { }\nfun main() { }", "function 'main' is already declared", 2)
    }

    // ---- Runtime calls ----

    @Test
    fun runtimeCallChecks() {
        accepts(inMain("let s = \"hi\";\nruntime.log(s);"))
        rejects(inMain("runtime.log(42);"), "argument 1 of 'runtime.log' must be String, found Int")
        rejects(inMain("runtime.log(\"a\", \"b\");"), "'runtime.log' expects 1 argument(s), found 2")
        rejects(inMain("runtime.print(\"a\");"), "unknown Runtime function 'runtime.print'")
    }

    // ---- Limits from spec/bytecode.md ----

    @Test
    fun atMost255LocalsPerFunction() {
        val decls = (1..255).joinToString("\n") { "let v$it = $it;" }
        accepts(inMain(decls))
        rejects(inMain(decls + "\nlet v256 = 256;"), "too many variables in function 'main'")
    }
}
