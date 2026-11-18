# Runtime is linked into the VM executable (docs/ai-context/01-architecture.md).
add_library(tacos_runtime STATIC
    src/runtime.cpp
    src/broker_client.cpp
    src/device_state.cpp
    src/data_helpers.cpp
    src/camera_detector_client.cpp
    src/window_manager_client.cpp)


include(FetchContent)

FetchContent_Declare(
    nlohmann_json
    URL https://github.com/nlohmann/json/releases/download/v3.12.0/json.tar.xz
)

FetchContent_MakeAvailable(nlohmann_json)

target_link_libraries(
    tacos_runtime
    PRIVATE nlohmann_json::nlohmann_json
)

target_include_directories(tacos_runtime PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}/include)

add_executable(runtime_unit_tests
    tests/runtime_tests.cpp
)

target_link_libraries(runtime_unit_tests
    PRIVATE tacos_runtime
)

add_test(
    NAME runtime_unit
    COMMAND runtime_unit_tests
)

find_package(Threads REQUIRED)
add_executable(runtime_data_helper_tests
    tests/data_helpers_tests.cpp
)
target_link_libraries(runtime_data_helper_tests
    PRIVATE tacos_runtime Threads::Threads
)
add_test(
    NAME runtime_data_helpers
    COMMAND runtime_data_helper_tests
)

add_subdirectory(tests/camera)
