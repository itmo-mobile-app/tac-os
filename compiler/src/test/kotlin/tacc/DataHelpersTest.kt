package tacc

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertTrue

/**
 * Observation/Target data helpers (#127, #132): signatures, type errors and lowering onto the
 * existing CALL_RUNTIME. Their execution is covered by the tacc -> tacvm E2E in tests/data-tc.
 */
class DataHelpersTest {
    private fun rejects(source: String, message: String) {
        val e = compileError(source)
        assertIs<SemanticError>(e)
        assertEquals(message, e.message)
    }

    private fun inMain(body: String) = "fun main() {\n$body\n}"

    // ---- Positive ----

    private val observationFragment = """
        fun observation(targetId: String, observerId: String, latitude: Float, longitude: Float,
                        confidence: Float, timestamp: Int): String {
            let result = "{\"targetId\":";
            result = text.concat(result, json.quote(targetId));
            result = text.concat(result, ",\"observerId\":");
            result = text.concat(result, json.quote(observerId));
            result = text.concat(result, ",\"latitude\":");
            result = text.concat(result, text.fromFloat(latitude));
            result = text.concat(result, ",\"longitude\":");
            result = text.concat(result, text.fromFloat(longitude));
            result = text.concat(result, ",\"confidence\":");
            result = text.concat(result, text.fromFloat(confidence));
            result = text.concat(result, ",\"timestamp\":");
            result = text.concat(result, text.fromInt(timestamp));
            return text.concat(result, "}");
        }

        fun main() {
            const detections = camera.detectMarkers();
            let index = 0;
            while (index < json.arrayLength(detections)) {
                const detection = json.arrayObject(detections, index);
                const id = json.getInt(detection, "id");
                const distance = json.getFloat(detection, "distance");
                runtime.log(observation(text.fromInt(id), "device-1", distance / 6371000.0, 0.0,
                    0.9 / (1.0 + distance / 25.0), 1800000000100));
                index = index + 1;
            }
        }
    """.trimIndent()

    @Test
    fun observationFragmentCompilesToRuntimeCalls() {
        val module = Module.read(compile(observationFragment))
        val calls = module.functions.flatMap { fn -> disassemble(module, fn.name) }
            .filter { "CALL_RUNTIME" in it }
            .map { it.substringAfter("CALL_RUNTIME ") }
            .toSet()
        assertEquals(
            setOf(
                "camera.detectMarkers, 0", "json.arrayLength, 1", "json.arrayObject, 2",
                "json.getInt, 2", "json.getFloat, 2", "json.quote, 1", "text.concat, 2",
                "text.fromInt, 1", "text.fromFloat, 1", "runtime.log, 1",
            ),
            calls,
        )
    }

    @Test
    fun targetFragmentExtractsFiveFields() {
        val bytes = compile(
            """
            fun main() {
                const payload = "{\"targetId\":\"17\",\"latitude\":59.9505,\"longitude\":30.311,\"probability\":0.85,\"lastSeenAt\":1800000000100}";
                const targetId: String = json.getString(payload, "targetId");
                const latitude: Float = json.getFloat(payload, "latitude");
                const longitude: Float = json.getFloat(payload, "longitude");
                const probability: Float = json.getFloat(payload, "probability");
                const lastSeenAt: Int = json.getInt(payload, "lastSeenAt");
                const x = (longitude - 30.308) / (30.314 - 30.308) * 960.0;
                runtime.log(text.concat(targetId, text.fromFloat(x)));
                runtime.log(text.fromInt(lastSeenAt));
                if (probability > 0.5) { runtime.log(text.fromFloat(latitude)); }
            }
            """.trimIndent(),
        )
        assertEquals(2, bytes[4].toInt(), "a Float result selects format v2")
    }

    @Test
    fun helperResultsAreUsableInExpressionsAndArgumentsEvaluateLeftToRight() {
        val module = Module.read(
            compile(inMain("runtime.log(text.concat(text.fromInt(1), text.fromInt(2)));")),
        )
        assertEquals(
            listOf(
                "0 PUSH_CONST 1", "3 CALL_RUNTIME text.fromInt, 1",
                "7 PUSH_CONST 2", "10 CALL_RUNTIME text.fromInt, 1",
                "14 CALL_RUNTIME text.concat, 2", "18 CALL_RUNTIME runtime.log, 1",
                "22 POP", "23 PUSH_CONST 0", "26 RET",
            ),
            disassemble(module, "main"),
        )
    }

    @Test
    fun stringOnlyHelpersKeepFormatV1() {
        val bytes = compile(inMain("runtime.log(text.concat(\"a\", text.fromInt(json.getInt(\"{}\", \"k\"))));"))
        assertEquals(1, bytes[4].toInt())
    }

