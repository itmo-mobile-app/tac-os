foreach(case Create Insert Select Update UpdateErrors Literals Syntax Semantic Atomicity Independent)
    string(TOLOWER "${case}" name)
    add_test(NAME dbms.${name}
        COMMAND "${CMAKE_COMMAND}" -E env
            "GOWORK=off" "GOCACHE=${TACOS_DBMS_GO_CACHE}"
            "${TACOS_GO_EXECUTABLE}" test -count=1 -run "^Test${case}$" .)
    set_tests_properties(dbms.${name} PROPERTIES
        WORKING_DIRECTORY "${TACOS_DBMS_SOURCE_DIR}" LABELS dbms)
endforeach()

add_test(NAME dbms.cli-unit
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off" "GOCACHE=${TACOS_DBMS_GO_CACHE}"
        "${TACOS_GO_EXECUTABLE}" test -count=1 ./internal/cli)
set_tests_properties(dbms.cli-unit PROPERTIES
    WORKING_DIRECTORY "${TACOS_DBMS_SOURCE_DIR}" LABELS dbms)

add_test(NAME dbms.cli
    COMMAND "${CMAKE_COMMAND}"
        "-DDBMS=${TACOS_DBMS_EXECUTABLE}"
        "-DTEST_DIR=${CMAKE_CURRENT_SOURCE_DIR}"
        "-DOUTPUT_DIR=${CMAKE_CURRENT_BINARY_DIR}"
        -P "${CMAKE_CURRENT_SOURCE_DIR}/cli_test.cmake")
set_tests_properties(dbms.cli PROPERTIES LABELS dbms)
