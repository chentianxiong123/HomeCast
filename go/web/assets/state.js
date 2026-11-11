// 统一状态源前端桥（阶段2）：订阅后端 /api/v1/state/stream（SSE 单流），
// 把业务状态写入 Alpine.store('playback')，并桥接成 hcBus 事件给现有孤岛
// （nowplaying/state/queue/now），使 lyric/subtitle/recent/list-highlight 不改。
// 前端不再持有业务状态——只渲染 + 发命令（player.js / state.js 负责）。
(function () {
  'use strict';
  if (!window.Alpine) return; // 未引入 Alpine 则退化为纯 hcBus 桥（不依赖 store）

  // 默认播放状态（Alpine store 初值）
  Alpine.store('playback', {
    source: 'web',
    bvid: '', title: '', artist: '', cover: '',
    playing: false,
    position: 0, duration: 0,
    play_mode: 'order',
    queue: [], qidx: -1,
    spk_did: '',
  });

  // 把后端快照写入 Alpine store
  function applyState(s) {
    if (!s) return;
    Alpine.store('playback', {
      source: s.source || 'web',
      bvid: s.bvid || '', title: s.title || '', artist: s.artist || '', cover: s.cover || '',
      playing: !!s.playing,
      position: s.position || 0, duration: s.duration || 0,
      play_mode: s.play_mode || 'order',
      queue: Array.isArray(s.queue) ? s.queue : [],
      qidx: typeof s.qidx === 'number' ? s.qidx : -1,
      spk_did: s.spk_did || '',
    });
    // 桥接 hcBus（现有孤岛不改）：nowplaying / state / queue
    const song = s.bvid ? { bvid: s.bvid, title: s.title, artist: s.artist, cover: s.cover, duration: s.duration } : null;
    try {
      if (window.hcBus) {
        window.hcBus.emit('nowplaying', { bvid: s.bvid || '' });
        if (song) window.hcBus.emit('now', song);
        window.hcBus.emit('state', { state: s.playing ? 'playing' : 'paused', song });
        window.hcBus.emit('queue', { list: s.queue || [], idx: s.qidx });
      }
    } catch (e) {}
  }

  // 进度更新（高频 progress 事件：只更新 position/playing，不重建队列）
  function applyProgress(p) {
    if (!p) return;
    const st = Alpine.store('playback');
    st.position = p.position || 0;
    if (p.duration > 0) st.duration = p.duration;
    st.playing = !!p.playing;
    if (p.source) st.source = p.source;
    if (p.spk_did !== undefined) st.spk_did = p.spk_did;
    // 桥接低频 state（进度变化也要面板刷新）——节流到 ~250ms
    try {
      if (window.hcBus) window.hcBus.emit('state', { state: st.playing ? 'playing' : 'paused', song: st.bvid ? { bvid: st.bvid, title: st.title, artist: st.artist, cover: st.cover, duration: st.duration } : null });
    } catch (e) {}
  }

  try {
    const es = new EventSource('/api/v1/state/stream');
    es.addEventListener('state', (ev) => {
      try {
        const s = JSON.parse(ev.data);
        applyState(s);
        migrateLegacy(s); // 一次性：旧 localStorage 队列 → 后端（仅后端空队列时）
      } catch (e) {}
    });
    es.addEventListener('progress', (ev) => {
      try { applyProgress(JSON.parse(ev.data)); } catch (e) {}
    });
    es.onerror = () => {}; // 自动重连
  } catch (e) {}

  // 一次性迁移：旧前端 localStorage（hc:queue / hc:song）→ 后端队列（阶段4）。
  // 仅当后端队列为空 且 localStorage 有旧数据时导入一次；导入后清除旧键，避免重复。
  let migrated = false;
  function migrateLegacy(s) {
    if (migrated) return;
    migrated = true;
    try {
      const qraw = localStorage.getItem('hc:queue');
      const qd = qraw ? JSON.parse(qraw) : null;
      const oldQueue = qd && Array.isArray(qd.list) && qd.list.length ? qd.list : [];
      // 后端已有队列：不覆盖，仅清理旧键
      if (s && s.queue && s.queue.length) {
        localStorage.removeItem('hc:queue');
        return;
      }
      // 后端空：导入旧队列（若 hc:queue 有，用它；否则单曲 hc:song）
      // 用独立 fetch（EventSource 首事件可能早于 window.hcState 定义，不依赖它）
      function postCmd(op, body) {
        fetch('/api/v1/state/command', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(Object.assign({ op: op }, body || {})),
        }).catch(function () {});
      }
      if (oldQueue.length) {
        postCmd('queue', { queue: oldQueue.map(function (x) { return { bvid: x.bvid, title: x.title, artist: x.artist, cover: x.cover, duration: x.duration }; }), qidx: typeof qd.idx === 'number' ? qd.idx : 0 });
      } else {
        const sraw = localStorage.getItem('hc:song');
        if (sraw) {
          const sc = JSON.parse(sraw);
          if (sc && sc.bvid) postCmd('play', { bvid: sc.bvid, title: sc.title, artist: sc.artist, cover: sc.cover });
        }
      }
      localStorage.removeItem('hc:queue');
      localStorage.removeItem('hc:song');
    } catch (e) {}
  }

  // 命令入口：供 player.js / 其他孤岛发命令到后端（统一走后端）
  window.hcState = {
    // 发命令（后端更新状态 → SSE 广播 → 前端渲染）
    cmd(op, extra) {
      const body = Object.assign({ op }, extra || {});
      return fetch('/api/v1/state/command', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }).then((r) => r.json()).catch(() => {});
    },
    // web 位置上报（高频）
    progress(position, duration, playing) {
      fetch('/api/v1/state/progress', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        // 带 bvid：后端只接受匹配当前播放歌的进度，防多客户端旧歌进度覆盖新歌
        body: JSON.stringify({ position, duration, playing, bvid: this.get().bvid || '' }),
      }).catch(() => {});
    },
    // 当前播放快照（读 Alpine store）
    get() { return Alpine.store('playback'); },
  };
})();
