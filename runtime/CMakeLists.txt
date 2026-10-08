# Runtime is linked into the VM executable (docs/ai-context/01-architecture.md).
add_library(tacos_runtime STATIC
    src/runtime.cpp)

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
