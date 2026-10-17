# The prototype is Go code outside the VM; production Map remains a .tc application.
find_program(TACOS_GO_EXECUTABLE NAMES go)
if(NOT TACOS_GO_EXECUTABLE)
    message(FATAL_ERROR "Go 1.22+ is required to build the Map prototype")
endif()

set(TACOS_MAP_EXECUTABLE "${CMAKE_CURRENT_BINARY_DIR}/map-demo${CMAKE_EXECUTABLE_SUFFIX}")
set(TACOS_MAP_GO_CACHE "${CMAKE_CURRENT_BINARY_DIR}/go-cache")
file(GLOB_RECURSE map_go_sources CONFIGURE_DEPENDS "${CMAKE_CURRENT_SOURCE_DIR}/*.go")
add_custom_command(OUTPUT "${TACOS_MAP_EXECUTABLE}"
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off" "GOCACHE=${TACOS_MAP_GO_CACHE}"
        "${TACOS_GO_EXECUTABLE}" build -buildvcs=false -o "${TACOS_MAP_EXECUTABLE}" ./cmd/map-demo
    WORKING_DIRECTORY "${CMAKE_CURRENT_SOURCE_DIR}"
    DEPENDS ${map_go_sources} "${CMAKE_CURRENT_SOURCE_DIR}/go.mod"
    COMMENT "Building the Go Tactical Map prototype"
    VERBATIM)
add_custom_target(tacos_map ALL DEPENDS "${TACOS_MAP_EXECUTABLE}")

add_test(NAME map_unit
    COMMAND "${CMAKE_COMMAND}" -E env
        "GOWORK=off" "GOCACHE=${TACOS_MAP_GO_CACHE}"
        "${TACOS_GO_EXECUTABLE}" test -count=1 ./...)
set_tests_properties(map_unit PROPERTIES WORKING_DIRECTORY "${CMAKE_CURRENT_SOURCE_DIR}")

add_test(NAME map_preview
    COMMAND "${TACOS_MAP_EXECUTABLE}"
        -config "${CMAKE_CURRENT_SOURCE_DIR}/assets/test-map.json"
        -preview "${CMAKE_CURRENT_BINARY_DIR}/map-preview.png")
