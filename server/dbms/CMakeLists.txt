# Allow the component to be verified without building unrelated components.
if(CMAKE_SOURCE_DIR STREQUAL CMAKE_CURRENT_SOURCE_DIR)
    cmake_minimum_required(VERSION 3.16)
    project(tacos-dbms LANGUAGES CXX)
    set(CMAKE_CXX_STANDARD 17)
    set(CMAKE_CXX_STANDARD_REQUIRED ON)
    set(CMAKE_CXX_EXTENSIONS OFF)
    option(TACOS_SANITIZE "Build with AddressSanitizer and UndefinedBehaviorSanitizer" OFF)
    add_compile_options(-Wall -Wextra -Wpedantic)
    if(TACOS_SANITIZE)
        add_compile_options(-fsanitize=address,undefined -fno-omit-frame-pointer)
        add_link_options(-fsanitize=address,undefined)
    endif()
    enable_testing()
    set(TACOS_DBMS_STANDALONE ON)
endif()

add_library(tacos_dbms
    src/lexer.cpp
    src/parser.cpp
    src/database.cpp
)
target_include_directories(tacos_dbms PUBLIC include)
target_compile_features(tacos_dbms PUBLIC cxx_std_17)

add_executable(tacdb src/main.cpp)
target_link_libraries(tacdb PRIVATE tacos_dbms)

if(TACOS_DBMS_STANDALONE)
    add_subdirectory("${CMAKE_CURRENT_SOURCE_DIR}/../../tests/dbms"
                     "${CMAKE_CURRENT_BINARY_DIR}/dbms-tests")
endif()
