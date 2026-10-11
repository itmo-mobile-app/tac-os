cmake_minimum_required(VERSION 3.16)
project(tac-os LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

option(TACOS_SANITIZE "Build with AddressSanitizer and UndefinedBehaviorSanitizer" OFF)

# The compiler is a Kotlin project built by Gradle (compiler/); tests call its launcher.
set(TACOS_TACC "${CMAKE_SOURCE_DIR}/compiler/build/install/tacc/bin/tacc"
    CACHE FILEPATH "Path to the tacc compiler launcher (built with ./gradlew installDist)")

add_compile_options(-Wall -Wextra -Wpedantic)

if(TACOS_SANITIZE)
    add_compile_options(-fsanitize=address,undefined -fno-omit-frame-pointer)
    add_link_options(-fsanitize=address,undefined)
endif()

enable_testing()

add_subdirectory(runtime)
add_subdirectory(vm)
add_subdirectory(tests)
