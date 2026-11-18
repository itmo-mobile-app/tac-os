set(WINDOW_RUNTIME_SOURCE "${CMAKE_CURRENT_SOURCE_DIR}/window-manager.tc")
set(WINDOW_RUNTIME_BYTECODE "${CMAKE_CURRENT_BINARY_DIR}/window-manager.bc")

add_custom_command(
    OUTPUT "${WINDOW_RUNTIME_BYTECODE}"
    COMMAND "${TACOS_TACC}" "${WINDOW_RUNTIME_SOURCE}" "${WINDOW_RUNTIME_BYTECODE}"
    DEPENDS "${WINDOW_RUNTIME_SOURCE}"
    COMMENT "Compiling Window Manager .tc integration scenario"
    VERBATIM)

add_custom_target(window_runtime_bytecode
    DEPENDS "${WINDOW_RUNTIME_BYTECODE}")

add_executable(window_runtime_runner runtime_runner.cpp)
target_link_libraries(window_runtime_runner PRIVATE tacos_vm)
add_dependencies(window_runtime_runner window_runtime_bytecode)

add_test(NAME map_wm_e2e
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off"
        "${TACOS_GO_EXECUTABLE}" test -count=1
        -run "^TestMapClientWindowManagerE2E$" ./backend/headless)
set_tests_properties(map_wm_e2e PROPERTIES
    WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/window-manager"
    TIMEOUT 120)

add_test(NAME map_broker_wm_e2e
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off"
        "${TACOS_GO_EXECUTABLE}" test -count=1
        -run "^TestMapBrokerWindowManagerE2E$" ./backend/headless)
set_tests_properties(map_broker_wm_e2e PROPERTIES
    WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/window-manager"
    TIMEOUT 180)

add_test(NAME map_runtime_wm_e2e
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off"
        "TACOS_RUNTIME_E2E_RUNNER=$<TARGET_FILE:window_runtime_runner>"
        "TACOS_RUNTIME_E2E_BYTECODE=${WINDOW_RUNTIME_BYTECODE}"
        "${TACOS_GO_EXECUTABLE}" test -count=1
        -run "^TestRuntimeWindowManagerE2E$" ./backend/headless)
set_tests_properties(map_runtime_wm_e2e PROPERTIES
    WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/window-manager"
    TIMEOUT 60)
