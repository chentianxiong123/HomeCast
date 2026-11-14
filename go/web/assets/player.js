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
    dock: $('player-dock'), toggle: $('d-toggle'), prev: $('d-prev'), next: $('d-next'), stop: $('d-stop'),
    cover: $('d-cover'), title: $('d-title'), artist: $('d-artist'),
    cur: $('d-cur'), dur: $('d-dur'), prog: $('d-progress'), vol: $('d-vol'), mode: $('d-mode'), muteBtn: $('d-mute'),
    qualityBtn: $('d-quality'), eq: $('d-eq'), fav: $('d-fav'),
  };
  if (!el.dock || !el.toggle) return;

  // ---- 本地临时 UI 态（Alpine 不管这些高频/设备偏好；业务状态不走这） ----
  let quality = parseInt(localStorage.getItem('hc:quality') || '192', 10); // 64/128/192
  if (el.qualityBtn) el.qualityBtn.textContent = quality + 'k';
  let volMuted = false; // 静音（WebKit audio.muted 同帧读取旧值，用 JS 变量）
  let lastSource = null; // 上次音源（web/speaker）：切回 web 时恢复 web 本地音量态用
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
    // 停止按钮：仅音箱模式显示（点了=音箱停+回手机，复用核心 stop_speaker，与设备页同一命令）
    if (el.stop) el.stop.classList.toggle('hidden', p.source !== 'speaker');
    // 歌曲信息（只在变化时写 DOM，防高频重排）
    if (hasSong) {
      if (el.title.textContent !== p.title) el.title.textContent = p.title || '';
      if (el.artist.textContent !== p.artist) el.artist.textContent = p.artist || '未知作者';
      if (el.cover.getAttribute('src') !== p.cover) { el.cover.src = p.cover || ''; el.cover.alt = p.title || ''; }
      if (el.qualityBtn) el.qualityBtn.textContent = quality + 'k';
      // 设备切换唯一入口在设备页（投送/停止），dock 不再显示“切回网页”徽标
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
    // 总时长用 audio.duration 优先——store duration 切歌瞬间可能残留旧值（queue 恢复的歌 duration=0），
    // 用 store 会卡旧歌时长（如都是 3:04）。audio.duration 是真实当前音频时长，最准。
    const isWeb = pb().source === 'web';
    const dispDur = (isWeb && audio.duration) ? audio.duration : pb().duration;
    if (!seeking && dispDur > 0) {
      const curT = isWeb ? audio.currentTime : p.position;
      el.cur.textContent = fmt(curT);
      el.dur.textContent = fmt(dispDur);
      el.prog.value = Math.round((curT / dispDur) * 1000);
    }
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
    // 独占：同一时刻只有一个设备出声。source≠web（音箱模式）时本机必须闭嘴——
    // 不能只 return 不管 audio，否则音箱在响、手机同时还在放 = 双响
    if (p.source !== 'web' || !p.bvid) {
      if (!audio.paused) audio.pause();
      prevSource = p.source;
      return;
    }
    // 从音箱模式切回 web：强制重载当前歌（lastBvid 在音箱模式下可能已被音箱桥更新到同 bvid，
    // 或 audio.src 还停在旧歌——必须重置 lastBvid 让下面载源分支无条件执行）
    if (prevSource !== 'web') lastBvid = '';
    prevSource = p.source;
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
  let prevSource = 'web'; // 上一个 source（切回 web 时强制重载）
  let lastBvid = '';

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
    if (pb().source === 'speaker') return; // 音箱 seek 未接，忽略
    // 用 audio.duration（真实当前音频时长）优先——store duration 可能滞后于切歌，
    // 用它算 sec 会跳到旧歌末尾→立即 ended→误切歌（点进度条切歌 bug 根因）
    const d = audio.duration || pb().duration || 0;
    if (d <= 0) return;
    const sec = (pct / 100) * d;
    if (sec >= d) return; // 防越界（跳到末尾会触发 ended 误切歌）
    seeking = true;
    audio.currentTime = sec; // 本地即时（高频 UI），随后上报后端
    window.hcState.progress(sec, d, !audio.paused);
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
    v = Number(v) || 0; // range input value 是字符串，后端 Volume int 解码数字必须转
    volMuted = v === 0;
    try { localStorage.setItem('hc:vol', String(v)); } catch (e) {}
    if (pb().source === 'speaker') {
      // 音箱模式：音量走后端 SSOT（set_volume command → Output.SetVolume 真调音箱 + 广播）。
      // 后端 bcastSnapshot → SSE state → applyState 回填滑块（spk-volume 事件），不在此处直接调 API。
      window.hcState && window.hcState.cmd('set_volume', { volume: v });
    } else {
      // web 模式：本地 audio 直控
      audio.volume = v / 100;
      audio.muted = volMuted;
    }
    renderMute();
  }
  function mute() {
    if (pb().source === 'speaker') {
      const did = pb().spk_did;
      const wasMuted = volMuted; // 音箱静音态只看 volMuted（audio.volume 在音箱模式无意义）
      if (wasMuted) {
        // 取消静音 → 恢复上次音量（def=0 兜底 80，否则取消静音没声）
        volMuted = false;
        let def = parseInt(localStorage.getItem('hc:vol') || '80', 10) || 80;
        if (def <= 0) def = 80;
        if (did) window.hcState && window.hcState.cmd('set_volume', { volume: def });
        if (el.vol) el.vol.value = def; // ★补：滑块 UI 同步（旧实现漏，取消静音滑块停 0）
        try { localStorage.setItem('hc:vol', String(def)); } catch (e) {}
      } else {
        // 静音 → 音量 0（走后端 SSOT，换歌/重启后 Play 重应用 wantVolume=0 保持静音）
        volMuted = true;
        if (did) window.hcState && window.hcState.cmd('set_volume', { volume: 0 });
      }
    } else {
      if (volMuted || audio.volume === 0) {
        volMuted = false;
        if (audio.volume === 0) {
          let def = parseInt(localStorage.getItem('hc:vol') || '80', 10) || 80;
          if (def <= 0) def = 80;
          audio.volume = def / 100;
          if (el.vol) el.vol.value = def;
          try { localStorage.setItem('hc:vol', String(def)); } catch (e) {}
        }
      } else {
        volMuted = true;
      }
      audio.muted = volMuted;
    }
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
    // 上报后端（低频 250ms；widget 已由后端 progress handler 顺手写，不再单独 POST）
    const now = Date.now();
    if (now - lastReport > 250) {
      lastReport = now;
      window.hcState.progress(audio.currentTime, audio.duration || 0, !audio.paused);
    }
  });
  // seek 完成后重置 seeking（否则 render/timeupdate 的 !seeking 检查永久跳过，进度/时长远停）
  audio.addEventListener('seeked', () => {
    if (seeking) {
      seeking = false;
      render();
    }
  });
  let lastReport = 0;

  // ---- widget 下行控制（SSE 推送，非轮询）：挂件 ⏸/◀/▶ 按钮 → 前端即时消费 ----
  try {
    const es = new EventSource('/api/v1/widget/events');
    es.addEventListener('toggle', () => toggle());
    es.addEventListener('prev', () => step('prev'));
    es.addEventListener('next', () => step('next'));
    // 歌词当前行统一走后端（SSE lyric-state 带 line + current_time/offset）→ hcBus 广播
    // 给歌词页/字幕条。前端按 ct 定位高亮（不按文本匹配：重复句会跳行闪烁，刚踩过）
    es.addEventListener('lyric-state', (ev) => {
      try {
        const d = JSON.parse(ev.data);
        if (d && typeof d.line === 'string') hcBus.emit('lyric-line', { text: d.line, ct: d.current_time, offset: d.offset || 0 });
      } catch (e) {}
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
    seeking = true; // 防 timeupdate 覆盖拖动中的进度条
    const d = pb().duration || audio.duration || 1;
    el.cur.textContent = fmt((el.prog.value / 1000) * d);
  });
  el.vol.addEventListener('input', () => setVol(el.vol.value));
  el.toggle.addEventListener('click', toggle);
  // dock 停止按钮 → 核心停止（当前音箱→停+回web；非当前设备→no-op），与设备页同一命令
  if (el.stop) el.stop.addEventListener('click', () => {
    const p = pb();
    if (p.spk_did) window.hcState.cmd('stop_speaker', { did: p.spk_did });
  });
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
    window.hcState.cmd('play', { bvid: d.bvid, title: d.title || '', artist: d.artist || '', cover: d.cover || '', duration: Number(d.duration) || 0 });
  });
  hcBus.on('toggle', toggle);
  hcBus.on('seek', (d) => { if (d && typeof d.pct === 'number') seek(d.pct); });
  hcBus.on('prev', () => step('prev'));
  hcBus.on('next', () => step('next'));
  hcBus.on('play-next', (d) => { if (d && d.bvid) window.hcState.cmd('play', { bvid: d.bvid, title: d.title || '', artist: d.artist || '', cover: d.cover || '', duration: Number(d.duration) || 0 }); });
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
        const ok = t.includes('正在音箱播放');
        // 注意：投送成功即切音箱模式由后端一步完成（/hx/speaker/play 内同步
        // switch_speaker）——前端不再二次发命令，消除双响时间窗口
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
  // ---- 状态流变化 → 驱动音频 + 渲染（事件驱动，不轮询）----
  // state.js applyState/applyProgress 更新 Alpine.store 后 emit 'state'（含进度变化），
  // 音箱模式 bvid/playing/source 不变、position 每秒变化 → 直接依赖该事件触发重绘。
  if (window.hcBus) {
    hcBus.on('state', () => {
      // 切回 web：恢复 web 本地音量态。web/speaker 是独立音域，speaker 静音（volMuted=true）
      // 不得残留到 web（否则切回 web 音频被静音/滑块被压 0）。web 音量只认 localStorage。
      const src = window.hcState?.get()?.source;
      if (src === 'web' && lastSource === 'speaker') {
        volMuted = false;
        const v = parseInt(localStorage.getItem('hc:vol') || '80', 10) || 80;
        audio.volume = v / 100;
        audio.muted = false;
        if (el.vol) el.vol.value = v;
        renderMute();
      }
      if (src) lastSource = src;
      driveAudio(); render(); updateMediaMeta();
    });
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
