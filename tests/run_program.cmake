# Runs a .tc/.bc program and compares its standard output with an expected file.
#
# Required: TACVM, BYTECODE, EXPECTED.
# Optional: TACC and SOURCE — compile SOURCE into BYTECODE first.
#
# A single trailing newline is ignored on both sides: spec/runtime-api.md does not yet
# say whether runtime.log appends one.

if(DEFINED TACC)
    execute_process(
        COMMAND ${TACC} ${SOURCE} -o ${BYTECODE}
        RESULT_VARIABLE compile_result)
    if(NOT compile_result EQUAL 0)
        message(FATAL_ERROR "tacc failed (${compile_result}) on ${SOURCE}")
    endif()
endif()

execute_process(
    COMMAND ${TACVM} ${BYTECODE}
    RESULT_VARIABLE run_result
    OUTPUT_VARIABLE actual)
if(NOT run_result EQUAL 0)
    message(FATAL_ERROR "tacvm failed (${run_result}) on ${BYTECODE}")
endif()

file(READ ${EXPECTED} expected)
string(REGEX REPLACE "\n$" "" actual "${actual}")
string(REGEX REPLACE "\n$" "" expected "${expected}")

if(NOT actual STREQUAL expected)
    message(FATAL_ERROR "unexpected output\n--- expected ---\n${expected}\n--- actual ---\n${actual}")
endif()
