# Required E2E: compiles and runs the real Analyzer source with the real tacc and tacvm.
add_test(NAME analyzer_tc_observation_e2e
    COMMAND "${CMAKE_COMMAND}"
        "-DTACC=${TACOS_TACC}"
        "-DSOURCE=${CMAKE_SOURCE_DIR}/app/analyzer/tc/observation.tc"
        "-DTACVM=$<TARGET_FILE:tacvm>"
        "-DBYTECODE=${CMAKE_CURRENT_BINARY_DIR}/observation.bc"
        "-DEXPECTED=${CMAKE_CURRENT_SOURCE_DIR}/expected.txt"
        -P "${CMAKE_SOURCE_DIR}/tests/run_program.cmake")
set_tests_properties(analyzer_tc_observation_e2e PROPERTIES
    TIMEOUT 60
    LABELS "analyzer;float"
    RESOURCE_LOCK tac_os_ipc_socket)

# Real two-process Analyzer (.tc) -> Map (.tc) IPC, including unavailable Map.
find_package(Python3 REQUIRED COMPONENTS Interpreter)
add_test(NAME analyzer_tc_ipc_e2e
    COMMAND "${Python3_EXECUTABLE}"
        "${CMAKE_CURRENT_SOURCE_DIR}/test_ipc_analyzer.py"
        "${TACOS_TACC}"
        "$<TARGET_FILE:tacvm>"
        "${CMAKE_SOURCE_DIR}/app/analyzer/tc/observation.tc")
set_tests_properties(analyzer_tc_ipc_e2e PROPERTIES
    TIMEOUT 90
    LABELS "analyzer;ipc"
    RESOURCE_LOCK tac_os_ipc_socket)
