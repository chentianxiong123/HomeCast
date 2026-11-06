package com.wails.app;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ServiceInfo;
import android.graphics.Color;
import android.graphics.PixelFormat;
import android.graphics.Typeface;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.util.Log;
import android.view.Gravity;
import android.view.MotionEvent;
import android.view.View;
import android.view.WindowManager;
import android.widget.LinearLayout;
import android.widget.TextView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;

/**
 * 安卓悬浮窗歌词挂件（用户理念：桌面 GTK 挂件的安卓对等物）。
 *
 * 独立 Java Service（wails3 官方安卓无悬浮窗 API，只有单 WebView 窗口）。
 * - TYPE_APPLICATION_OVERLAY 悬浮窗 + TextView 显示当前歌词行
 * - SSE 订阅后端真实 HTTP（http://127.0.0.1:28976，与 WebView 同源）：
 *   · /api/v1/widget/events 拿 event: lyric-state + data 快照（bvid/ct/playing/title/artist）
 *   · /api/v1/music/lyric?bvid= 拿行 [[t,text,dur],...]（仅 bvid 变化时拉取，缓存）
 * - 拖动持久化（SharedPreferences），startForeground 通知保持进程
 *
 * 歌词进度（逐字）由挂件本地按 ct 算行；状态（切歌/播放/标题）走 SSE 即时推送（废轮询）。
 */
public class LyricOverlayService extends android.app.Service {
    private static final String TAG = "LyricOverlay";
    private static final String API = "http://127.0.0.1:28976";
    private static final String CHANNEL_ID = "hc_lyric_overlay";
    private static final int NOTIFICATION_ID = 0x4C79; // "Ly"
    private static final String PREFS = "hc_lyric_overlay";
    private static final long POLL_MS = 500;

    private WindowManager windowManager;
    private TextView lyricView;
    private WindowManager.LayoutParams params;
    private volatile boolean running = false;
    private Thread pollThread;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private SharedPreferences prefs;
    private android.view.View rootView;
    private String currentBvid = "";
    private double currentOffset = 0;


    @Override
    public void onCreate() {
        super.onCreate();
        // 幂等：Service 已建过悬浮窗则不重建（防刷出一大堆窗口；Android 单例但进程重建会重复）
        if (rootView != null) {
            Log.w(TAG, "onCreate: 悬浮窗已存在，跳过重建（幂等）");
            startForegroundCompat();
            running = true;
            return;
        }
        prefs = getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        windowManager = (WindowManager) getSystemService(WINDOW_SERVICE);
        createOverlayView();
        startForegroundCompat();
        running = true;
        pollThread = new Thread(this::sseLoop, "hc-lyric-sse");
        pollThread.start();
        Log.i(TAG, "LyricOverlayService created");
    }

    private void createOverlayView() {
        lyricView = new TextView(this);
        lyricView.setText("HomeCast 歌词");
        lyricView.setTextSize(20);
        lyricView.setTextColor(Color.WHITE);
        lyricView.setShadowLayer(3, 0, 1, Color.argb(200, 0, 0, 0));
        lyricView.setTypeface(Typeface.DEFAULT_BOLD);
        lyricView.setGravity(Gravity.CENTER);
        // 对齐桌面挂件视觉：透明底 + 1px 白描边 + 圆角
        lyricView.setPadding(dp(14), dp(8), dp(14), dp(8));
        lyricView.setBackground(drawRoundedBorder());

        // 控制条（特殊接口适配：安卓 UI；逻辑仍走后端 /widget/toggle、/lyric/offset）
        LinearLayout ctrl = new LinearLayout(this);
        ctrl.setOrientation(LinearLayout.HORIZONTAL);
        ctrl.setGravity(Gravity.CENTER);
        TextView btnPause = ctrlBtn("⏸");
        TextView btnMinus = ctrlBtn("−");
        TextView btnPlus = ctrlBtn("+");
        btnPause.setOnClickListener(v -> postJson("/api/v1/widget/toggle", "{}"));
        btnMinus.setOnClickListener(v -> postOffset(-0.5));
        btnPlus.setOnClickListener(v -> postOffset(0.5));
        ctrl.addView(btnMinus);
        ctrl.addView(btnPause);
        ctrl.addView(btnPlus);

        // 总布局：歌词行 + 控制条（垂直）
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setGravity(Gravity.CENTER);
        root.addView(lyricView);
        root.addView(ctrl);
        root.setPadding(dp(4), dp(4), dp(4), dp(4));

        int type = Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                ? WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY
                : WindowManager.LayoutParams.TYPE_PHONE;
        params = new WindowManager.LayoutParams(
                WindowManager.LayoutParams.WRAP_CONTENT,
                WindowManager.LayoutParams.WRAP_CONTENT,
                type,
                WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE
                        | WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL,
                PixelFormat.TRANSLUCENT);
        params.gravity = Gravity.TOP | Gravity.START;
        // 恢复上次位置
        params.x = prefs.getInt("x", 40);
        params.y = prefs.getInt("y", 200);

        rootView = root;
        // 幂等：先移除已挂的同视图，防重复 addView（IllegalStateException）
        try { if (rootView != null) windowManager.removeView(rootView); } catch (Exception ignored) {}
        windowManager.addView(root, params);
        attachDrag(root);
    }

