// 音箱推送进度（本地模拟，xiaomusic/MiAir 方法论）：
// 前端 1 秒轮询自己的后端 /api/v1/speaker/progress/{did}（服务端本地时钟算位置），
// 不打小米云。轮询期间设备列表可能被 htmx 重绘 → 每 tick 重新遍历 DOM 动态绑定。
(function () {
  'use strict';
  var fmt = function (s) {
    s = Math.max(0, Math.floor(s));
    var m = Math.floor(s / 60), sec = s % 60;
    return (m < 10 ? '0' + m : '' + m) + ':' + (sec < 10 ? '0' + sec : '' + sec);
  };

  // 单流 SSE（零轮询）：一个 /speaker/events 连接收所有音响本地模拟进度（不打小米云）。
  // 服务端 1s 定时推 {did: {state,position,duration,title}}；htmx 重绘设备列表时
  // 每帧重新遍历 DOM 动态绑定（progress 事件驱动渲染，不再 setInterval 轮询）。
  var lastData = {}; // did -> 最近一次 SSE 进度（htmx 重绘后即时恢复，不闪空）
  function applyAll(all) {
    if (!all) return;
    var boxes = document.querySelectorAll('.sp-prog[data-did]');
    if (!boxes.length) return;
    boxes.forEach(function (b) {
      var did = b.getAttribute('data-did');
      var d = all[did];
      if (!d) { render(b, { state: 'idle' }); return; }
      lastData[did] = d;
      render(b, d);
    });
  }
  try {
    var es = new EventSource('/api/v1/speaker/events');
    es.addEventListener('progress', function (ev) {
      try { applyAll(JSON.parse(ev.data)); } catch (e) {}
    });
    es.onerror = function () {}; // 断线自动重连（EventSource 内建）
  } catch (e) {}
  // htmx 重绘设备列表后（#speaker-section innerHTML swap）：用 lastData 即时恢复，
  // 不闪空（SSE 下个 1s tick 也会来，但重绘瞬间先恢复）
  document.addEventListener('htmx:afterSwap', function (ev) {
    var t = ev.target && ev.target.id;
    if (t === 'speaker-section' || (ev.detail && ev.detail.elt && ev.detail.elt.closest && ev.detail.elt.closest('#speaker-section'))) {
      if (Object.keys(lastData).length) applyAll(lastData);
    }
  });

  function render(box, d) {
    var state = d.state || 'idle';
    if (state === 'idle' || !d.bvid) {
      box.classList.add('hidden');
      return;
    }
    box.classList.remove('hidden');
    var fill = box.querySelector('.sp-prog-fill');
    var time = box.querySelector('.sp-prog-time');
    var st = box.querySelector('.sp-prog-state');
    if (!fill || !time || !st) return;
    var dur = d.duration || 0;
    var pos = d.position || 0;
    var pct = dur > 0 ? Math.min(100, Math.round((pos / dur) * 100)) : 0;
    fill.style.width = pct + '%';
    time.textContent = fmt(pos) + (dur > 0 ? ' / ' + fmt(dur) : '');
    if (state === 'paused') st.textContent = '已暂停 · 自动续播待命中';
    else if (d.next_bvid) st.textContent = '本地进度 · 播完自动连播';
    else st.textContent = '本地进度';
  }

  // 轮询已废（单流 SSE 驱动渲染）
})();