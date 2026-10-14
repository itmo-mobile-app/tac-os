# CMake orchestrates the Go component alongside the C++ VM and Runtime.
if(CMAKE_SOURCE_DIR STREQUAL CMAKE_CURRENT_SOURCE_DIR)
    cmake_minimum_required(VERSION 3.16)
    project(tacos-dbms LANGUAGES NONE)
    enable_testing()
    set(TACOS_DBMS_STANDALONE ON)
endif()

find_program(TACOS_GO_EXECUTABLE NAMES go)
if(NOT TACOS_GO_EXECUTABLE)
    message(FATAL_ERROR "Go 1.22+ is required to build the DBMS")
endif()
execute_process(COMMAND "${TACOS_GO_EXECUTABLE}" env GOVERSION
    OUTPUT_VARIABLE go_version OUTPUT_STRIP_TRAILING_WHITESPACE
    RESULT_VARIABLE go_status)
string(REGEX REPLACE "^go" "" go_version "${go_version}")
if(NOT go_status EQUAL 0 OR go_version VERSION_LESS "1.22")
    message(FATAL_ERROR "Go 1.22+ is required; found ${go_version}")
endif()

set(TACOS_DBMS_SOURCE_DIR "${CMAKE_CURRENT_SOURCE_DIR}")
set(TACOS_DBMS_GO_CACHE "${CMAKE_CURRENT_BINARY_DIR}/go-cache")
set(TACOS_DBMS_EXECUTABLE "${CMAKE_CURRENT_BINARY_DIR}/tacdb${CMAKE_EXECUTABLE_SUFFIX}")
file(GLOB_RECURSE dbms_go_sources CONFIGURE_DEPENDS "${CMAKE_CURRENT_SOURCE_DIR}/*.go")
add_custom_command(OUTPUT "${TACOS_DBMS_EXECUTABLE}"
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off" "GOCACHE=${TACOS_DBMS_GO_CACHE}"
        "${TACOS_GO_EXECUTABLE}" build -buildvcs=false -o "${TACOS_DBMS_EXECUTABLE}" ./cmd/tacdb
    WORKING_DIRECTORY "${CMAKE_CURRENT_SOURCE_DIR}"
    DEPENDS ${dbms_go_sources} "${CMAKE_CURRENT_SOURCE_DIR}/go.mod"
    COMMENT "Building the Go SQL DBMS"
    VERBATIM)
add_custom_target(tacos_dbms ALL DEPENDS "${TACOS_DBMS_EXECUTABLE}")

if(NOT TACOS_DBMS_STANDALONE)
    foreach(variable TACOS_GO_EXECUTABLE TACOS_DBMS_SOURCE_DIR TACOS_DBMS_GO_CACHE TACOS_DBMS_EXECUTABLE)
        set(${variable} "${${variable}}" PARENT_SCOPE)
    endforeach()
endif()

if(TACOS_DBMS_STANDALONE)
    add_subdirectory("${CMAKE_CURRENT_SOURCE_DIR}/../../tests/dbms"
                     "${CMAKE_CURRENT_BINARY_DIR}/dbms-tests")
endif()
