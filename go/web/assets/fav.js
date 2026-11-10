// 收藏统一模块（hcFav）：所有收藏按钮共用同一逻辑（toggle + 状态集合 + 广播）。
// 后端权威：POST /hx/fav/toggle（query 参数，兼容安卓壳 asset 桥丢 body）→ 返回 fav_btn.html 片段替换按钮。
// 前端只管：发请求 + 用返回片段替换自身 + 同步全局集合 + 广播 fav-changed（各处按钮刷新状态）。
// 用法：按钮加 class="hc-fav" + data-bvid/title/artist/cover/duration；hcFav.bind(container) 或事件委托。
(function () {
  'use strict';
  let faved = new Set(); // bvid -> true（全局收藏集合）

  function load() {
    return fetch('/api/v1/fav/list').then((r) => r.json()).then((j) => {
      faved = new Set(((j && j.data) || []).map((f) => f && f.bvid).filter(Boolean));
      document.dispatchEvent(new CustomEvent('fav-changed'));
      return faved;
    }).catch(() => faved);
  }

  function isFaved(bvid) { return faved.has(bvid); }

  // 从按钮元素取歌曲数据（data-* 属性）
  function songOf(btn) {
    return {
      bvid: btn.dataset.bvid || '',
      title: btn.dataset.title || '',
      artist: btn.dataset.artist || '',
      cover: btn.dataset.cover || '',
      duration: btn.dataset.duration || 0,
    };
  }

  // 更新按钮视觉状态（不请求，仅从集合渲染）
  function syncBtn(btn) {
    if (!btn || !btn.dataset.bvid) return;
    const on = faved.has(btn.dataset.bvid);
    btn.classList.toggle('text-pink-500', on);
    btn.classList.toggle('dark:text-pink-400', on);
    btn.classList.toggle('text-gray-500', !on);
    btn.classList.toggle('dark:text-gray-400', !on);
    btn.title = on ? '取消收藏' : '收藏';
    btn.innerHTML = on
      ? '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor"><path d="M12 21s-7-4.6-9.5-9C.8 8.6 2.3 5 5.5 5 8 5 12 8 12 8s4-3 6.5-3c3.2 0 4.7 3.6 3 7-2.5 4.4-9.5 9-9.5 9z"/></svg>'
      : '<svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 21s-7-4.6-9.5-9C.8 8.6 2.3 5 5.5 5 8 5 12 8 12 8s4-3 6.5-3c3.2 0 4.7 3.6 3 7-2.5 4.4-9.5 9-9.5 9z"/></svg>';
  }

  // 切换收藏（统一逻辑）：已收藏需确认 → POST /hx/fav/toggle → 更新集合 + 广播
  function toggle(btn) {
    const s = songOf(btn);
    if (!s.bvid) return Promise.resolve();
    const wasFaved = faved.has(s.bvid);
    if (wasFaved && !window.confirm('确定取消收藏？')) return Promise.resolve();
    const qs = new URLSearchParams({
      bvid: s.bvid, title: s.title || '', artist: s.artist || '', cover: s.cover || '', duration: String(s.duration || 0),
    });
    return fetch('/hx/fav/toggle?' + qs.toString(), { method: 'POST' })
      .then((r) => {
        if (!r.ok) throw new Error('bad');
        if (wasFaved) faved.delete(s.bvid); else faved.add(s.bvid);
        document.dispatchEvent(new CustomEvent('fav-changed', { detail: { bvid: s.bvid, faved: !wasFaved } }));
        window.hcBus.emit('toast', { msg: wasFaved ? '已取消收藏' : '已收藏 ♥' });
        syncBtn(btn);
      })
      .catch(() => window.hcBus.emit('toast', { msg: '收藏失败' }));
  }

  // 绑定容器内所有 .hc-fav 按钮（事件委托，重绘安全）
  function bind(container) {
    const root = container || document;
    root.addEventListener('click', (e) => {
      const btn = e.target.closest('.hc-fav');
      if (btn && btn.dataset.bvid) { e.preventDefault(); e.stopPropagation(); toggle(btn); }
    });
    root.addEventListener('fav-changed', () => root.querySelectorAll('.hc-fav').forEach(syncBtn));
  }
  document.addEventListener('fav-changed', () => document.querySelectorAll('.hc-fav').forEach(syncBtn));

  window.hcFav = { load, isFaved, toggle, syncBtn, bind, faved: () => faved };
  load();
  bind();
})();
