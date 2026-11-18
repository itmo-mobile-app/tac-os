find_package(Python3 REQUIRED COMPONENTS Interpreter)

add_executable(device_fixture_vm fixture_vm.cpp)
target_link_libraries(device_fixture_vm PRIVATE tacos_vm)

add_test(NAME device_vm_e2e
    COMMAND "${Python3_EXECUTABLE}" "${CMAKE_CURRENT_SOURCE_DIR}/test_device.py"
        "${TACOS_TACC}" "$<TARGET_FILE:tacvm>" "$<TARGET_FILE:device_fixture_vm>"
)
set_tests_properties(device_vm_e2e PROPERTIES TIMEOUT 60)
