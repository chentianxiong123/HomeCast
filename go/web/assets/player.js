// 播放器孤岛（M1）：独立进程管理 audio 实时态（C 客户端瞬时态）
// 不 import 任何页面代码；只经 hcBus(hc:*) 与外界通信：
//   收 play{toggle,seek,pct}，发 state/nowplaying
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

  const fmt = (s) => {
    if (!isFinite(s) || !s) return '0:00';
    const m = Math.floor(s / 60), x = Math.floor(s % 60);
    return m + ':' + String(x).padStart(2, '0');
  };

  function render() {
    el.dock.classList.toggle('hidden', !song);
    const playing = state === 'playing';
    el.toggle.innerHTML = playing
      ? '<svg class="w-7 h-7" viewBox="0 0 24 24" fill="currentColor"><path d="M6 5h4v14H6zM14 5h4v14h-4z"/></svg>'
      : '<svg class="w-7 h-7 ml-0.5" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>';
    el.prev.disabled = el.next.disabled = !song;
    el.prev.classList.toggle('opacity-40', !song);
    el.next.classList.toggle('opacity-40', !song);
  }

  function play(data) {
    song = data;
    el.title.textContent = data.title;
    el.artist.textContent = data.artist || '未知作者';
    el.cover.src = data.cover || '';
    el.cover.alt = data.title;
    audio.src = '/api/v1/music/stream/' + data.bvid + '?quality=192';
    audio.play().catch(() => {});
    setState('loading');
    hcBus.emit('nowplaying', { bvid: data.bvid });
  }
  function toggle() { if (!song) return; audio.paused ? audio.play().catch(() => {}) : audio.pause(); }
  function seek(pct) { if (!song || !audio.duration) return; audio.currentTime = (pct / 100) * audio.duration; }
  function setVol(v) { audio.volume = v / 100; audio.muted = v === 0; }
  function setState(s) { state = s; render(); hcBus.emit('state', { state, song }); }

  // audio 事件 → 状态机
  audio.addEventListener('play', () => setState('playing'));
  audio.addEventListener('pause', () => setState(audio.ended ? 'ended' : 'paused'));
  audio.addEventListener('ended', () => setState('ended'));
  audio.addEventListener('loadedmetadata', () => { el.dur.textContent = fmt(audio.duration); });
  audio.addEventListener('timeupdate', () => {
    if (!song) return;
    el.cur.textContent = fmt(audio.currentTime);
    const d = audio.duration || 1;
    el.prog.value = Math.round((audio.currentTime / d) * 1000);
  });

  // 控制
  el.prog.addEventListener('change', () => seek((el.prog.value / 1000) * 100));
  el.vol.addEventListener('input', () => setVol(el.vol.value));
  el.toggle.addEventListener('click', toggle);
  el.prev.addEventListener('click', () => hcBus.emit('prev'));
  el.next.addEventListener('click', () => hcBus.emit('next'));
  audio.volume = 0.8;

  // 总线：播放指令来自卡片/列表（htmx 渲染侧广播，不 import）
  hcBus.on('play', play);
  hcBus.on('toggle', toggle);
  hcBus.on('seek', (d) => seek(d.pct));

  render();
})();
