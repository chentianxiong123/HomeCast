// 播放器孤岛（阶段2 重构）：不再是状态持有者，而是「web 播放引擎 + 渲染层」。
// 业务状态唯一住后端（/api/v1/state/stream SSE → Alpine.store('playback')，state.js 桥接）。
// 本文件只做：
//   1. 命令转发（用户点击 → POST /api/v1/state/command）
//   2. HTMLAudio 物理播放执行（audio.play/pause/seek 由状态流驱动）
//   3. 高频 UI：timeupdate 上报进度、拖动进度条即时反馈、音量条（本地临时态）
//   4. 渲染：从 Alpine.store('playback') 派生面板 DOM
(function () {
  const audio = document.getElementById('audio');
  if (!audio) return;
  const $ = (id) => document.getElementById(id);
  const el = {
    dock: $('player-dock'), toggle: $('d-toggle'), prev: $('d-prev'), next: $('d-next'),
    cover: $('d-cover'), title: $('d-title'), artist: $('d-artist'),
    cur: $('d-cur'), dur: $('d-dur'), prog: $('d-progress'), vol: $('d-vol'), mode: $('d-mode'), muteBtn: $('d-mute'),
    qualityBtn: $('d-quality'), eq: $('d-eq'), fav: $('d-fav'),
  };
  if (!el.dock || !el.toggle) return;

  // ---- 本地临时 UI 态（Alpine 不管这些高频/设备偏好；业务状态不走这） ----
  let quality = parseInt(localStorage.getItem('hc:quality') || '192', 10); // 64/128/192
  if (el.qualityBtn) el.qualityBtn.textContent = quality + 'k';
  let volMuted = false; // 静音（WebKit audio.muted 同帧读取旧值，用 JS 变量）
  let lastBvid = '';    // 上次加载的 audio.src（避免重复设）
  let seeking = false;  // 拖动进度条中（暂停上报，松手再上报）

  const fmt = (s) => {
    if (!isFinite(s) || !s) return '0:00';
    const m = Math.floor(s / 60), x = Math.floor(s % 60);
    return m + ':' + String(x).padStart(2, '0');
  };

  // ---- 播放状态（Alpine.store('playback') 由 state.js 维护）----
  function pb() { return window.hcState ? window.hcState.get() : { playing: false, bvid: '', title: '', artist: '', cover: '', position: 0, duration: 0, queue: [], qidx: -1, play_mode: 'order', source: 'web' }; }

  // ---- 渲染：从 playback 状态派生面板 DOM ----
  function render() {
    const p = pb();
    const hasSong = !!(p.bvid);
    el.dock.classList.toggle('hidden', !hasSong);
    const playing = p.playing;
    el.toggle.innerHTML = playing
      ? '<svg class="w-7 h-7" viewBox="0 0 24 24" fill="currentColor"><path d="M6 5h4v14H6zM14 5h4v14h-4z"/></svg>'
      : '<svg class="w-7 h-7 ml-0.5" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>';
    if (el.eq) el.eq.style.display = playing ? 'flex' : 'none';
    el.prev.disabled = el.next.disabled = !hasSong || p.queue.length < 2;
    el.prev.classList.toggle('opacity-40', el.prev.disabled);
    el.next.classList.toggle('opacity-40', el.next.disabled);
    // 歌曲信息（只在变化时写 DOM，防高频重排）
    if (hasSong) {
      if (el.title.textContent !== p.title) el.title.textContent = p.title || '';
      if (el.artist.textContent !== p.artist) el.artist.textContent = p.artist || '未知作者';
      if (el.cover.getAttribute('src') !== p.cover) { el.cover.src = p.cover || ''; el.cover.alt = p.title || ''; }
      if (el.qualityBtn) el.qualityBtn.textContent = quality + 'k';
      renderMode(p.play_mode);
      // 收藏按钮同步：data-* 属性更新 + 状态刷新（统一 hcFav）
      const favBtn = el.fav;
      if (favBtn) {
        if (favBtn.dataset.bvid !== p.bvid) {
          favBtn.dataset.bvid = p.bvid;
          favBtn.dataset.title = p.title || '';
          favBtn.dataset.artist = p.artist || '';
          favBtn.dataset.cover = p.cover || '';
          favBtn.dataset.duration = String(p.duration || 0);
          if (window.hcFav) window.hcFav.syncBtn(favBtn);
        }
      }
    }
    // 进度显示（音箱模式进度由 SSE 推，web 由 timeupdate 推）
    if (!seeking && p.duration > 0) {
      el.cur.textContent = fmt(p.position);
      el.dur.textContent = fmt(p.duration);
      el.prog.value = Math.round((p.position / p.duration) * 1000);
    }
    // 音箱模式：进度条可拖（seek 走后端真 seek，不再置灰）
    if (el.prog) el.prog.classList.toggle('opacity-40', false);
  }

  // ---- 播放模式图标 ----
  const MODE_ICONS = {
    order: '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6h16M4 12h16M4 18h16"/></svg>',
    loop: '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14M7 22l-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/></svg>',
    single: '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 2 21 6l-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="M7 22l-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/></svg><text x="9" y="17" font-size="7" fill="currentColor" stroke="none">1</text>',
    random: '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 3h5v5"/><path d="M4 20 21 3"/><path d="M21 16v5h-5"/><path d="m15 15 6 6"/><path d="M4 4l5 5"/></svg>',
  };
  function renderMode(m) {
    if (!el.mode) return;
    const mode = m || 'order';
    el.mode.innerHTML = MODE_ICONS[mode] || MODE_ICONS.order;
    el.mode.title = { order: '顺序播放', loop: '列表循环', single: '单曲循环', random: '随机播放' }[mode] || '';
    el.mode.classList.toggle('text-pink-400', mode !== 'order');
  }

  // ---- 状态流驱动 HTMLAudio（web 播放引擎）：bvid 变→载源；playing 变→播放/暂停 ----
  function driveAudio() {
    const p = pb();
    if (p.source !== 'web' || !p.bvid) return; // 音箱模式：web 音频不动
    if (p.bvid !== lastBvid) {
      lastBvid = p.bvid;
      const streamURL = '/api/v1/music/stream/' + p.bvid + '?quality=' + quality;
      audio.src = streamURL;
      if (p.playing) audio.play().catch(() => {});
      else audio.load();
    } else if (p.playing && audio.paused) {
      audio.play().catch(() => {});
    } else if (!p.playing && !audio.paused) {
      audio.pause();
    }
  }

  // ---- 命令转发（用户点击 → 后端） ----
  function toggle() {
    const p = pb();
    if (!p.bvid) return;
    if (p.source === 'speaker') {
      // 音箱模式：暂停/恢复走音箱控制
      window.hcState.cmd(p.playing ? 'pause' : 'resume');
      return;
    }
    window.hcState.cmd(p.playing ? 'pause' : 'resume');
  }
  function seek(pct) {
    if (!pb().bvid) return;
    const sec = (pct / 100) * (pb().duration || audio.duration || 0);
    if (pb().source === 'speaker') {
      // 音箱模式：seek 走后端（改内存 offset + 重下发 URL 触发音箱重拉）
      seeking = true;
      window.hcState.cmd('seek', { sec });
      setTimeout(() => { seeking = false; }, 600);
      render();
      return;
    }
    seeking = true;
    audio.currentTime = sec; // 本地即时（高频 UI），随后上报后端
    window.hcState.progress(sec, pb().duration, !audio.paused);
    render();
  }
  function step(dir) {
    window.hcState.cmd(dir === 'prev' ? 'prev' : 'next');
  }
  function cycleMode() {
    const order = ['order', 'loop', 'single', 'random'];
    const cur = order.indexOf(pb().play_mode) || 0;
    const next = order[(cur + 1) % order.length];
    window.hcState.cmd('mode', { mode: next });
    try { localStorage.setItem('hc:mode', next); } catch (e) {}
    hcBus.emit('toast', { msg: { order: '顺序播放', loop: '列表循环', single: '单曲循环', random: '随机播放' }[next] });
  }
  function setQuality(q) {
    quality = q;
    try { localStorage.setItem('hc:quality', String(q)); } catch (e) {}
    if (el.qualityBtn) el.qualityBtn.textContent = q + 'k';
    const p = pb();
    if (p.bvid && p.source === 'web') {
      // 重载音频源（同曲新音质）；状态不变，仅音频源变
      lastBvid = '';
      audio.src = '/api/v1/music/stream/' + p.bvid + '?quality=' + quality;
      if (p.playing) audio.play().catch(() => {});
    }
    hcBus.emit('toast', { msg: '音质 ' + q + 'k' });
  }
  function setVol(v) {
    try { localStorage.setItem('hc:vol', String(v)); } catch (e) {}
    if (pb().source === 'speaker') {
      // 音箱让位模式：音量走音箱 SetVolume API（不打本地 audio）
      var did = pb().spk_did;
      if (did) {
        fetch('/api/v1/speaker/volume', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ did: did, volume: Number(v) })
        }).catch(function () {});
      }
      volMuted = Number(v) === 0;
      renderMute();
      return;
    }
    audio.volume = v / 100;
    volMuted = v === 0;
    audio.muted = volMuted;
    renderMute();
  }
  function mute() {
    if (pb().source === 'speaker') {
      // 音箱让位模式：静音/取消静音 → 音箱音量 0 / 恢复上次音量
      var did = pb().spk_did;
      if (!did) return;
      var target = volMuted ? (parseInt(localStorage.getItem('hc:vol') || '80', 10) || 80) : 0;
      if (volMuted && el.vol) el.vol.value = target;
      fetch('/api/v1/speaker/volume', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ did: did, volume: target })
      }).catch(function () {});
      volMuted = !volMuted;
      renderMute();
      return;
    }
    if (volMuted || audio.volume === 0) {
      volMuted = false;
      if (audio.volume === 0) {
        const def = parseInt(localStorage.getItem('hc:vol') || '80', 10) || 80;
        audio.volume = def / 100;
        if (el.vol) el.vol.value = def;
        try { localStorage.setItem('hc:vol', String(def)); } catch (e) {}
      }
    } else {
      volMuted = true;
    }
    audio.muted = volMuted;
    renderMute();
  }
  function renderMute() {
    if (!el.muteBtn) return;
    const muted = volMuted;
    el.muteBtn.innerHTML = muted
      ? '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 5 6 9H2v6h4l5 4V5z"/><path d="M23 9l-6 6M17 9l6 6"/></svg>'
      : '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 5 6 9H2v6h4l5 4V5z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/><path d="M19 5a9 9 0 0 1 0 14"/></svg>';
    if (el.vol) el.vol.classList.toggle('opacity-40', muted);
  }

  // ---- MediaSession ----
  function updateMediaMeta() {
    if (!('mediaSession' in navigator)) return;
    const p = pb();
    if (p.bvid) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: p.title || '', artist: p.artist || '',
        artwork: p.cover ? [{ src: p.cover, sizes: '512x512' }] : [],
      });
    }
    navigator.mediaSession.playbackState = p.playing ? 'playing' : 'paused';
    try {
      navigator.mediaSession.setActionHandler('play', () => window.hcState.cmd('resume'));
      navigator.mediaSession.setActionHandler('pause', () => window.hcState.cmd('pause'));
      navigator.mediaSession.setActionHandler('previoustrack', () => window.hcState.cmd('prev'));
      navigator.mediaSession.setActionHandler('nexttrack', () => window.hcState.cmd('next'));
    } catch (e) {}
  }

  // ---- audio 事件：播放物理结束 → 后端 next（状态流驱动）----
  audio.addEventListener('ended', () => {
    const p = pb();
    if (p.play_mode === 'single' && p.bvid) { audio.currentTime = 0; audio.play().catch(() => {}); return; }
    window.hcState.cmd('next'); // order/loop/random 走后端
  });
  audio.addEventListener('loadedmetadata', () => {
    const p = pb();
    if (p.duration !== audio.duration) window.hcState.progress(p.position, audio.duration, !audio.paused);
  });
  audio.addEventListener('timeupdate', () => {
    const p = pb();
    if (!p.bvid || p.source !== 'web') return;
    // 高频 UI：本地进度条即时（不等待 SSE 往返）
    if (!seeking) {
      el.cur.textContent = fmt(audio.currentTime);
      const d = audio.duration || 1;
      el.prog.value = Math.round((audio.currentTime / d) * 1000);
      el.dur.textContent = fmt(audio.duration || 0);
    }
    // 上报后端（低频 250ms）
    const now = Date.now();
    if (now - lastReport > 250) {
      lastReport = now;
      window.hcState.progress(audio.currentTime, audio.duration || 0, !audio.paused);
      reportWidget(); // widget 250ms 上报（~4Hz，桌面/安卓歌词挂件跟得上 web）
    }
  });
  let lastReport = 0;

  // ---- widget 上报（桌面歌词挂件：后端 widgetState 读，SSE lyric-state 推行） ----
  function reportWidget() {
    const p = pb();
    if (!p.bvid) return;
    fetch('/api/v1/widget/state', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        bvid: p.bvid, title: p.title, artist: p.artist || '',
        current_time: audio.currentTime, duration: audio.duration || 0,
        playing: !audio.paused,
      }),
    }).catch(() => {});
  }

  // ---- widget 下行控制（SSE 推送，非轮询）：挂件 ⏸/◀/▶ 按钮 → 前端即时消费 ----
  try {
    const es = new EventSource('/api/v1/widget/events');
    es.addEventListener('toggle', () => toggle());
    es.addEventListener('prev', () => step('prev'));
    es.addEventListener('next', () => step('next'));
    // 歌词当前行统一走后端（SSE lyric-state 带 line）→ hcBus 广播给歌词页/字幕条
    es.addEventListener('lyric-state', (ev) => {
      try { const d = JSON.parse(ev.data); if (d && typeof d.line === 'string') hcBus.emit('lyric-line', d.line); } catch (e) {}
    });
    es.onerror = () => {};
  } catch (e) {}

  // ---- 控制事件 ----
  el.prog.addEventListener('change', () => {
    seeking = false;
    seek((el.prog.value / 1000) * 100);
  });
  el.prog.addEventListener('input', () => {
    // 拖动中本地即时反馈（高频 UI），松手 change 才 seek+上报
    const d = pb().duration || audio.duration || 1;
    el.cur.textContent = fmt((el.prog.value / 1000) * d);
  });
  el.vol.addEventListener('input', () => setVol(el.vol.value));
  el.toggle.addEventListener('click', toggle);
  el.prev.addEventListener('click', () => step('prev'));
  el.next.addEventListener('click', () => step('next'));
  el.cover.addEventListener('click', () => {
    const p = pb();
    if (!p.bvid) return;
    const lyp = document.getElementById('lyric-panel');
    if (lyp && !lyp.classList.contains('ly-hidden')) hcBus.emit('lyric-close');
    else hcBus.emit('lyric-open', { bvid: p.bvid, title: p.title, artist: p.artist, cover: p.cover });
  });
  if (el.mode) el.mode.addEventListener('click', cycleMode);
  if (el.qualityBtn) el.qualityBtn.addEventListener('click', () => {
    setQuality(quality === 64 ? 128 : quality === 128 ? 192 : 64);
  });
  if (el.muteBtn) el.muteBtn.addEventListener('click', mute);

  // ---- 外部点播桥接（其他页面 hcBus 'play'）→ 发后端命令 ----
  hcBus.on('play', (d) => {
    if (!d || !d.bvid) return;
    window.hcState.cmd('play', { bvid: d.bvid, title: d.title || '', artist: d.artist || '', cover: d.cover || '' });
  });
  hcBus.on('toggle', toggle);
  hcBus.on('seek', (d) => { if (d && typeof d.pct === 'number') seek(d.pct); });
  hcBus.on('prev', () => step('prev'));
  hcBus.on('next', () => step('next'));
  hcBus.on('play-next', (d) => { if (d && d.bvid) window.hcState.cmd('play', { bvid: d.bvid, title: d.title || '', artist: d.artist || '', cover: d.cover || '' }); });
  hcBus.on('mode-cycle', cycleMode);
  hcBus.on('mode', (d) => { if (d && d.m) window.hcState.cmd('mode', { mode: d.m }); });
  hcBus.on('mute', mute);
  hcBus.on('queue-clear', () => window.hcState.cmd('clear'));
  hcBus.on('queue-remove', (d) => { if (d && d.bvid) window.hcState.cmd('remove', { bvid: d.bvid }); });
  hcBus.on('quality', (d) => { if (d && d.q) setQuality(d.q); });
  hcBus.on('play-all-list', (d) => {
    const items = ((d && d.items) || []).filter((x) => x && x.bvid);
    if (!items.length) return;
    window.hcState.cmd('queue', { queue: items.map((x) => ({ bvid: x.bvid, title: x.title, artist: x.artist, cover: x.cover })), qidx: 0 });
  });
  hcBus.on('play-fav-all', async () => {
    try {
      const j = await (await fetch('/api/v1/fav/list')).json();
      const items = j.data || [];
      if (!items.length) { hcBus.emit('toast', { msg: '收藏夹是空的' }); return; }
      window.hcState.cmd('queue', { queue: items.map((x) => ({ bvid: x.bvid, title: x.title, artist: x.artist, cover: x.cover })), qidx: 0 });
    } catch (e) { hcBus.emit('toast', { msg: '加载失败' }); }
  });

  // ---- 投送桥接（cast/speaker）：player 持当前歌（Alpine store）调后端 ----
  hcBus.on('cast-play', (d) => {
    const p = pb();
    if (!p.bvid) { hcBus.emit('cast-result', { ok: false, udn: d.udn, msg: '请先播放一首歌' }); return; }
    const body = new URLSearchParams({ bvid: p.bvid, udn: d.udn, title: p.title });
    fetch('/hx/cast/play', { method: 'POST', body })
      .then((r) => r.text())
      .then((t) => hcBus.emit('cast-result', { ok: t.includes('成功'), udn: d.udn, msg: '投送失败' }))
      .catch(() => hcBus.emit('cast-result', { ok: false, udn: d.udn, msg: '网络错误' }));
  });
  hcBus.on('speaker-play', (d) => {
    const p = pb();
    if (!p.bvid) { hcBus.emit('speaker-result', { ok: false, did: d.did, msg: '请先播放一首歌' }); return; }
    const body = new URLSearchParams({
      bvid: p.bvid, did: d.did, title: p.title || '',
      duration: String(p.duration || 0), next_bvid: nextBVID() || '',
    });
    fetch('/hx/speaker/play', { method: 'POST', body })
      .then((r) => r.text())
      .then((t) => {
        const ok = t.includes('音箱播放');
        if (ok) window.hcState.cmd('switch_speaker', { did: d.did }); // 切音箱模式（后端统一）
        hcBus.emit('speaker-result', { ok, did: d.did, msg: ok ? '音箱播放' : '投送失败' });
      })
      .catch(() => hcBus.emit('speaker-result', { ok: false, did: d.did, msg: '网络错误' }));
  });
  function nextBVID() {
    const p = pb();
    if (!p.bvid || !p.queue || p.queue.length < 2) return '';
    const i = p.queue.findIndex((q) => q.bvid === p.bvid);
    if (i < 0) return p.queue[0].bvid;
    return p.queue[(i + 1) % p.queue.length].bvid;
  }

  // ---- 状态流变化 → 驱动音频 + 渲染（state.js 已写 Alpine.store）----
  // 订阅 Alpine store 变化（Alpine 3 用 effect，这里简单轮询 + 事件）
  if (window.Alpine) {
    // Alpine 3: Alpine.effect 在组件外不可用，用 store 监听事件（Alpine 触发 'alpine:store'）
    let last = '';
    setInterval(() => {
      const p = pb();
      const key = p.bvid + '|' + p.playing + '|' + p.source + '|' + p.play_mode + '|' + p.queue.length;
      if (key !== last) {
        last = key;
        driveAudio();
        render();
        updateMediaMeta();
      }
    }, 120);
  }

  // ---- 音量/静音初始化（本地设备偏好）----
  const initVol = parseInt(localStorage.getItem('hc:vol') || '80', 10) || 80;
  audio.volume = initVol / 100;
  volMuted = false;
  audio.muted = false;
  if (el.vol) el.vol.value = initVol;
  renderMute();

  // 初始渲染（无歌隐藏 dock）
  render();
})();
