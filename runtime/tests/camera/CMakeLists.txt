# This small test can also run before the Compiler and C++ Runtime are built.
if(CMAKE_SOURCE_DIR STREQUAL CMAKE_CURRENT_SOURCE_DIR)
    cmake_minimum_required(VERSION 3.16)
    project(tacos-camera-helper-tests LANGUAGES NONE)
    enable_testing()
endif()

find_program(TACOS_GO_EXECUTABLE NAMES go)
if(NOT TACOS_GO_EXECUTABLE)
    message(FATAL_ERROR "Go is required to build the camera test helper")
endif()
find_package(Python3 REQUIRED COMPONENTS Interpreter)

set(detector_dir "${CMAKE_CURRENT_SOURCE_DIR}/../../../prototypes/marker-detect")
set(TACOS_CAMERA_FAKE_HELPER "${CMAKE_CURRENT_BINARY_DIR}/marker-detect")
file(GLOB camera_helper_sources CONFIGURE_DEPENDS
    "${detector_dir}/internal/runtimehelper/*.go"
    "${detector_dir}/ipc/*.go"
    "${detector_dir}/marker/*.go")
add_custom_command(OUTPUT "${TACOS_CAMERA_FAKE_HELPER}"
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off" "CGO_ENABLED=0" "GOPROXY=off"
        "GOCACHE=${CMAKE_CURRENT_BINARY_DIR}/go-cache"
        "${TACOS_GO_EXECUTABLE}" build -buildvcs=false
        -o "${TACOS_CAMERA_FAKE_HELPER}" ./internal/runtimehelper
    WORKING_DIRECTORY "${detector_dir}"
    DEPENDS ${camera_helper_sources} "${detector_dir}/go.mod"
    COMMENT "Building the camera test helper without OpenCV"
    VERBATIM)
add_custom_target(camera_fake_helper ALL DEPENDS "${TACOS_CAMERA_FAKE_HELPER}")

add_test(NAME camera_helper_contract
    COMMAND "${Python3_EXECUTABLE}" "${CMAKE_CURRENT_SOURCE_DIR}/helper_test.py"
        "${TACOS_CAMERA_FAKE_HELPER}")
set_tests_properties(camera_helper_contract PROPERTIES TIMEOUT 45)

if(TARGET tacos_runtime)
    add_executable(camera_runtime_client runtime_client.cpp)
    target_link_libraries(camera_runtime_client PRIVATE tacos_runtime)
    add_dependencies(camera_runtime_client camera_fake_helper)
    add_test(NAME camera_runtime_contract
        COMMAND "${Python3_EXECUTABLE}" "${CMAKE_CURRENT_SOURCE_DIR}/runtime_test.py"
            "$<TARGET_FILE:camera_runtime_client>" "${TACOS_CAMERA_FAKE_HELPER}")
    set_tests_properties(camera_runtime_contract PROPERTIES TIMEOUT 120)
endif()
