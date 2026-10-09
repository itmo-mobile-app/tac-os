file(MAKE_DIRECTORY "${OUTPUT_DIR}/cli-input")
# Preserve the fixture's intended LF characters even after a CRLF checkout.
foreach(fixture cli.sql cli-errors.sql cli-incomplete.sql cli-unclosed.sql)
    file(READ "${TEST_DIR}/${fixture}" source)
    string(REPLACE "\r\n" "\n" source "${source}")
    file(WRITE "${OUTPUT_DIR}/cli-input/${fixture}" "${source}")
endforeach()

execute_process(COMMAND "${DBMS}"
    INPUT_FILE "${OUTPUT_DIR}/cli-input/cli.sql"
    OUTPUT_VARIABLE actual ERROR_VARIABLE error RESULT_VARIABLE status)
file(READ "${TEST_DIR}/cli.expected" expected)
string(REPLACE "\r\n" "\n" expected "${expected}")
if(NOT status EQUAL 0 OR NOT error STREQUAL "" OR NOT actual STREQUAL expected)
    message(FATAL_ERROR "CLI scenario failed: status=${status}\nstdout=${actual}\nstderr=${error}")
endif()

# Check that a failed statement does not prevent the next framed statement.
execute_process(COMMAND "${DBMS}"
    INPUT_FILE "${OUTPUT_DIR}/cli-input/cli-errors.sql"
    OUTPUT_VARIABLE actual ERROR_VARIABLE error RESULT_VARIABLE status)
if(NOT status EQUAL 1 OR NOT actual STREQUAL "OK\nOK\nx\n7\n1 row(s)\n")
    message(FATAL_ERROR "CLI error recovery failed: status=${status}\nstdout=${actual}\nstderr=${error}")
endif()
if(NOT error MATCHES "ERROR at position [0-9]+:.*unsupported")
    message(FATAL_ERROR "CLI did not report the unsupported statement position: ${error}")
endif()

execute_process(COMMAND "${DBMS}"
    INPUT_FILE "${OUTPUT_DIR}/cli-input/cli-incomplete.sql"
    OUTPUT_VARIABLE actual ERROR_VARIABLE error RESULT_VARIABLE status)
if(NOT status EQUAL 1 OR NOT actual STREQUAL "OK\n" OR NOT error MATCHES "Expected ';'")
    message(FATAL_ERROR "CLI accepted incomplete SQL: status=${status}\nstdout=${actual}\nstderr=${error}")
endif()

execute_process(COMMAND "${DBMS}"
    INPUT_FILE "${OUTPUT_DIR}/cli-input/cli-unclosed.sql"
    OUTPUT_VARIABLE actual ERROR_VARIABLE error RESULT_VARIABLE status)
if(NOT status EQUAL 1 OR NOT error MATCHES "Unterminated TEXT")
    message(FATAL_ERROR "CLI accepted an unterminated string: status=${status}\nstderr=${error}")
endif()
