package com.homecast.app;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;

/**
 * homecast 安卓壳：内嵌 Go 服务端（assets/hc-server，GOOS=android arm64 交叉编译）
 * 启动流程：拷出二进制 → 拉起监听 127.0.0.1:28976 → 轮询就绪 → WebView 直载。
 * 单服务单端口，网页与后端同源，无跨域；localStorage 持久化（收藏/队列/主题）。
 */
public class MainActivity extends Activity {

    private static final int PORT = 28976;
    private static final String URL = "http://127.0.0.1:" + PORT;

    private Process backend;
    private WebView web;

    @Override
    protected void onCreate(Bundle b) {
        super.onCreate(b);
        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true); // localStorage 持久化（收藏/队列/主题）
        s.setMediaPlaybackRequiresUserGesture(false); // 音乐自动播放
        s.setUserAgentString(s.getUserAgentString() + " HomeCastApp/1.0");
        web.setWebViewClient(new WebViewClient());
        setContentView(web);

        startBackendIfNeeded(); // 后台线程：探测→启动→轮询→load（主线程禁网络，会崩）
    }

    /**
     * 后台线程统一流程：优先探测 28976 已就绪（旧实例/后台重建/重复打开）直接复用，
     * 避免重复起进程；未就绪才 copy+启动。轮询就绪后 load。
     */
    private void startBackendIfNeeded() {
        new Thread(() -> {
            try {
                if (!backendReady()) {
                    startBackend();
                }
                // 轮询就绪后加载（服务端冷启 <1s，保险起见最多等 8s）
                long deadline = System.currentTimeMillis() + 8000;
                while (System.currentTimeMillis() < deadline) {
                    if (backendReady()) {
                        runOnUiThread(() -> web.loadUrl(URL));
                        return;
                    }
                    try { Thread.sleep(400); } catch (InterruptedException ignored) {}
                }
                runOnUiThread(() -> Toast.makeText(this, "服务端启动超时", Toast.LENGTH_LONG).show());
            } catch (IOException e) {
                String msg = e.getMessage();
                runOnUiThread(() -> Toast.makeText(this, "启动失败: " + msg, Toast.LENGTH_LONG).show());
            }
        }).start();
    }

    /**
     * 从 assets/<abi>/hc-server 拷出服务端二进制（每次覆盖，升级即生效）并拉起。
     * 双 ABI 支持：按设备 SUPPORTED_ABIS 匹配 assets 子目录（arm64-v8a 真机 / x86_64 waydroid、模拟器）
     */
    private void startBackend() throws IOException {
        String chosen = pickAbi();
        File dir = new File(getFilesDir(), "hc");
        if (dir.exists() || dir.mkdirs()) {
            File tmp = new File(dir, "hc-server.tmp");
            File bin = new File(dir, "hc-server");
            InputStream in = getAssets().open(chosen + "/hc-server");
            OutputStream out = new FileOutputStream(tmp);
            byte[] buf = new byte[8192];
            int n;
            while ((n = in.read(buf)) > 0) out.write(buf, 0, n);
            out.close();
            in.close();
            // rename 替换：可执行文件被旧进程映射时直写会被 ETXTBSY 拒，rename 允许（旧 inode 归运行中的进程，新文件就位）
            try {
                java.nio.file.Files.move(tmp.toPath(), bin.toPath(),
                        java.nio.file.StandardCopyOption.REPLACE_EXISTING);
            } catch (IOException e2) {
                if (!tmp.renameTo(bin)) {
                    tmp.delete();
                    throw e2;
                }
            }
            bin.setExecutable(true, true);

            ProcessBuilder pb = new ProcessBuilder(bin.getAbsolutePath());
            pb.environment().put("HC_PORT", Integer.toString(PORT));
            // 安卓 app 进程无 HOME：指向可写私有目录，服务端默认数据目录
            // ~/.config/homecast 才能落盘（SQLite 收藏/队列/设置）
            pb.environment().put("HOME", getFilesDir().getAbsolutePath());
            pb.redirectErrorStream(true);
            pb.redirectOutput(new File(dir, "hc.log"));
            backend = pb.start();
        }
    }

    /** 找当前设备可用的 ABI 服务端：SUPPORTED_ABIS 优先，回退到已内置的目录 */
    private String pickAbi() {
        String[] fallbacks = {"arm64-v8a", "x86_64"};
        for (String abi : android.os.Build.SUPPORTED_ABIS) {
            try {
                getAssets().open(abi + "/hc-server").close();
                return abi;
            } catch (IOException ignored) {}
        }
        for (String abi : fallbacks) {
            try {
                getAssets().open(abi + "/hc-server").close();
                return abi;
            } catch (IOException ignored) {}
        }
        throw new RuntimeException("assets 缺少 hc-server（arm64-v8a / x86_64）");
    }

    private boolean backendReady() {
        try {
            HttpURLConnection c = (HttpURLConnection) new URL(URL).openConnection();
            c.setConnectTimeout(1000);
            c.setReadTimeout(1000);
            int code = c.getResponseCode();
            c.disconnect();
            return code == 200;
        } catch (IOException e) {
            return false;
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        if (backend != null) backend.destroy(); // 关停内嵌服务端
    }
}
