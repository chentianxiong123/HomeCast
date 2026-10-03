// 右键菜单（零件）：列表行/卡片右键 → 通用操作菜单（YesPlayMusic ContextMenu 同款）。
// 数据来自元素 data-ctx-* 属性（bvid/title/artist/cover/duration）；事件全走 hcBus 复用现有孤岛。
(function () {
  let menu = null;

  function closeMenu() {
    if (menu) { menu.remove(); menu = null; }
  }

  function buildItem(label, icon, fn) {
    const b = document.createElement('button');
    b.className = 'w-full flex items-center space-x-2 px-4 py-2 text-left text-gray-200 hover:bg-white/10 transition-colors';
    b.innerHTML = icon + '<span class="flex-1">' + label + '</span>';
    b.addEventListener('click', () => { closeMenu(); fn(); });
    return b;
  }

  function openMenu(x, y, item) {
    closeMenu();
    const m = document.createElement('div');
    m.className = 'ctx-menu fixed z-[70] min-w-[190px] rounded-xl bg-gray-800 border border-gray-700 shadow-2xl py-1.5 text-sm overflow-hidden';
    const ic = '<svg class="w-4 h-4 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="';
    m.appendChild(buildItem('播放', ic + 'M8 5v14l11-7z"/></svg>', () => hcBus.emit('play', item)));
    m.appendChild(buildItem('下一首播放', ic + 'M4 6v12l8-6z"/><path d="M12 6v12l8-6z" opacity="0.4"/></svg>', () => hcBus.emit('play-next', item)));
    m.appendChild(buildItem('收藏 / 取消收藏', ic + 'M12 21s-7-4.6-9.5-9C.8 8.6 2.3 5 5.5 5 8 5 12 8 12 8s4-3 6.5-3c3.2 0 4.7 3.6 3 7-2.5 4.4-9.5 9-9.5 9z"/></svg>', () => toggleFav(item)));
    m.appendChild(buildItem('复制链接', ic + 'M8 8h10v10H8z"/><path d="M6 14H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v2"/></svg>', () => copyLink(item)));
    document.body.appendChild(m);

    const r = m.getBoundingClientRect();
    if (x + r.width > window.innerWidth) x = Math.max(4, window.innerWidth - r.width - 8);
    if (y + r.height > window.innerHeight) y = Math.max(4, window.innerHeight - r.height - 8);
    m.style.left = x + 'px';
    m.style.top = y + 'px';
    menu = m;
  }

  function getItem(el) {
    return {
      bvid: el.dataset.ctxBvid,
      title: el.dataset.ctxTitle || '',
      artist: el.dataset.ctxArtist || '',
      cover: el.dataset.ctxCover || '',
      duration: parseInt(el.dataset.ctxDuration || '0', 10),
    };
  }

  // 触屏长按 = 右键（手机没有右键）：按下 500ms 不动 → 出菜单；移动/松开/取消则作废；
  // 长按后 400ms 内吞 click（防长按结束误触播放）
  let pressTimer = null, pressXY = null, pressTarget = null, suppressClickUntil = 0;
  document.addEventListener('pointerdown', (e) => {
    if (menu && !menu.contains(e.target)) closeMenu();
    const el = e.target.closest('[data-ctx-bvid]');
    if (!el) return;
    pressTarget = el;
    pressXY = { x: e.clientX, y: e.clientY };
    clearTimeout(pressTimer);
    pressTimer = setTimeout(() => {
      if (!pressTarget || !pressXY) return;
      suppressClickUntil = Date.now() + 400;
      const pos = pressXY;
      pressTarget = null;
      openMenu(pos.x, pos.y, getItem(el));
    }, 500);
  });
  document.addEventListener('pointermove', (e) => {
    if (!pressTarget || !pressXY) return;
    if (Math.abs(e.clientX - pressXY.x) > 10 || Math.abs(e.clientY - pressXY.y) > 10) {
      clearTimeout(pressTimer);
      pressTarget = null;
    }
  });
  document.addEventListener('pointerup', () => { clearTimeout(pressTimer); pressTarget = null; });
  document.addEventListener('pointercancel', () => { clearTimeout(pressTimer); pressTarget = null; });
  document.addEventListener('click', (e) => {
    if (Date.now() < suppressClickUntil) { e.preventDefault(); e.stopPropagation(); }
  }, true);

  function toggleFav(item) {
    // 取消收藏需确认：先查当前是否已收藏
    fetch('/api/v1/fav/list')
      .then((r) => r.json())
      .then((j) => {
        const faved = ((j && j.data) || []).some((f) => f && f.bvid === item.bvid);
        if (faved && !window.confirm('确定取消收藏？')) return;
        const fd = new FormData();
        fd.set('bvid', item.bvid);
        fd.set('title', item.title || '');
        fd.set('artist', item.artist || '');
        fd.set('cover', item.cover || '');
        fd.set('duration', String(item.duration || 0));
        return fetch('/hx/fav/toggle', { method: 'POST', body: fd });
      })
      .then(() => hcBus.emit('toast', { msg: '收藏已切换 ♥' }))
      .catch(() => hcBus.emit('toast', { msg: '收藏失败' }));
  }

  function copyLink(item) {
    try {
      navigator.clipboard.writeText('https://www.bilibili.com/video/' + item.bvid);
      hcBus.emit('toast', { msg: '链接已复制' });
    } catch (e) {
      hcBus.emit('toast', { msg: '复制失败' });
    }
  }

  document.addEventListener('contextmenu', (e) => {
    const el = e.target.closest('[data-ctx-bvid]');
    if (!el) { closeMenu(); return; }
    e.preventDefault();
    openMenu(e.clientX, e.clientY, getItem(el));
  });
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeMenu(); });
  window.addEventListener('blur', closeMenu);
})();