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
    LABELS "analyzer;float")
