// 播放器孤岛（M1/M2）：独立进程管理 audio 实时态（C 客户端瞬时态）
// 不 import 任何页面代码；只经 hcBus(hc:*) 与外界通信：
//   收 play{toggle,seek,pct,prev,next}，发 state/nowplaying/lyric-open
// 播放队列 = 进程内播放历史（任意页面点播入队）；P 态 = localStorage 恢复上次
(function () {
  const audio = document.getElementById('audio');
  if (!audio) return;
  const $ = (id) => document.getElementById(id);
  const el = {
    dock: $('player-dock'), toggle: $('d-toggle'), prev: $('d-prev'), next: $('d-next'),
    cover: $('d-cover'), title: $('d-title'), artist: $('d-artist'),
    cur: $('d-cur'), dur: $('d-dur'), prog: $('d-progress'), vol: $('d-vol'), mode: $('d-mode'), muteBtn: $('d-mute'),
    qualityBtn: $('d-quality'), speedBtn: $('d-speed'), eq: $('d-eq'),
  };
  if (!el.dock || !el.toggle) return;

  let song = null; // {bvid,title,artist,cover}
  let state = 'idle'; // idle/loading/playing/paused/ended
  let queue = []; // 播放队列（自动上下文），prev/next 队列走位
  let qidx = -1;
  let lastSaveAt = 0;
  let playMode = localStorage.getItem('hc:mode') || 'order'; // order/loop/single/random
  let quality = parseInt(localStorage.getItem('hc:quality') || '192', 10); // 64/128/192
  let speed = parseFloat(localStorage.getItem('hc:speed') || '1'); // 0.75~2.0

  const fmt = (s) => {
    if (!isFinite(s) || !s) return '0:00';
    const m = Math.floor(s / 60), x = Math.floor(s % 60);
    return m + ':' + String(x).padStart(2, '0');
  };

  // ---- P 设备持久态：恢复上次播放（localStorage，不依赖服务器） ----
  function saveP() {
    try {
      localStorage.setItem('hc:song', JSON.stringify({ song, at: audio.currentTime }));
    } catch (e) {}
  }
  function restoreP() {
    try {
      const raw = localStorage.getItem('hc:song');
      if (!raw) return;
      const d = JSON.parse(raw);
      if (!d || !d.song || !d.song.bvid) return;
      song = d.song;
      queue = [song]; qidx = 0;
      el.title.textContent = song.title;
      el.artist.textContent = song.artist || '未知作者';
      el.cover.src = song.cover || '';
      audio.src = '/api/v1/music/stream/' + song.bvid + '?quality=192';
      if (d.at > 5) audio.currentTime = d.at;
      setState('paused');
      hcBus.emit('nowplaying', { bvid: song.bvid });
    } catch (e) {}
  }

  // ---- 状态渲染 ----
  function render() {
    el.dock.classList.toggle('hidden', !song);
    const playing = state === 'playing';
    el.toggle.innerHTML = playing
      ? '<svg class="w-7 h-7" viewBox="0 0 24 24" fill="currentColor"><path d="M6 5h4v14H6zM14 5h4v14h-4z"/></svg>'
      : '<svg class="w-7 h-7 ml-0.5" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>';
    if (el.eq) el.eq.style.display = state === 'playing' ? 'flex' : 'none'; // 频谱只在真正播放时出现（inline 优先，防 flex 覆盖）
    el.prev.disabled = el.next.disabled = !song || queue.length < 2;
    el.prev.classList.toggle('opacity-40', el.prev.disabled);
    el.next.classList.toggle('opacity-40', el.next.disabled);
    renderMode();
  }

  // ---- 播放模式：order/loop/single/random（Vue 版同款轮换 + localStorage P 态） ----
  const MODE_ICONS = {
    order: '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6h16M4 12h16M4 18h16"/></svg>',
    loop: '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14M7 22l-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/></svg>',
    single: '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 2 21 6l-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="M7 22l-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/></svg><text x="9" y="17" font-size="7" fill="currentColor" stroke="none">1</text>',
    random: '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 3h5v5"/><path d="M4 20 21 3"/><path d="M21 16v5h-5"/><path d="m15 15 6 6"/><path d="M4 4l5 5"/></svg>',
  };
  const MODE_ORDER = ['order', 'loop', 'single', 'random'];
  function renderMode() {
    if (!el.mode) return;
    el.mode.innerHTML = MODE_ICONS[playMode] || MODE_ICONS.order;
    el.mode.title = { order: '顺序播放', loop: '列表循环', single: '单曲循环', random: '随机播放' }[playMode];
    el.mode.classList.toggle('text-pink-400', playMode !== 'order');
    el.mode.classList.toggle('hover:bg-gray-100', true);
  }
  function setPlayMode(m) {
    playMode = m;
    try { localStorage.setItem('hc:mode', m); } catch (e) {}
    renderMode();
    hcBus.emit('toast', { msg: { order: '顺序播放', loop: '列表循环', single: '单曲循环', random: '随机播放' }[m] });
  }
  function cycleMode() {
    setPlayMode(MODE_ORDER[(MODE_ORDER.indexOf(playMode) + 1) % MODE_ORDER.length]);
  }

  // ---- 播放队列（C 态，进程内）：自动上下文 + 插队（参照 YesPlayMusic next 页） ----
  function emitQueue() {
    hcBus.emit('queue', {
      list: queue.map((q) => ({ bvid: q.bvid, title: q.title, artist: q.artist, cover: q.cover })),
      idx: qidx,
    });
  }
  function notify(d) { // 切歌系统通知（一次性请求权限，失败静默）
    try {
      if (!('Notification' in window)) return;
      if (Notification.permission === 'granted') {
        new Notification(d.title || '', { body: d.artist || '', icon: d.cover || undefined, silent: true });
      } else if (Notification.permission === 'default') {
        Notification.requestPermission();
      }
    } catch (e) {}
  }
  function rememberRecent(d) { // 最近播放（去重保序，最多 30 首）
    try {
      let list = JSON.parse(localStorage.getItem('hc:recent') || '[]');
      list = list.filter((x) => x && x.bvid !== d.bvid);
      list.unshift({ bvid: d.bvid, title: d.title || '', artist: d.artist || '', cover: d.cover || '', duration: d.duration || 0 });
      if (list.length > 30) list = list.slice(0, 30);
      localStorage.setItem('hc:recent', JSON.stringify(list));
    } catch (e) {}
  }
  function loadSong(d) {
    song = d;
    notify(d);
    rememberRecent(d);
    hcBus.emit('now', d); // 歌词页/其他孤岛取当前曲
    el.title.textContent = d.title;
    el.artist.textContent = d.artist || '未知作者';
    el.cover.src = d.cover || '';
    el.cover.alt = d.title;
    audio.src = '/api/v1/music/stream/' + d.bvid + '?quality=' + quality;
    audio.play().catch(() => {});
    setState('loading');
    hcBus.emit('nowplaying', { bvid: d.bvid });
    saveP();
  }
  function play(data) { // 外部点播：去重后进队尾并播放
    const i = queue.findIndex((q) => q.bvid === data.bvid);
    if (i >= 0) queue.splice(i, 1);
    queue.push({ bvid: data.bvid, title: data.title, artist: data.artist, cover: data.cover });
    qidx = queue.length - 1;
    loadSong(data);
    emitQueue();
  }
  function playNext(data) { // 「下一首播放」：插到当前歌后面，不打断播放
    if (!song) { play(data); return; }
    const i = queue.findIndex((q) => q.bvid === data.bvid);
    if (i >= 0) queue.splice(i, 1);
    queue.splice(qidx + 1, 0, { bvid: data.bvid, title: data.title, artist: data.artist, cover: data.cover });
    emitQueue();
  }
  function step(dir) { // 队列走位：next 向后 / prev 向前（循环；prev 播放中先回秒）
    if (queue.length < 2) return;
    if (dir === 'prev' && audio.currentTime > 3) { audio.currentTime = 0; return; }
    qidx = (qidx + (dir === 'next' ? 1 : -1) + queue.length) % queue.length;
    const t = queue[qidx];
    if (t) loadSong(t);
    emitQueue();
  }
  function toggle() { if (!song) return; audio.paused ? audio.play().catch(() => {}) : audio.pause(); }
  function seek(pct) { if (!song || !audio.duration) return; audio.currentTime = (pct / 100) * audio.duration; }
  function setQuality(q) { // 切换后若在播则按新音质重载（Vue 版同逻辑）
    quality = q;
    try { localStorage.setItem('hc:quality', String(q)); } catch (e) {}
    if (el.qualityBtn) el.qualityBtn.textContent = q + 'k';
    if (song) loadSong(song);
    hcBus.emit('toast', { msg: '音质 ' + q + 'k' });
  }
  function setSpeed(v) {
    speed = v;
    audio.playbackRate = v;
    try { localStorage.setItem('hc:speed', String(v)); } catch (e) {}
    if (el.speedBtn) el.speedBtn.textContent = v + 'x';
    hcBus.emit('toast', { msg: '速度 ' + v + 'x' });
  }
  function setVol(v) {
    audio.volume = v / 100;
    audio.muted = v === 0;
    try { localStorage.setItem('hc:vol', String(v)); } catch (e) {}
  }
  function setState(s) {
    state = s;
    render();
    hcBus.emit('state', { state, song });
    updateMediaMeta();
  }

  // ---- MediaSession：系统媒体键 / 媒体栏可见（YesPlayMusic 同款） ----
  function updateMediaMeta() {
    if (!('mediaSession' in navigator)) return;
    if (song) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: song.title || '',
        artist: song.artist || '',
        artwork: song.cover ? [{ src: song.cover, sizes: '512x512' }] : [],
      });
    }
    const acting = { playing: !audio.paused, paused: audio.paused, previoustrack: true, nexttrack: true };
    navigator.mediaSession.playbackState = audio.paused ? 'paused' : 'playing';
    try {
      navigator.mediaSession.setActionHandler('play', () => audio.play().catch(() => {}));
      navigator.mediaSession.setActionHandler('pause', () => audio.pause());
      navigator.mediaSession.setActionHandler('previoustrack', () => step('prev'));
      navigator.mediaSession.setActionHandler('nexttrack', () => step('next'));
    } catch (e) {}
  }

  // ---- widget 上报（节流 5s；桌面挂件轮询用，薄封装直连 API） ----
  function reportWidget() {
    if (!song) return;
    fetch('/api/v1/widget/state', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        bvid: song.bvid, title: song.title, artist: song.artist || '',
        current_time: audio.currentTime, duration: audio.duration || 0,
        playing: !audio.paused,
      }),
    }).catch(() => {});
  }

  // ---- audio 事件 → 状态机 ----
  audio.addEventListener('play', () => setState('playing'));
  audio.addEventListener('pause', () => { setState(audio.ended ? 'ended' : 'paused'); if (el.eq) el.eq.style.display = 'none'; });
  audio.addEventListener('ended', () => { if (el.eq) el.eq.style.display = 'none'; });
  audio.addEventListener('error', () => { if (el.eq) el.eq.style.display = 'none'; });
  audio.addEventListener('ended', () => {
    setState('ended');
    if (playMode === 'single' && song) { audio.currentTime = 0; audio.play().catch(() => {}); return; } // 单曲循环
    if (playMode === 'random') { // 随机下一首（不重复当前）
      if (queue.length < 2) return;
      let n;
      do { n = Math.floor(Math.random() * queue.length); } while (n === qidx);
      qidx = n;
      loadSong(queue[qidx]);
      emitQueue();
      return;
    }
    step('next'); // order/loop：走队（循环）
  });
  audio.addEventListener('loadedmetadata', () => { el.dur.textContent = fmt(audio.duration); });
  audio.addEventListener('timeupdate', () => {
    if (!song) return;
    el.cur.textContent = fmt(audio.currentTime);
    const d = audio.duration || 1;
    el.prog.value = Math.round((audio.currentTime / d) * 1000);
    const now = Date.now();
    if (now - lastSaveAt > 5000) { lastSaveAt = now; saveP(); reportWidget(); }
  });
  audio.addEventListener('play', () => reportWidget());
  audio.addEventListener('pause', () => reportWidget());

  // ---- 控制 ----
  el.prog.addEventListener('change', () => seek((el.prog.value / 1000) * 100));
  el.vol.addEventListener('input', () => setVol(el.vol.value));
  el.toggle.addEventListener('click', toggle);
  el.prev.addEventListener('click', () => step('prev'));
  el.next.addEventListener('click', () => step('next'));
  el.cover.addEventListener('click', () => {
    if (!song) return;
    // 封面点击 toggle：展开歌词面板 / 已展开则缩回（可展开可缩回去）
    const lyp = document.getElementById('lyric-panel');
    if (lyp && !lyp.classList.contains('ly-hidden')) {
      hcBus.emit('lyric-close');
    } else {
      hcBus.emit('lyric-open', song);
    }
  });

  if (el.mode) el.mode.addEventListener('click', cycleMode);
  if (el.qualityBtn) el.qualityBtn.addEventListener('click', () => {
    setQuality(quality === 64 ? 128 : quality === 128 ? 192 : 64);
  });
  if (el.speedBtn) el.speedBtn.addEventListener('click', () => {
    const speeds = [0.75, 1, 1.25, 1.5, 2];
    setSpeed(speeds[(speeds.indexOf(speed) + 1) % speeds.length]);
  });
  audio.playbackRate = speed;
  function mute() { audio.muted = !audio.muted; }
  if (el.muteBtn) el.muteBtn.addEventListener('click', mute);
  audio.volume = parseInt(localStorage.getItem('hc:vol') || '80', 10) / 100;

  // ---- 总线 ----
  hcBus.on('play', play);
  hcBus.on('toggle', toggle);
  hcBus.on('seek', (d) => seek(d.pct));
  hcBus.on('prev', () => step('prev'));
  hcBus.on('next', () => step('next'));
  hcBus.on('play-next', playNext);
  hcBus.on('play-fav-all', async () => { // 收藏夹播放全部（直连 JSON API，薄封装）
    try {
      const j = await (await fetch('/api/v1/fav/list')).json();
      const items = j.data || [];
      if (!items.length) { hcBus.emit('toast', { msg: '收藏夹是空的' }); return; }
      queue = items.map((x) => ({ bvid: x.bvid, title: x.title, artist: x.artist, cover: x.cover }));
      qidx = 0;
      loadSong(queue[0]);
      emitQueue();
    } catch (e) { hcBus.emit('toast', { msg: '加载失败' }); }
  });
  hcBus.on('play-all-list', (d) => { // 任意列表「播放全部」（调用侧从 DOM 收集数据）
    const items = ((d && d.items) || []).filter((x) => x && x.bvid);
    if (!items.length) return;
    queue = items;
    qidx = 0;
    loadSong(queue[0]);
    emitQueue();
  });
  hcBus.on('queue-clear', () => { // 清空队列：保留当前歌继续播，其余移除
    queue = [];
    qidx = -1;
    emitQueue();
    render();
    hcBus.emit('toast', { msg: '队列已清空' });
  });
  hcBus.on('queue-remove', (d) => { // 从队列删除：删当前则自动切下一首，删空则收 dock
    const i = queue.findIndex((q) => q.bvid === d.bvid);
    if (i < 0) return;
    queue.splice(i, 1);
    if (i < qidx) qidx--;
    else if (i === qidx) {
      if (queue.length) {
        qidx = Math.min(qidx, queue.length - 1);
        loadSong(queue[qidx]);
      } else {
        song = null;
        audio.pause();
        setState('idle');
        hcBus.emit('nowplaying', { bvid: '' });
      }
    }
    emitQueue();
  });
  hcBus.on('mute', mute);
  hcBus.on('mode-cycle', cycleMode);
  hcBus.on('mode', (d) => { if (d && d.m) setPlayMode(d.m); });

  hcBus.on('request-now', () => { if (song) hcBus.emit('now', song); }); // 歌词页初始化拉当前曲
  hcBus.on('quality', (d) => { if (d && d.q) setQuality(d.q); });
  hcBus.on('speed', (d) => { if (d && d.v) setSpeed(d.v); });

  // ---- 投送桥接：cast 页按钮只广播 hc:cast-play，这里持当前歌调后端投送 ----
  hcBus.on('cast-play', (d) => {
    if (!song) {
      hcBus.emit('cast-result', { ok: false, udn: d.udn, msg: '请先播放一首歌' });
      return;
    }
    const body = new URLSearchParams({ bvid: song.bvid, udn: d.udn, title: song.title });
    fetch('/hx/cast/play', { method: 'POST', body })
      .then((r) => r.text())
      .then((t) => hcBus.emit('cast-result', { ok: t.includes('成功'), udn: d.udn, msg: '投送失败' }))
      .catch(() => hcBus.emit('cast-result', { ok: false, udn: d.udn, msg: '网络错误' }));
  });

  // ---- 音箱投送桥接：同 cast 模式，player 持当前歌调后端 ----
  hcBus.on('speaker-play', (d) => {
    if (!song) {
      hcBus.emit('speaker-result', { ok: false, did: d.did, msg: '请先播放一首歌' });
      return;
    }
    const body = new URLSearchParams({ bvid: song.bvid, did: d.did });
    fetch('/hx/speaker/play', { method: 'POST', body })
      .then((r) => r.text())
      .then((t) => hcBus.emit('speaker-result', { ok: t.includes('音箱播放'), did: d.did, msg: '投送失败' }))
      .catch(() => hcBus.emit('speaker-result', { ok: false, did: d.did, msg: '网络错误' }));
  });

  restoreP();
  render();
})();
