cmake_minimum_required(VERSION 3.5.0)
project(HomeCast)

set(NATIVERENDER_ROOT_PATH ${CMAKE_CURRENT_SOURCE_DIR})
include_directories(${NATIVERENDER_ROOT_PATH}/include)

add_library(entry SHARED napi_init.cpp)
# 不把 Windows 构建机的 .so 全路径写入 DT_NEEDED / RUNPATH；HAP 内只按库名加载。
set(CMAKE_SKIP_RPATH TRUE)
add_library(homecast_go SHARED IMPORTED GLOBAL)
set_target_properties(homecast_go PROPERTIES
    IMPORTED_LOCATION ${NATIVERENDER_ROOT_PATH}/libs/${OHOS_ARCH}/libhomecast_go.so
    IMPORTED_NO_SONAME TRUE)

target_link_libraries(entry PUBLIC
    libace_napi.z.so
    libhilog_ndk.z.so
    homecast_go
)
