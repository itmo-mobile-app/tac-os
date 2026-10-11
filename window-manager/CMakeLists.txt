find_package(PkgConfig REQUIRED)
find_package(Threads REQUIRED)

pkg_check_modules(WLROOTS REQUIRED IMPORTED_TARGET wlroots)
pkg_check_modules(WAYLAND_SERVER REQUIRED IMPORTED_TARGET wayland-server)
pkg_check_modules(WAYLAND_CLIENT REQUIRED IMPORTED_TARGET wayland-client)
pkg_check_modules(WAYLAND_PROTOCOLS REQUIRED wayland-protocols)
find_program(WAYLAND_SCANNER wayland-scanner)
if(NOT WAYLAND_SCANNER)
    message(FATAL_ERROR "wayland-scanner is required to build tac_wm_window_client")
endif()

pkg_get_variable(WAYLAND_PROTOCOLS_DIR wayland-protocols pkgdatadir)
set(XDG_SHELL_XML "${WAYLAND_PROTOCOLS_DIR}/stable/xdg-shell/xdg-shell.xml")
set(XDG_SHELL_CLIENT_HEADER "${CMAKE_CURRENT_BINARY_DIR}/xdg-shell-client-protocol.h")
set(XDG_SHELL_PRIVATE_CODE "${CMAKE_CURRENT_BINARY_DIR}/xdg-shell-protocol.c")
set(XDG_SHELL_SERVER_HEADER "${CMAKE_CURRENT_BINARY_DIR}/xdg-shell-protocol.h")

add_custom_command(
    OUTPUT "${XDG_SHELL_SERVER_HEADER}"
    COMMAND "${WAYLAND_SCANNER}" server-header "${XDG_SHELL_XML}" "${XDG_SHELL_SERVER_HEADER}"
    DEPENDS "${XDG_SHELL_XML}"
)
add_custom_command(
    OUTPUT "${XDG_SHELL_CLIENT_HEADER}"
    COMMAND "${WAYLAND_SCANNER}" client-header "${XDG_SHELL_XML}" "${XDG_SHELL_CLIENT_HEADER}"
    DEPENDS "${XDG_SHELL_XML}")
add_custom_command(
    OUTPUT "${XDG_SHELL_PRIVATE_CODE}"
    COMMAND "${WAYLAND_SCANNER}" private-code "${XDG_SHELL_XML}" "${XDG_SHELL_PRIVATE_CODE}"
    DEPENDS "${XDG_SHELL_XML}")

add_executable(tac_wm
    src/main.cpp
    src/control_server.cpp
    src/compositor.cpp
    "${XDG_SHELL_SERVER_HEADER}")
target_include_directories(tac_wm PRIVATE include "${CMAKE_CURRENT_BINARY_DIR}")
target_compile_features(tac_wm PRIVATE cxx_std_17)
target_compile_definitions(tac_wm PRIVATE WLR_USE_UNSTABLE)
target_link_libraries(tac_wm PRIVATE PkgConfig::WLROOTS PkgConfig::WAYLAND_SERVER Threads::Threads)

add_executable(tac_wm_window_client
    src/window_client.cpp
    "${XDG_SHELL_CLIENT_HEADER}"
    "${XDG_SHELL_PRIVATE_CODE}")
target_include_directories(tac_wm_window_client PRIVATE "${CMAKE_CURRENT_BINARY_DIR}")
target_compile_features(tac_wm_window_client PRIVATE cxx_std_17)
target_link_libraries(tac_wm_window_client PRIVATE PkgConfig::WAYLAND_CLIENT)

add_executable(tac_wm_test_client src/test_client.cpp)
target_compile_features(tac_wm_test_client PRIVATE cxx_std_17)
target_include_directories(tac_wm_test_client PRIVATE include)
