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
