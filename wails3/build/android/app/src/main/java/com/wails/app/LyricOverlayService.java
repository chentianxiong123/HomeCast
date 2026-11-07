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
    /** ✕ 关闭回调：MainActivity 设置（同步前端 #d-tick/hc:androlyric + stopSelf）。 */
    public static Runnable onCloseRequest;

    private WindowManager windowManager;
    private TextView lyricView;
    private WindowManager.LayoutParams params;
    private volatile boolean running = false;
    private Thread pollThread;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private SharedPreferences prefs;
    private android.view.View rootView;
    private TextView btnPause; // 播放/暂停按钮（applyState 按 playing 切 ⏸/▶️）
    private android.graphics.drawable.Drawable rootBg; // 整体框背景（聚焦出框用）
    private LinearLayout ctrl; // 控制条（⑤ 平时隐藏，一抓出框才露）
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
        lyricView.setTextSize(22);
        lyricView.setTextColor(Color.WHITE);
        lyricView.setShadowLayer(3, 0, 1, Color.argb(200, 0, 0, 0));
        lyricView.setTypeface(Typeface.DEFAULT_BOLD);
        lyricView.setGravity(Gravity.CENTER);
        // 对齐桌面挂件视觉：透明底 + 1px 白描边 + 圆角
        lyricView.setPadding(dp(14), dp(8), dp(14), dp(8));
        // 整体一个框：歌词不再独立描边（root 统一圆角背景 + 描边）

        // 控制条（特殊接口适配：安卓 UI；逻辑仍走后端 /widget/toggle、/lyric/offset）
        ctrl = new LinearLayout(this);
        ctrl.setOrientation(LinearLayout.HORIZONTAL);
        ctrl.setGravity(Gravity.CENTER);
        // ⑤ 平时只歌词（控制条隐藏），一抓出框才露 − ⏸ + ✕
        ctrl.setVisibility(View.GONE);
        btnPause = ctrlBtn("⏸"); // 字段：applyState 按 playing 切 ⏸/▶️
        TextView btnMinus = ctrlBtn("−");
        TextView btnPlus = ctrlBtn("+");
        TextView btnClose = ctrlBtn("✕"); // 关闭悬浮窗（用户：还少 ✕）
        btnPause.setOnClickListener(v -> postJson("/api/v1/widget/toggle", "{}"));
        btnMinus.setOnClickListener(v -> postOffset(-0.5));
        btnPlus.setOnClickListener(v -> postOffset(0.5));
        btnClose.setOnClickListener(v -> {
            // ✕：停悬浮窗（stopSelf）+ 回调 MainActivity 同步前端（hc:androlyric=0 + #d-tick）
            if (onCloseRequest != null) onCloseRequest.run();
            else stopSelf();
        });
        ctrl.addView(btnMinus);
        ctrl.addView(btnPause);
        ctrl.addView(btnPlus);
        ctrl.addView(btnClose);

        // 总布局：歌词行 + 控制条（垂直）
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setGravity(Gravity.CENTER);
        root.addView(lyricView);
        root.addView(ctrl);
        root.setPadding(dp(6), dp(6), dp(6), dp(6));
        // 整体一个框：平时纯透明（不点击不出框），聚焦（触摸/拖动）才出框
        // 桌面挂件同款圆角 + 半透明深底 + 白描边（歌词+控制条在内）
        rootBg = drawRoundedBorder();
        root.setBackground(null); // 默认透明（未聚焦）

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
        b.setTextSize(20);
        b.setTextColor(Color.WHITE);
        b.setGravity(Gravity.CENTER);
        // ① 按钮大一点、触控区足（≥40dp）：padding 14,7
        b.setPadding(dp(14), dp(7), dp(14), dp(7));
        // ⑥ 点击反馈：按下时半透明白底（触感）
        b.setOnTouchListener((View v, MotionEvent ev) -> {
            if (ev.getActionMasked() == MotionEvent.ACTION_DOWN) {
                v.setBackgroundColor(Color.argb(60, 255, 255, 255)); // 按下微亮
            } else if (ev.getActionMasked() == MotionEvent.ACTION_UP
                    || ev.getActionMasked() == MotionEvent.ACTION_CANCEL) {
                v.setBackground(null); // 松开恢复透明
            }
            return false; // 不拦截（让 OnClickListener 收到）
        });
        // 无独立框：与整体框融合（用户：别有自己的框框）
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
        g.setColor(Color.argb(105, 12, 12, 20)); // ③ 深底稍深（白字更清晰）
        g.setCornerRadius(dp(14));
        g.setStroke(dp(1), Color.argb(130, 255, 255, 255)); // ③ 描边稍亮
        return g;
    }

    private void attachDrag(View target) {
        final float[] down = new float[2];
        target.setOnTouchListener((View v, MotionEvent ev) -> {
            switch (ev.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    // 一抓就出框：按下瞬间出框 + 控制条立刻露出（跟手）
                    if (rootBg != null) v.setBackground(rootBg);
                    if (ctrl != null) ctrl.setVisibility(View.VISIBLE);
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
                    // ④ 出框延迟 600ms（跟手）：恢复透明 + 控制条隐藏（平时只歌词）
                    ui.postDelayed(() -> {
                        v.setBackground(null);
                        if (ctrl != null) ctrl.setVisibility(View.GONE);
                    }, 600);
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
        final boolean playing = d.optBoolean("playing", true);
        ui.post(() -> {
            lyricView.setText(finalLine);
            // ⏸/▶️ UI 变化：播放中 ⏸（可暂停），暂停 ▶️（可播放）
            if (btnPause != null) btnPause.setText(playing ? "⏸" : "▶️");
        });
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