    // 控制条小按钮（白字半透明，视觉对齐桌面挂件）
    private TextView ctrlBtn(String text) {
        TextView b = new TextView(this);
        b.setText(text);
        b.setTextSize(18);
        b.setTextColor(Color.WHITE);
        b.setGravity(Gravity.CENTER);
        b.setPadding(dp(12), dp(4), dp(12), dp(4));
        b.setBackground(drawRoundedBorder());
        return b;
    }

    // 发后端意图（控制条按钮）；offset 特殊走 /api/v1/lyric/offset（body JSON）
    private void postJson(String path, String body) {
        new Thread(() -> {
            try {
                HttpURLConnection conn = (HttpURLConnection) new URL(API + path).openConnection();
                conn.setRequestMethod("POST");
                conn.setRequestProperty("Content-Type", "application/json");
                conn.setDoOutput(true);
                conn.getOutputStream().write(body.getBytes("UTF-8"));
                conn.getResponseCode();
                conn.disconnect();
            } catch (Exception e) {
                Log.d(TAG, "postJson error: " + e.getClass().getSimpleName());
            }
        }).start();
    }

    // ±0.5s 微调：用当前 bvid + 新 offset 发后端（逻辑住后端 PickLine）
    private void postOffset(double d) {
        String bvid = currentBvid;
        if (bvid == null || bvid.isEmpty()) return;
        double off = currentOffset + d;
        currentOffset = off;
        postJson("/api/v1/lyric/offset", "{\"bvid\":\"" + bvid + "\",\"offset\":" + off + "}");
    }

    private android.graphics.drawable.Drawable drawRoundedBorder() {
        android.graphics.drawable.GradientDrawable g = new android.graphics.drawable.GradientDrawable();
        g.setColor(Color.argb(90, 16, 16, 24)); // 半透明深底（白字可读）
        g.setCornerRadius(dp(14));
        g.setStroke(dp(1), Color.argb(110, 255, 255, 255)); // 1px 白描边
        return g;
    }

    private void attachDrag(View target) {
        final float[] down = new float[2];
        target.setOnTouchListener((View v, MotionEvent ev) -> {
            switch (ev.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    down[0] = ev.getRawX() - params.x;
                    down[1] = ev.getRawY() - params.y;
                    return true;
                case MotionEvent.ACTION_MOVE:
                    params.x = (int) (ev.getRawX() - down[0]);
                    params.y = (int) (ev.getRawY() - down[1]);
                    windowManager.updateViewLayout(rootView, params);
                    return true;
                case MotionEvent.ACTION_UP:
                    prefs.edit().putInt("x", params.x).putInt("y", params.y).apply();
                    return true;
            }
            return false;
        });
    }

