find_package(Threads REQUIRED)
add_executable(vm_value_tests values.cpp)
target_link_libraries(vm_value_tests PRIVATE tacos_vm Threads::Threads)
add_test(NAME vm_value_boundary COMMAND vm_value_tests)
set_tests_properties(vm_value_boundary PROPERTIES TIMEOUT 60)

# Enabled only after Compiler #132 and the real approved helpers are available.
option(TACOS_VM_DATA_E2E "Run real Observation/Target helper E2E after #127/#132" OFF)
if(TACOS_VM_DATA_E2E)
    find_package(Python3 REQUIRED COMPONENTS Interpreter)
    add_test(NAME vm_data_e2e
        COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/test_data.py
            ${TACOS_TACC} $<TARGET_FILE:tacvm>)
    set_tests_properties(vm_data_e2e PROPERTIES TIMEOUT 90)
endif()
