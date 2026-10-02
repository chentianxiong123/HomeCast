// 导航高亮（零件）：由 URL 推导（可推导不存），htmx swap / 前进后退后同步
(function () {
  function sync() {
    const p = location.pathname;
    document.querySelectorAll('.nav-tab').forEach((a) => {
      const href = a.getAttribute('href');
      const active = p === href || (p === '/' && href === '/hx/search');
      a.classList.toggle('nav-active', active);
    });
  }
  document.addEventListener('htmx:afterSwap', sync);
  window.addEventListener('popstate', sync);
  sync();
})();

// 封面/歌词入口 → 导航到歌词页（htmx.ajax 只换 #main，播放器不中断）
// 注：htmx2 的 htmx.ajax options.pushUrl 不生效，手动 pushState + popstate 派发同步 nav 高亮
hcBus.on('goto-lyric', () => {
  if (window.htmx) {
    htmx.ajax('GET', '/hx/lyric', { target: '#main', select: '#main', swap: 'innerHTML' });
    history.pushState(null, '', '/hx/lyric');
    window.dispatchEvent(new PopStateEvent('popstate'));
  } else {
    location.href = '/hx/lyric';
  }
});
