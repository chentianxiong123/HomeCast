// 网页字幕条（孤岛）：当前播放歌词的常驻字幕（多态呈现——桌面版由 GTK 挂件承担，
// 本环境为网页/浏览器时显示网页内字幕条；壳环境 window.hcEnv==='desktop' 默认关）
(function () {
  const bar = document.getElementById('subtitle-bar');
  const textEl = document.getElementById('subtitle-text');
  const progEl = document.getElementById('subtitle-prog');
  const xBtn = document.getElementById('subtitle-x');
  const tickBtn = document.getElementById('d-tick');
  const audio = document.getElementById('audio');
  if (!bar || !audio) return;

  const envDesktop = window.hcEnv === 'desktop';
  let enabled = envDesktop ? false : localStorage.getItem('hc:subtitle') !== '0';
  let lines = [];   // [[sec, line], ...]
  let lastIdx = -1;
  let lastBvid = '';
  let songTitle = '';

  // ---- 拖动定位（对齐桌面挂件可移动；坐标持久化 hc:subpos） ----
  let subPos = null;
  try { subPos = JSON.parse(localStorage.getItem('hc:subpos') || 'null'); } catch (e) {}
  function place() {
    const w = bar.offsetWidth || 200, h = bar.offsetHeight || 36;
    // 非法持久位置防御（非数字 → 用默认居中）
    const px = (subPos && typeof subPos.x === 'number') ? subPos.x : null;
    const py = (subPos && typeof subPos.y === 'number') ? subPos.y : null;
    let x = px != null ? px : Math.round((window.innerWidth - w) / 2);
    let y = py != null ? py : Math.round(window.innerHeight - h - 130); // 默认 dock 上方
    // 硬 clamp：任何来源的位置都拉回视口内（不超越屏幕）
    x = Math.max(8, Math.min(window.innerWidth - w - 8, x));
    y = Math.max(8, Math.min(window.innerHeight - h - 8, y));
    bar.style.left = x + 'px';
    bar.style.top = y + 'px';
  }
  let drag = null;
  bar.addEventListener('pointerdown', (e) => {
    if (e.target.closest('#subtitle-x')) return; // X 按钮不触发拖动
    drag = { dx: e.clientX - bar.offsetLeft, dy: e.clientY - bar.offsetTop };
    try { bar.setPointerCapture(e.pointerId); } catch (err) {}
  });
  bar.addEventListener('pointermove', (e) => {
    if (!drag) return;
    const x = Math.max(8, Math.min(window.innerWidth - bar.offsetWidth - 8, e.clientX - drag.dx));
    const y = Math.max(8, Math.min(window.innerHeight - bar.offsetHeight - 8, e.clientY - drag.dy));
    bar.style.left = x + 'px';
    bar.style.top = y + 'px';
  });
  function endDrag() {
    if (!drag) return;
    drag = null;
    subPos = { x: bar.offsetLeft, y: bar.offsetTop };
    try { localStorage.setItem('hc:subpos', JSON.stringify(subPos)); } catch (e) {}
  }
  window.addEventListener('resize', () => { if (!bar.hidden) place(); }); // 窗口/分辨率变化重定位（clamp 回视口，防旧位置悬在屏幕外）
  bar.addEventListener('pointerup', endDrag);
  bar.addEventListener('pointercancel', endDrag);

  // ---- 字幕字号（统一档位 [16,20,24,28,32]，持久 hc:subsize） ----
  const SUB_SIZES = [16, 20, 24, 28, 32];
  let subIdx = SUB_SIZES.indexOf(parseInt(localStorage.getItem('hc:subsize') || '24', 10));
  if (subIdx < 0) subIdx = 2;
  function applySubSize() { textEl.style.fontSize = SUB_SIZES[subIdx] + 'px'; }
  hcBus.on('subsize', (d) => { if (d && typeof d.idx === 'number') subIdx = Math.min(SUB_SIZES.length - 1, Math.max(0, d.idx)); applySubSize(); });
  applySubSize();

  function showBar() { bar.hidden = false; place(); if (tickBtn) tickBtn.classList.add('d-tick-on'); }
  function hideBar() { bar.hidden = true; if (tickBtn) tickBtn.classList.remove('d-tick-on'); }
  if (xBtn) xBtn.addEventListener('click', () => setEnabled(false)); // 字幕条自带 X 关闭
  // 悬停浮现控制（行为对齐桌面挂件）：JS mouseenter 显式控制，不依赖 CSS 变体环境差异
  if (xBtn && bar) {
    bar.addEventListener('mouseenter', () => { xBtn.style.opacity = '1'; });
    bar.addEventListener('mouseleave', () => { xBtn.style.opacity = '0'; });
    xBtn.style.opacity = '0';
  }
  function setEnabled(v) {
    enabled = v;
    try { localStorage.setItem('hc:subtitle', v ? '1' : '0'); } catch (e) {}
    v ? showBar() : hideBar();
  }

  async function loadLyrics(s) {
    songTitle = s.title || '';
    try {
      const kw = encodeURIComponent(((s.title || '') + ' ' + (s.artist || '')).trim());
      const j = await (await fetch('/api/v1/music/lyric/candidates?keyword=' + kw + '&limit=3')).json();
      const cand = j && j.data && j.data[0];
      lines = (cand && cand.lines) || [];
    } catch (e) { lines = []; }
    lastIdx = -1;
    if (textEl) textEl.textContent = lines.length ? lines[0][1] : '';
    if (enabled) place(); // 歌词写入后条宽变化，重定位（clamp 回视口）
  }

  function sync(sec) {
    if (!enabled || !lines.length) return;
    let idx = -1;
    for (let i = 0; i < lines.length; i++) { if (lines[i][0] <= sec) idx = i; else break; }
    if (idx === lastIdx) return;
    lastIdx = idx;
    if (textEl) textEl.textContent = idx >= 0 ? lines[idx][1] : (songTitle || '');
  }

  // 播放状态（含歌曲信息）：无歌隐藏，换歌重新取词
  hcBus.on('state', (d) => {
    const s = d.song;
    if (!s || !s.bvid) { hideBar(); return; }
    if (s.bvid !== lastBvid) { lastBvid = s.bvid; loadLyrics(s); }
    if (enabled) showBar();
  });

  audio.addEventListener('timeupdate', () => {
    sync(audio.currentTime);
    // 卡拉OK横向进度：当前句内推进（句起点→下一句起点）；无歌词行时整曲推进
    if (!progEl) return;
    let pct = 0;
    if (lines.length && enabled) {
      let idx = -1;
      for (let i = 0; i < lines.length; i++) { if (lines[i][0] <= audio.currentTime) idx = i; else break; }
      if (idx >= 0) {
        const t0 = lines[idx][0];
        const t1 = idx + 1 < lines.length ? lines[idx + 1][0] : (audio.duration || t0 + 1);
        pct = t1 > t0 ? Math.min(100, Math.max(0, ((audio.currentTime - t0) / (t1 - t0)) * 100)) : 0;
      }
    } else if (audio.duration) {
      pct = (audio.currentTime / audio.duration) * 100;
    }
    progEl.style.width = pct + '%';
  });

  if (tickBtn) tickBtn.addEventListener('click', () => setEnabled(!enabled));
  hcBus.on('subtitle-toggle', (d) => { if (d) setEnabled(!!d.on); });
  if (enabled) showBar();
})();