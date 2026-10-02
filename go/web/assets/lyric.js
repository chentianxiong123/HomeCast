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
      d.className = 'ly-line text-gray-500 text-lg my-1 transition-all duration-300 px-4 py-1 text-center';
      d.textContent = l[1];
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
      d.classList.toggle('text-lg', !active);
      d.classList.toggle('text-2xl', active);
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

  document.getElementById('ly-close').addEventListener('click', hide);
  backdrop.addEventListener('click', hide);
  document.addEventListener('keydown', esc);
})();
