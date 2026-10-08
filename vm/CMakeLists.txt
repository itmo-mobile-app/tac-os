# tacvm: loads and executes a .bc file, with the Runtime linked in.
add_executable(tacvm src/main.cpp)
target_link_libraries(tacvm PRIVATE tacos_runtime)
