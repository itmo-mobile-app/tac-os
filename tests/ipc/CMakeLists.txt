
find_package(Python3 REQUIRED COMPONENTS Interpreter)

add_test(
    NAME ipc_vm_e2e
    COMMAND
        "${Python3_EXECUTABLE}"
        "${CMAKE_CURRENT_SOURCE_DIR}/test_ipc.py"
        "${TACOS_TACC}"
        "$<TARGET_FILE:tacvm>"
)

set_tests_properties(ipc_vm_e2e PROPERTIES
    TIMEOUT 90
    RESOURCE_LOCK tac_os_ipc_socket
)
