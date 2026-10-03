// 队列视图（孤岛/C 态画布）：收 hc:queue → 渲染播放队列（自动上下文 + 插队）
// 数据在 player 进程内，页面只画（能重绘就别同步）
(function () {
  let list = [], idx = -1;
  let favedSet = new Set();

  // 初始恢复：读持久化队列（hc:queue），再靠广播覆盖——避免 player restoreP 广播早于本监听注册而丢队列
  try {
    const qraw = localStorage.getItem('hc:queue');
    if (qraw) {
      const qd = JSON.parse(qraw);
      if (qd && Array.isArray(qd.list) && qd.list.length) {
        list = qd.list.map((q) => ({ bvid: q.bvid, title: q.title, artist: q.artist, cover: q.cover }));
        idx = typeof qd.idx === 'number' && qd.idx >= 0 && qd.idx < list.length ? qd.idx : 0;
        render();
      }
    }
  } catch (e) {}

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  // 收藏状态集合（本地 SQLite 接口，稳定）；失败静默（按钮保持未收藏灰）
  fetch('/api/v1/fav/list').then(function (r) { return r.json(); }).then(function (j) {
    if (j && j.data && Array.isArray(j.data)) {
      favedSet = new Set(j.data.map(function (f) { return f && f.bvid; }).filter(Boolean));
      render();
    }
  }).catch(function () {});

  // 收藏按钮（初始未收藏灰心；已收藏粉心 + 取消确认，对齐右键菜单语义）
  function favBtnHTML(q) {
    return '<button class="q-fav w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0 text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors" data-bvid="' + esc(q.bvid) + '" title="收藏">' +
      '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor"><path d="M12 21s-7-4.6-9.5-9C.8 8.6 2.3 5 5.5 5 8 5 12 8 12 8s4-3 6.5-3c3.2 0 4.7 3.6 3 7-2.5 4.4-9.5 9-9.5 9z"/></svg></button>';
  }

  function updateFavBtn(btn, faved) {
    btn.classList.toggle('text-pink-500', faved);
    btn.classList.toggle('dark:text-pink-400', faved);
    btn.classList.toggle('text-gray-500', !faved);
    btn.classList.toggle('dark:text-gray-400', !faved);
    btn.title = faved ? '取消收藏' : '收藏';
  }

  // 「上次在听」卡片（hc:last：手动点播时记录，刷新/播完永久保留；点卡片直接播）
  function renderLast() {
    const v = document.getElementById('last-view');
    if (!v) return;
    let last = null;
    try {
      const raw = localStorage.getItem('hc:last');
      if (raw) last = JSON.parse(raw);
    } catch (e) {}
    if (!last || !last.bvid) { v.innerHTML = ''; return; }
    v.innerHTML =
      '<div class="flex items-center gap-4 p-4 bg-gradient-to-r from-pink-500/5 to-violet-500/5 border border-pink-500/20 rounded-2xl">' +
      '<img src="' + esc(last.cover) + '" alt="" loading="lazy" referrerpolicy="no-referrer" class="w-14 h-14 object-cover rounded-xl shadow-sm flex-shrink-0">' +
      '<div class="flex-1 min-w-0">' +
      '<p class="text-xs font-medium text-pink-400 mb-0.5">上次在听</p>' +
      '<p class="text-sm font-semibold text-gray-900 dark:text-white truncate">' + esc(last.title) + '</p>' +
      '<p class="text-xs text-gray-500 dark:text-gray-400 truncate">' + esc(last.artist || '未知作者') + (typeof last.at === 'number' && last.at > 1 ? ' · 上次听到 ' + fmtTime(last.at) : '') + '</p>' +
      '</div>' +
      '<button class="q-last-play w-11 h-11 rounded-full flex-shrink-0 flex items-center justify-center bg-gradient-to-r from-pink-500 to-violet-500 text-white shadow-md hover:brightness-105 transition-all" title="继续播放上次在听的歌">' +
      '<svg class="w-5 h-5 ml-0.5" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg></button>' +
      '</div>';
    v.querySelector('.q-last-play').addEventListener('click', () => {
      hcBus.emit('play', { bvid: last.bvid, title: last.title, artist: last.artist, cover: last.cover, duration: last.duration });
    });
  }

  function fmtTime(sec) {
    sec = Math.floor(sec || 0);
    const m = Math.floor(sec / 60), s = sec % 60;
    return m + ':' + (s < 10 ? '0' : '') + s;
  }

  function render() {
    const view = document.getElementById('queue-view');
    if (!view) return;
    renderLast();
    if (!list.length) {
      view.innerHTML =
        '<div class="text-center py-24 text-gray-500 dark:text-gray-400">' +
        '<svg class="w-16 h-16 mx-auto mb-4 text-gray-300 dark:text-gray-600" viewBox="0 0 24 24" fill="currentColor"><path d="M9 18V6H7v12h2zm8-12v12h2V6h-2z"/></svg>' +
        '<p class="text-lg font-medium">队列为空</p>' +
        '<p class="text-sm mt-1">随便点一首歌，队列会自动出现在这里</p></div>';
      return;
    }
    let html =
      '<div class="flex items-center justify-between mb-4">' +
      '<h2 class="text-lg font-semibold text-gray-900 dark:text-white">播放队列</h2>' +
      '<div class="flex items-center space-x-3">' +
      '<span class="text-sm text-gray-500 dark:text-gray-400">' + list.length + ' 首</span>' +
      '<button class="queue-clear text-sm text-gray-500 dark:text-gray-400 hover:text-red-400 transition-colors">清空</button>' +
      '</div></div>' +
      '<div class="grid gap-3">';
    list.forEach((q, i) => {
      const active = i === idx;
      html +=
        '<div class="group flex items-start space-x-4 p-4 bg-white dark:bg-gray-800 rounded-2xl shadow-sm transition-all duration-200 ' +
        (active ? 'is-playing border border-pink-500/50' : 'border border-gray-100 dark:border-gray-700') + '" data-ctx-bvid="' + esc(q.bvid) + '" data-ctx-title="' + esc(q.title) + '" data-ctx-artist="' + esc(q.artist) + '" data-ctx-cover="' + esc(q.cover) + '">' +
        '<div class="relative flex-shrink-0">' +
        '<img src="' + esc(q.cover) + '" alt="" loading="lazy" referrerpolicy="no-referrer" class="w-20 h-14 object-cover rounded-xl shadow-sm">' +
        '<button class="hx-play absolute inset-0 bg-black/30 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center cursor-pointer" data-bvid="' + esc(q.bvid) + '">' +
        '<svg class="w-6 h-6 text-white" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg></button></div>' +
        '<div class="flex-1 min-w-0">' +
        '<div class="flex items-center gap-1">' +
        '<p class="q-title flex-1 min-w-0 text-base font-semibold text-gray-900 dark:text-white truncate cursor-pointer hover:text-pink-400 transition-colors" data-bvid="' + esc(q.bvid) + '" title="点击展开/收起完整歌名">' + esc(q.title) + '</p>' +
        '<button class="q-bili w-7 h-7 rounded-full flex-shrink-0 flex items-center justify-center text-gray-400 dark:text-gray-500 hover:text-pink-400 transition-colors" data-bvid="' + esc(q.bvid) + '" title="打开 B 站原视频">' +
        '<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M7 17 17 7"/><path d="M8 7h9v9"/></svg></button>' +
        '</div>' +
        '<p class="text-sm text-gray-500 dark:text-gray-400 truncate">' + esc(q.artist) + '</p></div>' +
        (active ? '<span class="flex-shrink-0 text-pink-400 text-sm py-2">正在播放</span>' : '') +
        favBtnHTML(q) +
        '<button class="q-next w-9 h-9 rounded-full flex-shrink-0 flex items-center justify-center text-gray-500 dark:text-gray-400 hover:text-pink-400 transition-colors" data-bvid="' + esc(q.bvid) + '" title="下一首播放（插队）">' +
        '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6v12l8-6z"/><path d="M12 6v12l8-6z" opacity="0.4"/></svg></button>' +
        '<button class="q-del w-9 h-9 rounded-full flex-shrink-0 flex items-center justify-center text-gray-500 dark:text-gray-400 hover:text-red-400 transition-colors" data-bvid="' + esc(q.bvid) + '" title="从队列移除">' +
        '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6h12v14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2z"/><path d="M4 6h16"/></svg></button>' +
        '</div>';
    });
    html += '</div>';
    view.innerHTML = html;
    // 行内播放：只广播（孤岛不共享）
    view.querySelectorAll('.hx-play').forEach((btn) => {
      btn.addEventListener('click', () => {
        const s = list.find((x) => x.bvid === btn.dataset.bvid);
        if (s) hcBus.emit('play', s);
      });
    });
    view.querySelectorAll('.q-del').forEach((btn) => {
      btn.addEventListener('click', () => hcBus.emit('queue-remove', { bvid: btn.dataset.bvid }));
    });
    // 单击标题展开/收起完整歌名（长名不再截断看不到）
    view.querySelectorAll('.q-title').forEach((t) => {
      t.addEventListener('click', () => {
        t.classList.toggle('truncate');
        t.classList.toggle('whitespace-normal');
        t.classList.toggle('break-words');
      });
    });
    // 打开 B 站原视频：移到行尾外链小图标（标题点击改展开歌名了）
    view.querySelectorAll('.q-bili').forEach((btn) => {
      btn.addEventListener('click', () => hcBus.emit('open-bili', { bvid: btn.dataset.bvid }));
    });
    view.querySelectorAll('.q-next').forEach((btn) => {
      btn.addEventListener('click', () => {
        const s = list.find((x) => x.bvid === btn.dataset.bvid);
        if (s) hcBus.emit('play-next', s);
      });
    });
    // 收藏/取消收藏：先查状态（favedSet）→ 已收藏要确认（对齐右键菜单语义）
    view.querySelectorAll('.q-fav').forEach((btn) => {
      updateFavBtn(btn, favedSet.has(btn.dataset.bvid));
      btn.addEventListener('click', async () => {
        const q = list.find((x) => x.bvid === btn.dataset.bvid);
        if (!q) return;
        const faved = favedSet.has(q.bvid);
        if (faved && !window.confirm('确定取消收藏？')) return;
        const fd = new FormData();
        fd.append('bvid', q.bvid);
        fd.append('title', q.title || '');
        fd.append('artist', q.artist || '');
        fd.append('cover', q.cover || '');
        try {
          const r = await fetch('/hx/fav/toggle', { method: 'POST', body: fd });
          if (!r.ok) return;
          if (faved) favedSet.delete(q.bvid); else favedSet.add(q.bvid);
          updateFavBtn(btn, !faved);
          hcBus.emit('toast', { msg: !faved ? '已收藏 ♥' : '已取消收藏' });
        } catch (e) { /* 网络失败静默 */ }
      });
    });
    const clearBtn = view.querySelector('.queue-clear');
    if (clearBtn) clearBtn.addEventListener('click', () => hcBus.emit('queue-clear'));
  }

  hcBus.on('queue', (d) => { list = d.list || []; idx = d.idx; render(); });
  document.addEventListener('htmx:afterSwap', render);
})();