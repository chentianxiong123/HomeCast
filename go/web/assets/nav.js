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

  // 移动端侧边折叠导航：浮球 toggle + 点面板外关闭 + 选 tab 后收起
  const navBtn = document.getElementById('side-nav-btn');
  const navPanel = document.getElementById('side-nav-panel');
  if (navBtn && navPanel) {
    navBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      navPanel.classList.toggle('hidden');
    });
    document.addEventListener('click', (e) => {
      if (!navPanel.classList.contains('hidden') && !navPanel.contains(e.target) && e.target !== navBtn) {
        navPanel.classList.add('hidden');
      }
    });
    navPanel.addEventListener('click', (e) => {
      if (e.target.closest('a')) navPanel.classList.add('hidden');
    });
  }
})();
