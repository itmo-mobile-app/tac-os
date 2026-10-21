set(LANGUAGE_EXAMPLE_DIR ${CMAKE_CURRENT_SOURCE_DIR})

# Full chain: compile the spec/language.md example, then run the result.
add_test(NAME language_example_e2e
    COMMAND ${CMAKE_COMMAND}
        -DTACC=${TACOS_TACC}
        -DSOURCE=${LANGUAGE_EXAMPLE_DIR}/language-example.tc
        -DTACVM=$<TARGET_FILE:tacvm>
        -DBYTECODE=${CMAKE_CURRENT_BINARY_DIR}/language-example.bc
        -DEXPECTED=${LANGUAGE_EXAMPLE_DIR}/expected.txt
        -P ${CMAKE_SOURCE_DIR}/tests/run_program.cmake)

# Disabled until the VM executes the #62 instructions and CALL between .tc functions
# (CALL is not part of #62).
set_tests_properties(language_example_e2e PROPERTIES DISABLED TRUE)
