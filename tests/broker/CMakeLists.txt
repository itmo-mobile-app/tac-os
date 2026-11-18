find_package(Python3 REQUIRED COMPONENTS Interpreter)

set(broker_source "${CMAKE_SOURCE_DIR}/broker")
set(broker_executable "${CMAKE_CURRENT_BINARY_DIR}/tac-broker")
add_custom_command(
    OUTPUT "${broker_executable}"
    COMMAND "${CMAKE_COMMAND}" -E env "GOWORK=off"
        "${TACOS_GO_EXECUTABLE}" build -buildvcs=false -o "${broker_executable}" .
    WORKING_DIRECTORY "${broker_source}"
    DEPENDS "${broker_source}/go.mod" "${broker_source}/main.go"
    VERBATIM
)
add_custom_target(broker_test_server ALL DEPENDS "${broker_executable}")

add_test(
    NAME broker_vm_e2e
    COMMAND "${Python3_EXECUTABLE}" "${CMAKE_CURRENT_SOURCE_DIR}/test_broker.py"
        "${TACOS_TACC}" "$<TARGET_FILE:tacvm>" "${broker_executable}"
)
set_tests_properties(broker_vm_e2e PROPERTIES TIMEOUT 60)
