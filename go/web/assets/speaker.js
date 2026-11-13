// 音箱设备列表状态（设备卡片 播放中/已暂停 标签）：
// 单流 SSE /speaker/events 收所有音响状态（不打小米云），htmx 重绘时 lastData 防闪恢复。
// 进度统一由 state.js 从 /state/stream 拿（store 单写者，勿双写——双写=状态脏/回跳根因）。
(function () {
  var lastData = {}; // did -> 最近一次 SSE 进度（htmx 重绘后即时恢复，不闪空）
  function applyAll(all) {
    if (!all) return;
    // 注：不写 Alpine store（进度由 state.js 单写，双写=状态脏/回跳的根因）。
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