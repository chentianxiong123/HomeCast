#include "napi/native_api.h"
#include "hilog/log.h"
#include "libhomecast_go.h"

#include <vector>

static napi_value StartServerNative(napi_env env, napi_callback_info info) {
    size_t argc = 1;
    napi_value argv[1] = {nullptr};
    if (napi_get_cb_info(env, info, &argc, argv, nullptr, nullptr) != napi_ok || argc < 1) {
        napi_throw_type_error(env, nullptr, "startServer(filesDir) requires a sandbox directory");
        return nullptr;
    }

    size_t len = 0;
    if (napi_get_value_string_utf8(env, argv[0], nullptr, 0, &len) != napi_ok) {
        napi_throw_type_error(env, nullptr, "filesDir must be a string");
        return nullptr;
    }
    std::vector<char> dataDir(len + 1, '\0');
    if (napi_get_value_string_utf8(env, argv[0], dataDir.data(), len + 1, &len) != napi_ok) {
        napi_throw_type_error(env, nullptr, "filesDir must be a valid UTF-8 string");
        return nullptr;
    }

    OH_LOG_Print(LOG_APP, LOG_INFO, 0x0000, "HomeCast",
                 "StartServer dataDir=%{public}s", dataDir.data());
    // dataDir 的所有权只在调用期间使用；Go 侧立即复制为 Go string。
    StartServer(dataDir.data());
    napi_value result;
    napi_get_boolean(env, true, &result);
    return result;
}

EXTERN_C_START
static napi_value Init(napi_env env, napi_value exports) {
    OH_LOG_Print(LOG_APP, LOG_INFO, 0x0000, "HomeCast", "NAPI Init called");
    napi_property_descriptor desc[] = {
        {"startServer", nullptr, StartServerNative, nullptr, nullptr, nullptr, napi_default, nullptr}
    };
    napi_status status = napi_define_properties(env, exports, sizeof(desc) / sizeof(desc[0]), desc);
    OH_LOG_Print(LOG_APP, LOG_INFO, 0x0000, "HomeCast",
                 "NAPI exports initialized: status=%{public}d", static_cast<int>(status));
    return exports;
}
EXTERN_C_END

static napi_module homecastModule = {
    .nm_version = 1,
    .nm_flags = 0,
    .nm_filename = nullptr,
    .nm_register_func = Init,
    .nm_modname = "entry",
    .nm_priv = nullptr,
    .reserved = {0}
};

extern "C" __attribute__((constructor)) void RegisterEntryModule(void) {
    OH_LOG_Print(LOG_APP, LOG_INFO, 0x0000, "HomeCast", "RegisterEntryModule called");
    napi_module_register(&homecastModule);
}
