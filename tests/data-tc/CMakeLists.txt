# Compiler #132 / VM #133 E2E for the Observation/Target String/JSON helpers: real tacc and tacvm.
find_package(Python3 REQUIRED COMPONENTS Interpreter)

add_test(
    NAME data_tc_e2e
    COMMAND
        "${Python3_EXECUTABLE}"
        "${CMAKE_CURRENT_SOURCE_DIR}/test_data_tc.py"
        "${TACOS_TACC}"
        "$<TARGET_FILE:tacvm>"
)

set_tests_properties(data_tc_e2e PROPERTIES
    TIMEOUT 180
    LABELS "data;float")
