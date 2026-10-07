set(HELLO_DIR ${CMAKE_CURRENT_SOURCE_DIR})

# VM and Runtime only: run the hand-assembled reference bytecode.
add_test(NAME hello_vm
    COMMAND ${CMAKE_COMMAND}
        -DTACVM=$<TARGET_FILE:tacvm>
        -DBYTECODE=${HELLO_DIR}/hello.bc
        -DEXPECTED=${HELLO_DIR}/expected.txt
        -P ${CMAKE_SOURCE_DIR}/tests/run_program.cmake)

# Full chain: compile hello.tc, then run the result.
add_test(NAME hello_e2e
    COMMAND ${CMAKE_COMMAND}
        -DTACC=$<TARGET_FILE:tacc>
        -DSOURCE=${HELLO_DIR}/hello.tc
        -DTACVM=$<TARGET_FILE:tacvm>
        -DBYTECODE=${CMAKE_CURRENT_BINARY_DIR}/hello.bc
        -DEXPECTED=${HELLO_DIR}/expected.txt
        -P ${CMAKE_SOURCE_DIR}/tests/run_program.cmake)

# Disabled until the VM (#29) and the compiler (#28) can run hello world.
# Remove a test from this list once the component it needs is merged.
set_tests_properties(hello_vm hello_e2e PROPERTIES DISABLED TRUE)
