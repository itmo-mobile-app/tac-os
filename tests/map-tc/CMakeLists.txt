# Required E2E: do not turn absent Float support into a passing/skipped test.
add_test(NAME map_tc_projection_e2e
    COMMAND "${CMAKE_COMMAND}"
        "-DTACC=${TACOS_TACC}"
        "-DSOURCE=${CMAKE_SOURCE_DIR}/app/map/tc/projection.tc"
        "-DTACVM=$<TARGET_FILE:tacvm>"
        "-DBYTECODE=${CMAKE_CURRENT_BINARY_DIR}/projection.bc"
        "-DEXPECTED=${CMAKE_CURRENT_SOURCE_DIR}/expected.txt"
        -P "${CMAKE_SOURCE_DIR}/tests/run_program.cmake")
set_tests_properties(map_tc_projection_e2e PROPERTIES
    TIMEOUT 60
    LABELS "map;float")

# #139 uses #136's real Runtime/VM host with isolated WM socket paths.
# Keep this required: missing window support must not silently skip acceptance.
if(NOT TARGET window_runtime_runner)
    message(FATAL_ERROR "Map #139 requires the Window Runtime implementation from #136 / PR #156")
endif()

set(MAP_TC_SOURCE "${CMAKE_SOURCE_DIR}/app/map/tc/map.tc")
set(MAP_TC_BYTECODE "${CMAKE_CURRENT_BINARY_DIR}/map.bc")
add_custom_command(OUTPUT "${MAP_TC_BYTECODE}"
    COMMAND "${TACOS_TACC}" "${MAP_TC_SOURCE}" "${MAP_TC_BYTECODE}"
    DEPENDS "${MAP_TC_SOURCE}"
    COMMENT "Compiling the Tactical Map screen on .tc"
    VERBATIM)
add_custom_target(tacos_map_tc ALL DEPENDS "${MAP_TC_BYTECODE}")

add_test(NAME map_tc_window_e2e
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off"
        "TACOS_MAP_TC_TACC=${TACOS_TACC}"
        "TACOS_MAP_TC_RUNNER=$<TARGET_FILE:window_runtime_runner>"
        "TACOS_MAP_TC_BYTECODE=${MAP_TC_BYTECODE}"
        "${TACOS_GO_EXECUTABLE}" test -race -count=1
        -run "^TestTacticalMapTcE2E$" ./backend/headless)
set_tests_properties(map_tc_window_e2e PROPERTIES
    WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/window-manager"
    TIMEOUT 120
    LABELS "map;window;tc")
