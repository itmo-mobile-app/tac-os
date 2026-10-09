add_executable(tacos_dbms_tests dbms_tests.cpp)
target_link_libraries(tacos_dbms_tests PRIVATE tacos_dbms)

foreach(case create insert select literals syntax semantic atomicity independent)
    add_test(NAME dbms.${case} COMMAND tacos_dbms_tests ${case})
    set_tests_properties(dbms.${case} PROPERTIES LABELS dbms)
endforeach()

add_test(NAME dbms.cli
    COMMAND "${CMAKE_COMMAND}"
        "-DDBMS=$<TARGET_FILE:tacdb>"
        "-DTEST_DIR=${CMAKE_CURRENT_SOURCE_DIR}"
        "-DOUTPUT_DIR=${CMAKE_CURRENT_BINARY_DIR}"
        -P "${CMAKE_CURRENT_SOURCE_DIR}/cli_test.cmake")
set_tests_properties(dbms.cli PROPERTIES LABELS dbms)
