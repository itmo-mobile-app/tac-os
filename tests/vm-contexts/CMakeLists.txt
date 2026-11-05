find_package(Threads REQUIRED)
find_package(Python3 REQUIRED COMPONENTS Interpreter)
add_executable(vm_context_tests contexts.cpp)
target_link_libraries(vm_context_tests PRIVATE tacos_vm Threads::Threads)
add_test(NAME vm_contexts
    COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/test_contexts.py
        ${TACOS_TACC} $<TARGET_FILE:vm_context_tests>)
set_tests_properties(vm_contexts PROPERTIES TIMEOUT 90)