    @Test
    fun recoverableValidationCompilesWithExistingAbi() {
        val module = Module.read(compile(inMain("""
            const payload = "{\"targetId\":\"17\",\"latitude\":0,\"longitude\":0,\"probability\":0.9,\"lastSeenAt\":1}";
            if (json.validateTarget(payload) == 1) { runtime.log("good"); }
            if (json.validateObservation(payload, 0) == 0) { runtime.log("not observation"); }
        """.trimIndent())))
        val calls = module.functions.flatMap { fn -> disassemble(module, fn.name) }
        assertTrue(calls.any { it.contains("CALL_RUNTIME json.validateTarget, 1") })
        assertTrue(calls.any { it.contains("CALL_RUNTIME json.validateObservation, 2") })
        rejects(inMain("let s = json.validateTarget(1);"),
            "argument 1 of 'json.validateTarget' must be String, found Int")
        rejects(inMain("let s = json.validateObservation(\"{}\", \"1\");"),
            "argument 2 of 'json.validateObservation' must be Int, found String")
    }

    // ---- Negative ----

    @Test
    fun wrongArityIsRejected() {
        rejects(inMain("let n = json.arrayLength();"), "'json.arrayLength' expects 1 argument(s), found 0")
        rejects(inMain("let s = text.concat(\"a\");"), "'text.concat' expects 2 argument(s), found 1")
        rejects(inMain("let s = camera.detectMarkers(1);"), "'camera.detectMarkers' expects 0 argument(s), found 1")
    }

    @Test
    fun wrongArgumentTypesAreRejected() {
        rejects(inMain("let s = json.arrayObject(\"[]\", 0.0);"), "argument 2 of 'json.arrayObject' must be Int, found Float")
        rejects(inMain("let s = json.getInt(\"{}\", 1);"), "argument 2 of 'json.getInt' must be String, found Int")
        rejects(inMain("let s = text.fromFloat(1);"), "argument 1 of 'text.fromFloat' must be Float, found Int")
        rejects(inMain("let s = text.fromInt(1.5);"), "argument 1 of 'text.fromInt' must be Int, found Float")
        rejects(inMain("let s = json.quote(true);"), "argument 1 of 'json.quote' must be String, found Bool")
    }

    @Test
    fun resultTypesAreStatic() {
        rejects(inMain("let x: Int = json.getFloat(\"{}\", \"k\");"), "cannot initialize 'x' of type Int with a value of type Float")
        rejects(
            inMain("let x = json.getFloat(\"{}\", \"k\") + 1;"),
            "operator '+' requires matching Int or Float operands, found Float and Int",
        )
        rejects(inMain("let x = json.getInt(\"{}\", \"k\") == \"17\";"), "operator '==' requires operands of the same type, found Int and String")
    }

    @Test
    fun stringPlusRemainsAnError() =
        rejects(inMain("let s = \"a\" + \"b\";"), "operator '+' requires matching Int or Float operands, found String and String")

    @Test
    fun unknownHelpersAreRejected() {
        rejects(inMain("let s = json.parse(\"{}\");"), "unknown Runtime function 'json.parse'")
        rejects(inMain("let s = text.fromBool(true);"), "unknown Runtime function 'text.fromBool'")
    }

    @Test
    fun syntaxIsUnchanged() {
        assertIs<ParseError>(compileError(inMain("let s = json.a.b(\"{}\");")))
        assertIs<CompileError>(compileError(inMain("let s = json.getInt(\"{}\")[0];")))
    }

    // #144: compile production and supported example sources, and explicitly
    // verify the sole documented unimplemented IPC-poll proposal is rejected.
    @Test
    fun repositoryTcSourcesCompile() {
        val root = File("..").canonicalFile
        val proposedMap = File(root, "docs/examples/runtime-api/map.tc").canonicalFile
        assertTrue(proposedMap.isFile, "The #128 API proposal example is missing")
        val sources = listOf("app", "docs/examples", "tests")
            .map { File(root, it) }
            .flatMap { dir -> dir.walkTopDown().filter { it.isFile && it.extension == "tc" }.toList() }
            .filterNot { it.path.contains("${File.separator}negative${File.separator}") || it.canonicalFile == proposedMap }
        assertTrue(sources.any { it.path.endsWith("app/analyzer/tc/observation.tc") }, "Analyzer source not found")
        assertTrue(sources.any { it.path.endsWith("app/map/tc/map.tc") }, "Windowed Map source not found")
        val brokerTemplate = File(root, "tests/broker/map.tc").canonicalFile
        assertTrue(brokerTemplate.isFile, "Broker source template is missing")
        for (source in sources) {
            // The real Broker E2E injects an ephemeral port before compilation.
            // Use a valid fixture port here, rather than hiding this source.
            val program = if (source.canonicalFile == brokerTemplate) {
                val template = source.readText()
                assertTrue(template.contains("BROKER_PORT"), "Broker port placeholder is missing")
                template.replace("BROKER_PORT", "5000")
            } else {
                source.readText()
            }
            try {
                compile(program)
            } catch (e: CompileError) {
                throw AssertionError("${source.relativeTo(root)}:${e.line}: ${e.message}", e)
            }
        }
        val proposalError = compileError(proposedMap.readText())
        assertIs<SemanticError>(proposalError)
        assertTrue(proposalError.message.orEmpty().contains("runtime.ipc_poll"),
            "Proposal must remain rejected until the IPC polling API is implemented")
    }
}
