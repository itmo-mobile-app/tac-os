# Execution is reusable from C++ without invoking or embedding the CLI.
add_library(tacos_vm STATIC src/vm.cpp)
target_include_directories(tacos_vm PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/include)
target_link_libraries(tacos_vm PUBLIC tacos_runtime)

add_executable(tacvm src/main.cpp)
target_link_libraries(tacvm PRIVATE tacos_vm)

if(CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
    target_compile_options(tacos_vm PRIVATE -frounding-math -ffp-contract=off)
endif()
