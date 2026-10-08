# Runtime is linked into the VM executable (docs/ai-context/01-architecture.md).
add_library(tacos_runtime STATIC
    src/runtime.cpp)

target_include_directories(tacos_runtime PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}/include)
