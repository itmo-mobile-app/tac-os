# tacvm: loads and executes a .bc file, with the Runtime linked in.
add_executable(tacvm src/main.cpp)
target_link_libraries(tacvm PRIVATE tacos_runtime)

# Honor the Float contract, including caller rounding modes and separate operations.
if(CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
    target_compile_options(tacvm PRIVATE -frounding-math -ffp-contract=off)
endif()
