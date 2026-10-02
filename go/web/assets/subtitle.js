// 网页字幕条（孤岛）：当前播放歌词的常驻字幕（多态呈现——桌面版由 GTK 挂件承担，
// 本环境为网页/浏览器时显示网页内字幕条；壳环境 window.hcEnv==='desktop' 默认关）
(function () {
  const bar = document.getElementById('subtitle-bar');
  const tickBtn = document.getElementById('d-tick');
  const audio = document.getElementById('audio');
  if (!bar || !audio) return;

  const envDesktop = window.hcEnv === 'desktop';
  let enabled = envDesktop ? false : localStorage.getItem('hc:subtitle') !== '0';
  let lines = [];   // [[sec, line], ...]
  let lastIdx = -1;
  let lastBvid = '';
  let songTitle = '';

  function showBar() { bar.hidden = false; if (tickBtn) tickBtn.classList.add('text-pink-400'); }
  function hideBar() { bar.hidden = true; if (tickBtn) tickBtn.classList.remove('text-pink-400'); }
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
    bar.textContent = lines.length ? lines[0][1] : '';
  }

  function sync(sec) {
    if (!enabled || !lines.length) return;
    let idx = -1;
    for (let i = 0; i < lines.length; i++) { if (lines[i][0] <= sec) idx = i; else break; }
    if (idx === lastIdx) return;
    lastIdx = idx;
    bar.textContent = idx >= 0 ? lines[idx][1] : (songTitle || '');
  }

  // 播放状态（含歌曲信息）：无歌隐藏，换歌重新取词
  hcBus.on('state', (d) => {
    const s = d.song;
    if (!s || !s.bvid) { hideBar(); return; }
    if (s.bvid !== lastBvid) { lastBvid = s.bvid; loadLyrics(s); }
    if (enabled) showBar();
  });

  audio.addEventListener('timeupdate', () => sync(audio.currentTime));

  if (tickBtn) tickBtn.addEventListener('click', () => setEnabled(!enabled));
  if (enabled) showBar();
})();