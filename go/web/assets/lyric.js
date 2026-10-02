// 歌词页孤岛：竖向 KTV（独立页面 /hx/lyric，实底不透明）。
// 宿主元素在本页内（ly-lines/ly-progress/字号按钮...），其他页面元素缺省即跳过。
// 数据直连 JSON API（孤岛职责内取数），当前曲经 hc:now / hc:request-now 获取。
(function () {
  let inited = false;
  function init() {
  const view = document.getElementById('lyric-view');
  if (!view) { inited = false; return; }
  if (inited) return;
  inited = true;
  const linesEl = document.getElementById('ly-lines');
  const titleEl = document.getElementById('ly-title');
  const artistEl = document.getElementById('ly-artist');
  const coverEl = document.getElementById('ly-cover');
  const audio = document.getElementById('audio');
  if (!linesEl || !audio) return;

  let lines = []; // [[sec, text], ...]
  let lastIdx = -1;
  let current = null; // 当前曲 {bvid,title,artist,cover}

  // 字号标准化 5 档（14/16/18/22/26px），档位数字展示（无黑盒）
  const LY_SIZES = [14, 16, 18, 22, 26];
  let lyIdx = LY_SIZES.indexOf(parseInt(localStorage.getItem('hc:lysize') || '18', 10));
  if (lyIdx < 0) lyIdx = 2;

  function setLyIdx(delta) {
    lyIdx = Math.min(LY_SIZES.length - 1, Math.max(0, lyIdx + (delta || 0)));
    const v = LY_SIZES[lyIdx];
    try { localStorage.setItem('hc:lysize', String(v)); } catch (e) {}
    linesEl.style.setProperty('--line-size', v + 'px');
    const badge = document.getElementById('ly-size');
    if (badge) badge.textContent = (lyIdx + 1) + '/' + LY_SIZES.length;
  }

  function fmt(s) {
    s = Math.max(0, Math.floor(s || 0));
    const m = Math.floor(s / 60), r = s % 60;
    return m + ':' + String(r).padStart(2, '0');
  }

  function esc(x) {
    return String(x ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  function renderLines() {
    linesEl.innerHTML = '';
    if (!lines.length) {
      linesEl.innerHTML = '<div class="h-full flex items-center justify-center"><p class="text-gray-400">未找到歌词</p></div>';
      return;
    }
    lines.forEach((l, i) => {
      const d = document.createElement('div');
      d.className = 'ly-line text-gray-500 my-1 transition-all duration-300 px-4 py-1 text-center cursor-pointer hover:text-white/70';
      d.textContent = l[1];
      d.addEventListener('click', () => {
        const t = lines[i][0];
        audio.currentTime = t + 0.1;
        lastIdx = -1;
        sync(t);
      });
      linesEl.appendChild(d);
    });
  }

  function sync(sec) {
    if (!lines.length) return;
    let idx = -1;
    for (let i = 0; i < lines.length; i++) {
      if (lines[i][0] <= sec) idx = i; else break;
    }
    if (idx === lastIdx) return;
    lastIdx = idx;
    linesEl.querySelectorAll('.ly-line').forEach((d, i) => {
      const active = i === idx;
      d.classList.toggle('ly-active', active);
      d.classList.toggle('text-white', active);
      d.classList.toggle('font-semibold', active);
      d.classList.toggle('text-gray-500', !active);
      d.classList.toggle('scale-100', !active);
      d.classList.toggle('scale-110', active);
    });
    if (idx >= 0) linesEl.children[idx]?.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }

  async function load(song) {
    current = song;
    titleEl.textContent = song.title || '';
    artistEl.textContent = song.artist || '';
    if (coverEl) coverEl.src = song.cover || '';
    linesEl.innerHTML = '<div class="h-full flex items-center justify-center"><p class="text-gray-400">歌词加载中…</p></div>';
    try {
      const kw = encodeURIComponent(((song.title || '') + ' ' + (song.artist || '')).trim());
      const j = await (await fetch('/api/v1/music/lyric/candidates?keyword=' + kw + '&limit=3')).json();
      const cand = j && j.data && j.data[0];
      if (!cand || !cand.lines || !cand.lines.length) {
        lines = [];
        renderLines();
        return;
      }
      lines = cand.lines;
      renderLines();
      lastIdx = -1;
      sync(audio.currentTime);
    } catch (e) {
      linesEl.innerHTML = '<div class="h-full flex items-center justify-center"><p class="text-gray-400">歌词加载失败</p></div>';
    }
  }

  // 当前曲获取：直接响应 now，或主动请求（进页面前的事件可能已错过）
  hcBus.on('now', (song) => { if (song && song.bvid) load(song); });
  hcBus.emit('request-now');
  setTimeout(() => { if (!current) hcBus.emit('request-now'); }, 400);

  // KTV 跟随 + 时间显示（直接听 audio，孤岛内）
  const curEl = document.getElementById('ly-cur');
  const durEl = document.getElementById('ly-dur');
  audio.addEventListener('timeupdate', () => {
    sync(audio.currentTime);
    if (curEl) curEl.textContent = fmt(audio.currentTime);
    if (durEl) durEl.textContent = fmt(audio.duration);
  });
  audio.addEventListener('loadedmetadata', () => { if (durEl && audio.duration) durEl.textContent = fmt(audio.duration); });

  // 底部进度条：跟随播放 + 可拖跳转
  const lyProg = document.getElementById('ly-progress');
  if (lyProg) {
    audio.addEventListener('timeupdate', () => {
      lyProg.value = audio.duration ? Math.round((audio.currentTime / audio.duration) * 1000) : 0;
    });
    lyProg.addEventListener('change', () => {
      if (audio.duration) audio.currentTime = (lyProg.value / 1000) * audio.duration;
    });
    // 时长空态
    if (durEl && !audio.duration) durEl.textContent = '--:--';
  }

  // 返回按钮：回上一页（无历史则回队列）
  const back = document.getElementById('ly-back');
  if (back) back.addEventListener('click', () => {
    if (history.length > 1) history.back();
    else if (window.htmx) htmx.ajax('GET', '/hx/queue', { target: '#main', select: '#main', swap: 'innerHTML', pushUrl: true });
    else location.href = '/hx/queue';
  });

  // 设置页联动（hc:lysize 绝对档位）与字号按钮
  hcBus.on('lysize', (d) => { if (d && typeof d.idx === 'number') setLyIdx(d.idx - lyIdx); });
  const sBtn = document.getElementById('ly-small');
  const bBtn = document.getElementById('ly-big');
  if (sBtn) sBtn.addEventListener('click', () => setLyIdx(-1));
  if (bBtn) bBtn.addEventListener('click', () => setLyIdx(1));
  setLyIdx(0); // 应用已存档位并显示
  }
  document.addEventListener('htmx:afterSwap', init);
  init();
})();