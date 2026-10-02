// 歌词面板孤岛（M2）：KTV 模式，收 hc:lyric-open（player 封面点击广播）
// 歌词数据直连 JSON API（孤岛职责内的 HTTP 取数），不共享任何状态
(function () {
  const panel = document.getElementById('lyric-panel');
  if (!panel) return;
  const backdrop = document.getElementById('lyric-backdrop');
  const linesEl = document.getElementById('ly-lines');
  const titleEl = document.getElementById('ly-title');
  const artistEl = document.getElementById('ly-artist');
  const audio = document.getElementById('audio');

  let lines = []; // [[sec, text], ...]
  let lastIdx = -1;
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

  function show() { panel.classList.remove('hidden'); document.body.style.overflow = 'hidden'; }
  function hide() {
    panel.classList.add('hidden');
    document.body.style.overflow = '';
    lines = []; lastIdx = -1; linesEl.innerHTML = '';
  }
  function esc(e) { if (e.key === 'Escape') hide(); }

  function renderLines() {
    linesEl.innerHTML = '';
    lines.forEach((l, i) => {
      const d = document.createElement('div');
      d.className = 'ly-line text-gray-500 my-1 transition-all duration-300 px-4 py-1 text-center cursor-pointer hover:text-white/70';
      d.textContent = l[1];
      // 点击行 → 跳转到该句时间（YesPlayMusic 同款）
      d.addEventListener('click', () => {
        const t = lines[i][0];
        if (audio) { audio.currentTime = t + 0.1; }
        lastIdx = -1;
        sync(t);
      });
      linesEl.appendChild(d);
    });
  }
  function sync(sec) {
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
    titleEl.textContent = song.title || '';
    artistEl.textContent = song.artist || '';
    linesEl.innerHTML = '<p class="text-gray-400">歌词加载中…</p>';
    try {
      const kw = encodeURIComponent(((song.title || '') + ' ' + (song.artist || '')).trim());
      const j = await (await fetch('/api/v1/music/lyric/candidates?keyword=' + kw + '&limit=3')).json();
      const cand = j && j.data && j.data[0];
      if (!cand || !cand.lines || !cand.lines.length) {
        linesEl.innerHTML = '<p class="text-gray-400">未找到歌词</p>';
        return;
      }
      lines = cand.lines;
      renderLines();
      lastIdx = -1;
      sync(audio.currentTime);
    } catch (e) {
      linesEl.innerHTML = '<p class="text-gray-400">歌词加载失败</p>';
    }
  }

  hcBus.on('lyric-open', (song) => {
    if (!song || !song.bvid) return;
    show();
    load(song);
  });

  // KTV 跟随（直接听 audio，孤岛内）
  audio.addEventListener('timeupdate', () => { if (!panel.classList.contains('hidden')) sync(audio.currentTime); });

  // 底部进度条：跟随播放 + 可拖跳转
  const lyProg = document.getElementById('ly-progress');
  if (lyProg) {
    audio.addEventListener('timeupdate', () => {
      if (panel.classList.contains('hidden')) return;
      lyProg.value = audio.duration ? Math.round((audio.currentTime / audio.duration) * 1000) : 0;
    });
    lyProg.addEventListener('change', () => {
      if (audio.duration) audio.currentTime = (lyProg.value / 1000) * audio.duration;
    });
  }

  hcBus.on('lysize', (d) => { if (d && typeof d.idx === 'number') setLyIdx(d.idx - lyIdx); });

  // 字号按钮（标准化档位切换）
  document.getElementById('ly-small').addEventListener('click', () => setLyIdx(-1));
  document.getElementById('ly-big').addEventListener('click', () => setLyIdx(1));
  setLyIdx(0); // 应用已存档位并显示
})();