    private void startForegroundCompat() {
        NotificationManager nm = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel ch = new NotificationChannel(
                    CHANNEL_ID, "歌词悬浮窗", NotificationManager.IMPORTANCE_LOW);
            nm.createNotificationChannel(ch);
        }
        PendingIntent contentIntent = null;
        Intent launch = getPackageManager().getLaunchIntentForPackage(getPackageName());
        if (launch != null) {
            int flags = Build.VERSION.SDK_INT >= Build.VERSION_CODES.M
                    ? PendingIntent.FLAG_IMMUTABLE : 0;
            contentIntent = PendingIntent.getActivity(this, 0, launch, flags);
        }
        Notification n = new Notification.Builder(this,
                Build.VERSION.SDK_INT >= Build.VERSION_CODES.O ? CHANNEL_ID : "lyric")
                .setSmallIcon(android.R.drawable.ic_media_play)
                .setContentTitle("HomeCast 歌词")
                .setContentText("悬浮歌词已开启")
                .setOngoing(true)
                .setContentIntent(contentIntent)
                .build();
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(NOTIFICATION_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
        } else {
            startForeground(NOTIFICATION_ID, n);
        }
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        running = false;
        if (pollThread != null) pollThread.interrupt();
        try {
            if (rootView != null) windowManager.removeView(rootView);
        } catch (Exception ignored) {
        }
        super.onDestroy();
        Log.i(TAG, "LyricOverlayService destroyed");
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    // ---- SSE 订阅：/api/v1/widget/events（去轮询，用户理念"废轮询要推送"） ----
    // 事件 event: lyric-state + data(JSON 快照) → applyState（切歌拉行 / 同歌更新当前行）
    private void sseLoop() {
        while (running) {
            try {
                HttpURLConnection conn = (HttpURLConnection) new URL(API + "/api/v1/widget/events").openConnection();
                conn.setRequestProperty("Accept", "text/event-stream");
                conn.setConnectTimeout(3000);
                conn.setReadTimeout(0); // 长连接，不超时（SSE 流）
                conn.setDoInput(true);
                BufferedReader br = new BufferedReader(new InputStreamReader(conn.getInputStream(), "UTF-8"));

                // 连接后先拉一次快照，避免等首个事件前空白
                try {
                    JSONObject state = getJson("/api/v1/widget/state");
                    if (state != null && state.optJSONObject("data") != null) {
                        applyState(state.optJSONObject("data"));
                    }
                } catch (Exception ignore) {}

                String event = "";
                String line;
                while (running && (line = br.readLine()) != null) {
                    if (line.startsWith("event:")) {
                        event = line.substring(6).trim();
                    } else if (line.startsWith("data:")) {
                        String data = line.substring(5).trim();
                        if ("lyric-state".equals(event) && !data.isEmpty()) {
                            applyState(new JSONObject(data));
                        }
                        event = "";
                    } else if (line.isEmpty()) {
                        event = "";
                    }
                }
                br.close();
                conn.disconnect();
            } catch (Exception e) {
                Log.d(TAG, "sse error: " + e.getClass().getSimpleName());
            }
            try {
                Thread.sleep(POLL_MS); // 断线重连间隔（非轮询）
            } catch (InterruptedException ie) {
                break;
            }
        }
    }

    // SSE lyric-state 落地：bvid 变 → 拉歌词行；同歌 → 按 ct 更新当前行
    // applyState SSE lyric-state 落地（多平台统一：显示层纯渲染，不本地定位）
    // 逻辑一份住后端（PickLine），这里只用 SSE 算好的 line；无 line 退回标题。
    private void applyState(JSONObject d) {
        String line = d.optString("line", "");
        String title = d.optString("title", "");
        String artist = d.optString("artist", "");
        currentBvid = d.optString("bvid", "");
        currentOffset = d.optDouble("offset", currentOffset);
        if (line == null || line.isEmpty()) {
            line = title.isEmpty() ? "HomeCast 歌词" : title + " - " + artist;
        }
        final String finalLine = line;
        ui.post(() -> lyricView.setText(finalLine));
    }

    // ---- 后端 HTTP（cleartext 127.0.0.1 已放行 network_security_config） ----
    private JSONObject getJson(String path) {
        HttpURLConnection conn = null;
        try {
            conn = (HttpURLConnection) new URL(API + path).openConnection();
            conn.setConnectTimeout(1000);
            conn.setReadTimeout(1000);
            conn.setRequestMethod("GET");
            int code = conn.getResponseCode();
            if (code != 200) return null;
            try (BufferedReader br = new BufferedReader(
                    new InputStreamReader(conn.getInputStream(), StandardCharsets.UTF_8))) {
                StringBuilder sb = new StringBuilder();
                String line;
                while ((line = br.readLine()) != null) sb.append(line);
                return new JSONObject(sb.toString());
            }
        } catch (Exception e) {
            return null;
        } finally {
            if (conn != null) conn.disconnect();
        }
    }

    private int dp(int v) {
        return Math.round(getResources().getDisplayMetrics().density * v);
    }
}
