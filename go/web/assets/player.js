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
    cur: $('d-cur'), dur: $('d-dur'), prog: $('d-progress'), vol: $('d-vol'),
  };
  if (!el.dock || !el.toggle) return;

  let song = null; // {bvid,title,artist,cover}
  let state = 'idle'; // idle/loading/playing/paused/ended
  let queue = []; // 播放历史（去重），prev/next 队列走位
  let qidx = -1;
  let lastSaveAt = 0;

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
    el.prev.disabled = el.next.disabled = !song || queue.length < 2;
    el.prev.classList.toggle('opacity-40', el.prev.disabled);
    el.next.classList.toggle('opacity-40', el.next.disabled);
  }

  // ---- 播放 ----
  function enqueue(d) {
    const i = queue.findIndex((q) => q.bvid === d.bvid);
    if (i >= 0) { qidx = i; return; }
    queue.push({ bvid: d.bvid, title: d.title, artist: d.artist, cover: d.cover });
    qidx = queue.length - 1;
  }
  function play(data) {
    enqueue(data);
    song = data;
    el.title.textContent = data.title;
    el.artist.textContent = data.artist || '未知作者';
    el.cover.src = data.cover || '';
    el.cover.alt = data.title;
    audio.src = '/api/v1/music/stream/' + data.bvid + '?quality=192';
    audio.play().catch(() => {});
    setState('loading');
    hcBus.emit('nowplaying', { bvid: data.bvid });
    saveP();
  }
  function toggle() { if (!song) return; audio.paused ? audio.play().catch(() => {}) : audio.pause(); }
  function seek(pct) { if (!song || !audio.duration) return; audio.currentTime = (pct / 100) * audio.duration; }
  function setVol(v) { audio.volume = v / 100; audio.muted = v === 0; }
  function step(dir) { // 队列走位：next 向后、prev 向前（循环）
    if (queue.length < 2) return;
    const i = queue.findIndex((q) => q.bvid === (song && song.bvid));
    qidx = dir === 'next' ? (i + 1) % queue.length : (i - 1 + queue.length) % queue.length;
    const t = queue[qidx];
    if (t) play({ bvid: t.bvid, title: t.title, artist: t.artist, cover: t.cover });
  }
  function setState(s) {
    state = s;
    render();
    hcBus.emit('state', { state, song });
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
  audio.addEventListener('pause', () => setState(audio.ended ? 'ended' : 'paused'));
  audio.addEventListener('ended', () => setState('ended'));
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
    if (song) hcBus.emit('lyric-open', song); // 封面点击 → 歌词面板（Vue 版同款）
  });
  audio.volume = 0.8;

  // ---- 总线 ----
  hcBus.on('play', play);
  hcBus.on('toggle', toggle);
  hcBus.on('seek', (d) => seek(d.pct));
  hcBus.on('prev', () => step('prev'));
  hcBus.on('next', () => step('next'));

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

  restoreP();
  render();
})();
