// 音箱推送进度（本地模拟，xiaomusic/MiAir 方法论）：
// 前端 1 秒轮询自己的后端 /api/v1/speaker/progress/{did}（服务端本地时钟算位置），
// 不打小米云。轮询期间设备列表可能被 htmx 重绘 → 每 tick 重新遍历 DOM 动态绑定。
(function () {
  'use strict';
  // 单流 SSE（零轮询）：一个 /speaker/events 连接收所有音响本地模拟进度（不打小米云）。
  // 服务端 1s 定时推 {did: {state,position,duration,title}}；htmx 重绘设备列表时
  // 每帧重新遍历 DOM 动态绑定（progress 事件驱动渲染，不再 setInterval 轮询）。
  var lastData = {}; // did -> 最近一次 SSE 进度（htmx 重绘后即时恢复，不闪空）
  function applyAll(all) {
    if (!all) return;
    // 音箱进度 → Alpine store（dock 进度条/时间走起来）：source=speaker 时
    // 把本地模拟位置写进 playback store，并触发 render（render 读 pb().position）
    try {
      if (window.Alpine) {
        var st = Alpine.store('playback');
        if (st && st.source === 'speaker' && st.spk_did) {
          var d = all[st.spk_did];
          if (d && d.bvid) {
            st.position = d.position || 0;
            if (d.duration > 0) st.duration = d.duration;
            st.playing = d.state === 'playing';
            if (window.hcBus) window.hcBus.emit('state', { state: st.playing ? 'playing' : 'paused' });
          }
        }
      }
    } catch (e) {}
    // 广播给播放器面板（音响让位模式：进度/状态复用 player-dock）
    try { if (window.hcBus) window.hcBus.emit('speaker-progress', all); } catch (e) {}
    var boxes = document.querySelectorAll('.sp-state[data-did]');
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
    var label = box.querySelector('.sp-state-label');
    if (!label) return;
    if (state === 'idle' || !d.bvid) {
      box.classList.add('hidden');
      label.textContent = '';
      return;
    }
    box.classList.remove('hidden');
    if (state === 'paused') label.textContent = '已暂停';
    else if (d.next_bvid) label.textContent = '播放中 · 播完自动连播';
    else label.textContent = '播放中';
  }

  // 轮询已废（单流 SSE 驱动渲染）
})();