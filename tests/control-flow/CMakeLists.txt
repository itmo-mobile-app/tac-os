set(CONTROL_FLOW_DIR ${CMAKE_CURRENT_SOURCE_DIR})

# Full chain: compile control-flow.tc, then run the result.
add_test(NAME control_flow_e2e
    COMMAND ${CMAKE_COMMAND}
        -DTACC=${TACOS_TACC}
        -DSOURCE=${CONTROL_FLOW_DIR}/control-flow.tc
        -DTACVM=$<TARGET_FILE:tacvm>
        -DBYTECODE=${CMAKE_CURRENT_BINARY_DIR}/control-flow.bc
        -DEXPECTED=${CONTROL_FLOW_DIR}/expected.txt
        -P ${CMAKE_SOURCE_DIR}/tests/run_program.cmake)

