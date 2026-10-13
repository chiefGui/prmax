cmake_minimum_required(VERSION 3.16)
project(prmax_desktop LANGUAGES C CXX)

add_subdirectory(vendor/vogue)

add_executable(prmax WIN32
    src/main.c
    src/app/app.c
    src/app/model.c
    src/platform/gfx.cpp
    src/platform/net.c
    src/ui/board.c
    src/ui/clay.c
    src/ui/dialog.c
    src/ui/kit.c
    src/ui/panel.c
    src/ui/ui.c
    vendor/yyjson/yyjson.c
    res/prmax.rc
)

target_include_directories(prmax PRIVATE src vendor/clay vendor/yyjson)
target_compile_features(prmax PRIVATE c_std_11 cxx_std_17)
target_compile_definitions(prmax PRIVATE UNICODE _UNICODE WIN32_LEAN_AND_MEAN NOMINMAX _CRT_SECURE_NO_WARNINGS)
target_link_libraries(prmax PRIVATE vogue::vogue d2d1 dwrite winhttp dwmapi shell32)
set_property(TARGET prmax PROPERTY MSVC_RUNTIME_LIBRARY "MultiThreaded")

if(MSVC)
    target_compile_options(prmax PRIVATE /W3 /utf-8)
    set_source_files_properties(src/ui/clay.c vendor/yyjson/yyjson.c PROPERTIES COMPILE_OPTIONS /W0)
endif()
