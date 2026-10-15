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

  // 当前播放中的进度条（避免同一设备多个卡片重复请求）
  var lastFetch = {}; // did -> { ts, data }

  function poll() {
    var boxes = document.querySelectorAll('.sp-prog[data-did]');
    if (!boxes.length) return;
    var dids = [];
    boxes.forEach(function (b) { dids.push(b.getAttribute('data-did')); });
    var seen = {};
    boxes.forEach(function (box) {
      var did = box.getAttribute('data-did');
      if (seen[did]) return; // 同 did 只请求一次，其余共享
      seen[did] = true;
      var now = Date.now();
      var cached = lastFetch[did];
      if (cached && now - cached.ts < 800) { render(box, cached.data); return; }
      fetch('/api/v1/speaker/progress/' + encodeURIComponent(did))
        .then(function (r) { return r.json(); })
        .then(function (j) {
          var d = (j && j.data) || {};
          lastFetch[did] = { ts: Date.now(), data: d };
          boxes.forEach(function (b) {
            if (b.getAttribute('data-did') === did) render(b, d);
          });
        })
        .catch(function () {});
    });
  }

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

  setInterval(poll, 1000);
})();