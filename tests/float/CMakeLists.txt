# Compiler #107 is a separate task; enable this against its real tacc launcher.
option(TACOS_VM_FLOAT_E2E "Compile Float source with a Compiler #107 launcher" OFF)
if(TACOS_VM_FLOAT_E2E)
    add_test(NAME float_coordinates_e2e
        COMMAND ${CMAKE_COMMAND}
            -DTACC=${TACOS_TACC}
            -DSOURCE=${CMAKE_CURRENT_SOURCE_DIR}/coordinates.tc
            -DTACVM=$<TARGET_FILE:tacvm>
            -DBYTECODE=${CMAKE_CURRENT_BINARY_DIR}/coordinates.bc
            -DEXPECTED=${CMAKE_CURRENT_SOURCE_DIR}/expected.txt
            -P ${CMAKE_SOURCE_DIR}/tests/run_program.cmake)
    set_tests_properties(float_coordinates_e2e PROPERTIES TIMEOUT 30)
endif()
