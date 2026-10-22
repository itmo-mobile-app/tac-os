find_package(Python3 REQUIRED COMPONENTS Interpreter)
add_test(NAME vm_instructions
    COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/test_vm.py $<TARGET_FILE:tacvm>)
set_tests_properties(vm_instructions PROPERTIES TIMEOUT 60)

# Both compiler and VM language features are integrated; enable E2E by default.
option(TACOS_VM_LANGUAGE_E2E "Test VM with the Compiler #63 language features" ON)
if(TACOS_VM_LANGUAGE_E2E)
    foreach(scenario control-flow language-example recursion)
        add_test(NAME vm_${scenario}_e2e
            COMMAND ${CMAKE_COMMAND}
                -DTACC=${TACOS_TACC}
                -DSOURCE=${CMAKE_CURRENT_SOURCE_DIR}/${scenario}.tc
                -DTACVM=$<TARGET_FILE:tacvm>
                -DBYTECODE=${CMAKE_CURRENT_BINARY_DIR}/${scenario}.bc
                -DEXPECTED=${CMAKE_CURRENT_SOURCE_DIR}/${scenario}.expected
                -P ${CMAKE_SOURCE_DIR}/tests/run_program.cmake)
        set_tests_properties(vm_${scenario}_e2e PROPERTIES TIMEOUT 30)
    endforeach()
endif()
