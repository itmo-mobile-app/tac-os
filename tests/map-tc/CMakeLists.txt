# Required E2E: do not turn absent Float support into a passing/skipped test.
add_test(NAME map_tc_projection_e2e
    COMMAND "${CMAKE_COMMAND}"
        "-DTACC=${TACOS_TACC}"
        "-DSOURCE=${CMAKE_SOURCE_DIR}/app/map/tc/projection.tc"
        "-DTACVM=$<TARGET_FILE:tacvm>"
        "-DBYTECODE=${CMAKE_CURRENT_BINARY_DIR}/projection.bc"
        "-DEXPECTED=${CMAKE_CURRENT_SOURCE_DIR}/expected.txt"
        -P "${CMAKE_SOURCE_DIR}/tests/run_program.cmake")
set_tests_properties(map_tc_projection_e2e PROPERTIES
    TIMEOUT 60
    LABELS "map;float")
