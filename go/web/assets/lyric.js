// 歌词面板孤岛：竖向 KTV（dock 封面点击弹出，实底不透明、非独立页面）。
// 宿主元素在 shell（lyric-panel 常驻 DOM），一次绑定即可。
// 数据直连 JSON API；当前曲经 hc:lyric-open 传入（player 封面点击广播）。
(function () {
  const panel = document.getElementById('lyric-panel');
  if (!panel) return;
  const linesEl = document.getElementById('ly-lines');
  const titleEl = document.getElementById('ly-title');
  const artistEl = document.getElementById('ly-artist');
  const coverEl = document.getElementById('ly-cover');
  const audio = document.getElementById('audio');
  if (!linesEl || !audio) return;

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

  function fmt(s) {
    s = Math.max(0, Math.floor(s || 0));
    const m = Math.floor(s / 60), r = s % 60;
    return m + ':' + String(r).padStart(2, '0');
  }

  function show() { // 滑入动画：先显示再加 .ly-show（双 rAF 保证过渡生效）
    panel.classList.remove('ly-hidden');
    void panel.offsetWidth; // 强制 reflow
    panel.classList.add('ly-show');
  }
  function hide() { // 先过渡收起，300ms 后再隐藏（不遮挡 dock）
    panel.classList.remove('ly-show');
    setTimeout(() => { if (!panel.classList.contains('ly-show')) panel.classList.add('ly-hidden'); }, 300);
    lines = []; lastIdx = -1; linesEl.innerHTML = '';
  }

  // 歌词时间显示开关（YesPlayMusic showLyricsTime 同款；hc:lytime 持久）
  let lyTime = false;
  try { lyTime = localStorage.getItem('hc:lytime') === '1'; } catch (e) {}

  function renderLines() {
    linesEl.innerHTML = '';
    if (!lines.length) {
      linesEl.innerHTML = '<div class="h-full flex items-center justify-center"><p class="text-gray-400">未找到歌词</p></div>';
      return;
    }
    lines.forEach((l, i) => {
      const d = document.createElement('div');
      d.className = 'ly-line text-gray-500 my-1 transition-all duration-300 px-4 py-1 text-center cursor-pointer hover:text-white/70 flex items-center justify-center gap-2';
      if (lyTime) {
        const t = document.createElement('span');
        t.className = 'text-[10px] text-gray-500 font-mono tabular-nums flex-shrink-0 opacity-60';
        t.textContent = fmt(l[0]);
        d.appendChild(t);
      }
      const txt = document.createElement('span');
      txt.textContent = l[1];
      d.appendChild(txt);
      // 点击行 → 跳到该句时间（YesPlayMusic 同款）
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
    titleEl.textContent = song.title || '';
    artistEl.textContent = song.artist || '';
    if (coverEl && song.cover) coverEl.src = song.cover;
    if (bgCover && song.cover) bgCover.style.backgroundImage = 'url("' + song.cover + '")'; // 封面模糊背景（网易云同款）
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

  // ---- 歌词背景模式：封面模糊（cover，默认）/ 纯色（solid）----
  const bgCover = document.getElementById('ly-bg-cover');
  const bgSolid = document.getElementById('ly-bg-solid');
  let lyBg = 'cover';
  try { lyBg = localStorage.getItem('hc:lybg') || 'cover'; } catch (e) {}
  function applyLyBg() {
    const cover = lyBg === 'cover';
    if (bgCover) bgCover.classList.toggle('hidden', !cover);
    if (bgSolid) bgSolid.classList.toggle('hidden', cover);
  }
  hcBus.on('lybg', (d) => { if (d && d.v) { lyBg = d.v; applyLyBg(); } });
  applyLyBg();

  // 打开：player 封面点击广播（Vue 版同款交互）
  hcBus.on('lyric-open', (song) => {
    if (!song || !song.bvid) return;
    try { lyTime = localStorage.getItem('hc:lytime') === '1'; } catch (e) {} // 打开时同步开关（防页面加载后外部改动）
    show();
    load(song);
  });

  // 关闭：X 按钮 / ESC
  document.getElementById('ly-close').addEventListener('click', hide);
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && !panel.classList.contains('ly-hidden')) hide(); });

  // KTV 跟随 + 时间显示（直接听 audio，孤岛内；面板关闭时不更新）
  const curEl = document.getElementById('ly-cur');
  const durEl = document.getElementById('ly-dur');
  audio.addEventListener('timeupdate', () => {
    if (panel.classList.contains('ly-hidden')) return;
    sync(audio.currentTime);
    if (curEl) curEl.textContent = fmt(audio.currentTime);
    if (durEl && audio.duration) durEl.textContent = fmt(audio.duration);
  });

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

  // 设置页联动（hc:lysize 绝对档位）与字号按钮
  hcBus.on('lysize', (d) => { if (d && typeof d.idx === 'number') setLyIdx(d.idx - lyIdx); });
  hcBus.on('lytime', (d) => { if (d && typeof d.on === 'boolean') { lyTime = d.on; renderLines(); lastIdx = -1; sync(audio.currentTime); } });
  document.getElementById('ly-small').addEventListener('click', () => setLyIdx(-1));
  document.getElementById('ly-big').addEventListener('click', () => setLyIdx(1));
  setLyIdx(0); // 应用已存档位并显示
})();