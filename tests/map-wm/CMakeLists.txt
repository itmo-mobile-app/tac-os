add_test(NAME map_wm_e2e
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off"
        "${TACOS_GO_EXECUTABLE}" test -count=1
        -run "^TestMapClientWindowManagerE2E$" ./backend/headless)
set_tests_properties(map_wm_e2e PROPERTIES
    WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/window-manager"
    TIMEOUT 120)
