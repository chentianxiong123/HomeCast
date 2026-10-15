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
  let offset = 0; // 歌词对齐偏移（秒，±0.5 手动调，SQLite 持久化；music.py _lyric_offset 同款）
  let selMeta = null; // 后端已存选择（meta 端点：选的是哪个候选 + 对齐偏移）

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
      // 点击行 → 跳到该句时间（YesPlayMusic 同款；跳转位置减去对齐偏移保持所见即所听）
      d.addEventListener('click', () => {
        const t = lines[i][0] - offset;
        audio.currentTime = t + 0.1;
        lastIdx = -1;
        sync(t);
      });
      linesEl.appendChild(d);
    });
  }

  function sync(sec) {
    if (!lines.length) return;
    const t = sec + offset; // 对齐偏移：行时间 s <= 播放位置+偏移（music.py t = pos + offset）
    let idx = -1;
    for (let i = 0; i < lines.length; i++) {
      if (lines[i][0] <= t) idx = i; else break;
    }
    // 逐行进度（当前行唱到百分之几）：yrc 行自带时长（l[2]），LRC 用下一行差估算
    if (idx >= 0) {
      const start = lines[idx][0];
      const dur = (typeof lines[idx][2] === 'number' && lines[idx][2] > 0) ? lines[idx][2]
        : (lines[idx + 1] ? lines[idx + 1][0] - start : 0);
      const p = dur > 0 ? Math.min(1, Math.max(0, (t - start) / dur)) : 1;
      const d = linesEl.children[idx];
      if (d) d.style.setProperty('--prog', p.toFixed(3));
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

  let currentSong = null; // 侧栏操作（收藏/开B站/复制）作用的当前曲
  let loadGen = 0; // 换歌竞态保护：旧请求晚到直接丢弃（music.py bvid 校验同款，bili-music generation 同款）

  // ---- 歌词缓存（localStorage LRU 50 首）----
  const LY_CACHE_KEY = 'hc:lyc:cache';
  function lycGet(bvid) {
    try {
      const m = JSON.parse(localStorage.getItem(LY_CACHE_KEY) || '{}');
      const row = m[bvid];
      if (row) { row.at = Date.now(); m[bvid] = row; localStorage.setItem(LY_CACHE_KEY, JSON.stringify(m)); }
      return row || null;
    } catch (e) { return null; }
  }
  function lycSet(bvid, row) {
    try {
      const m = JSON.parse(localStorage.getItem(LY_CACHE_KEY) || '{}');
      row.at = Date.now();
      m[bvid] = row;
      const keys = Object.keys(m).sort((a, b) => (m[a].at || 0) - (m[b].at || 0));
      while (keys.length > 50) delete m[keys.shift()]; // LRU 淘汰最旧
      localStorage.setItem(LY_CACHE_KEY, JSON.stringify(m));
    } catch (e) {}
    // 同步 selMeta（当前高亮跟随本次选择）
    selMeta = { sid: row && row.id, name: row && row.name, artist: row && row.artist, duration: (row && (row.duration || row.dur)) || 0, offset: selMeta ? selMeta.offset : 0 };
    // 上报后端持久化“选的是哪个候选”（SQLite，增删改查；安卓壳丢 POST body，参数走 query）
    fetch('/api/v1/lyric/select?bvid=' + encodeURIComponent(bvid) +
      '&sid=' + encodeURIComponent((row && row.id) || 0) +
      '&name=' + encodeURIComponent((row && row.name) || '') +
      '&artist=' + encodeURIComponent((row && row.artist) || '') +
      '&duration=' + encodeURIComponent((row && (row.duration || row.dur)) || 0), { method: 'POST' })
      .catch(function () {});
  }
  // 来源行：当前歌词来自哪个网易云候选（无黑盒：可推导、可换）
  // 歌名解析（对齐 music.py _title_to_song：剥 []【】取《》; 取不到用全标题）
  function titleToSong(t) {
    const clean = (t || '').replace(/[\[【][^\]】]*[\]】]/g, '');
    const m = clean.match(/《([^》]+)》/);
    return m ? m[1].trim() : (clean.trim() || t || '');
  }
  // 时长秒 → mm:ss（候选行展示）
  const candDur = (c) => {
    const s = parseInt(c.duration || c.dur || 0, 10) || 0;
    return s > 0 ? Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0') : '--:--';
  };
  // 左侧候选栏渲染
  const candList = document.getElementById('ly-cand-list');
  const candTitle = document.getElementById('ly-cand-title');
  let cands = [];
  function renderCandBar() {
    if (candTitle) candTitle.textContent = '《' + titleToSong(currentSong && currentSong.title) + '》';
    if (!candList) return;
    if (!cands.length) {
      candList.innerHTML = '<div class="flex items-center justify-center h-full"><p class="text-xs text-gray-500">无歌词版本</p></div>';
      return;
    }
    candList.innerHTML = '';
    // 当前选中高亮：后端已存 sid（SQLite 持久化）优先，localStorage 缓存兜底；无则第一个
    const curId = (selMeta && selMeta.sid) || (lycGet(currentSong.bvid) || {}).id || 0;
    let chosen = null;
    let first = true;
    cands.forEach((c) => {
      const it = document.createElement('button');
      it.className = 'w-full text-left px-2.5 py-2 rounded-lg transition-colors ' +
        (first && !curId ? 'bg-pink-500/20 text-pink-300' : 'hover:bg-gray-800 text-gray-300');
      if (first && !curId && !chosen) { chosen = c; first = false; }
      if (curId && c.id === curId) it.className = 'bg-pink-500/20 text-pink-300';
      it.innerHTML = '<span class="block text-sm truncate">' + (c.name || '') + '</span>' +
        '<span class="block text-xs text-gray-500 truncate mt-0.5">' + (c.artist || '') + ' · ' + candDur(c) + '</span>';
      it.addEventListener('click', () => {
        if (currentSong) lycSet(currentSong.bvid, c); // 选定即缓存（music.py lyric_map 同款记忆）
        renderFromCand(c);
        renderCandBar();
      });
      candList.appendChild(it);
    });
    // 无选中来源时用第一个
    if (chosen) renderFromCand(chosen);
  }
  const sourceEl = document.getElementById('ly-source');
  function setSource(cand) {
    if (sourceEl) sourceEl.textContent = cand
      ? '来源：网易云 · ' + (cand.artist || '') + (cand.artist && cand.name ? '《' + cand.name + '》' : cand.name || '')
      : '未找到歌词 — 点侧栏 🔍 搜索歌词来源';
  }
  function renderFromCand(cand) {
    if (!cand || !cand.lines || !cand.lines.length) { lines = []; renderLines(); setSource(null); return; }
    lines = cand.lines;
    renderLines();
    lastIdx = -1;
    sync(audio.currentTime);
    setSource(cand);
  }
  async function load(song) {
    const gen = ++loadGen; // 竞态保护：本次加载的代际（music.py _panel_with bvid 校验同款）
    currentSong = song;
    titleEl.textContent = song.title || '';
    artistEl.textContent = song.artist || '';
    if (coverEl && song.cover) coverEl.src = song.cover;
    if (bgCover && song.cover) bgCover.style.backgroundImage = 'url("' + song.cover + '")'; // 封面模糊背景（网易云同款）
    linesEl.innerHTML = '<div class="h-full flex items-center justify-center"><p class="text-gray-400">歌词加载中…</p></div>';
    const stale = () => gen !== loadGen || !currentSong || currentSong.bvid !== song.bvid;
    // 恢复存档：选的是哪个候选 + 对齐偏移（SQLite 持久化，localStorage 清掉也不丢）
    selMeta = null;
    try {
      const mj = await (await fetch('/api/v1/lyric/meta?bvid=' + encodeURIComponent(song.bvid))).json();
      if (stale()) return; // 等待期间又切了歌 → 丢弃旧响应
      selMeta = (mj && mj.data) || null;
    } catch (e) {}
    setOffset(selMeta ? (parseFloat(selMeta.offset) || 0) : 0);
    // 先查缓存（同歌秒显，离线可看）；同时后台刷新候选栏（选中项=已存档 sid）
    const cached = lycGet(song.bvid);
    if (cached && cached.lines) renderFromCand(cached);
    try {
      const kw = encodeURIComponent(titleToSong(song.title) + ' ' + (song.artist || ''));
      const j = await (await fetch('/api/v1/music/lyric/candidates?keyword=' + kw + '&limit=8')).json();
      if (stale()) return; // 等待期间又切了歌 → 丢弃旧响应
      cands = (j && j.data) || [];
      renderCandBar(); // 候选栏渲染 + 无选中时用第一源
      if (!cands.length) { setSource(null); return; }
    } catch (e) {
      if (stale()) return;
      if (!lines.length) {
        linesEl.innerHTML = '<div class="h-full flex items-center justify-center"><p class="text-gray-400">歌词加载失败</p></div>';
      }
      setSource(null);
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

  // 关闭：X 按钮 / ESC / 封面再点（hc:lyric-close）
  document.getElementById('ly-close').addEventListener('click', hide);
  hcBus.on('lyric-close', hide);
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
  // ---- 歌词对齐 ±0.5s（music.py _nudge_lyric 同款：即时生效 + SQLite 持久化 + 广播字幕条随动） ----
  const nudgeVal = document.getElementById('ly-nudge-val');
  function setOffset(v) {
    offset = Math.round(v * 10) / 10; // 0.1s 精度防浮点丑数
    if (nudgeVal) nudgeVal.textContent = (offset > 0 ? '+' : '') + offset.toFixed(1) + 's';
    lastIdx = -1;
    if (lines.length) sync(audio.currentTime);
  }
  function nudge(d) {
    setOffset(offset + d);
    if (currentSong && currentSong.bvid) {
      // 对齐偏移持久化（回写 SQLite；安卓壳丢 POST body，参数走 query）
      fetch('/api/v1/lyric/offset?bvid=' + encodeURIComponent(currentSong.bvid) + '&offset=' + offset, { method: 'POST' }).catch(function () {});
      hcBus.emit('lyric-offset', { bvid: currentSong.bvid, offset: offset }); // 字幕条随动
    }
  }
  document.getElementById('ly-nudge-up').addEventListener('click', () => nudge(0.5));
  document.getElementById('ly-nudge-down').addEventListener('click', () => nudge(-0.5));
  // 一键归零：点击偏移值显示 → resetOffset（bili-music 同款；复用 nudge 路径持久化+广播）
  if (nudgeVal) nudgeVal.addEventListener('click', () => { if (offset !== 0) nudge(-offset); });
  // 侧栏歌曲操作（网易云歌词页同款）
  document.getElementById('ly-fav').addEventListener('click', () => {
    if (!currentSong) return;
    const fd = new FormData();
    fd.set('bvid', currentSong.bvid);
    fd.set('title', currentSong.title || '');
    fd.set('artist', currentSong.artist || '');
    fd.set('cover', currentSong.cover || '');
    fd.set('duration', String(currentSong.duration || 0));
    fetch('/hx/fav/toggle', { method: 'POST', body: fd })
      .then(() => hcBus.emit('toast', { msg: '收藏已切换 ♥' }))
      .catch(() => hcBus.emit('toast', { msg: '收藏失败' }));
  });
  document.getElementById('ly-copy').addEventListener('click', () => {
    if (!currentSong || !currentSong.bvid) return;
    navigator.clipboard.writeText('https://www.bilibili.com/video/' + currentSong.bvid)
      .then(() => hcBus.emit('toast', { msg: '链接已复制' }))
      .catch(() => hcBus.emit('toast', { msg: '复制失败' }));
  });
  setLyIdx(0); // 应用已存档位并显示
})();