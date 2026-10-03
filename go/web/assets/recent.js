// 最近播放（零件）：队列页顶部横排小卡（localStorage hc:recent，最多 10 显示）+ /hx/recent 全页面（全部 30 首）。
// 页面渲染型孤岛：afterSwap 重初始化（离开页面重置，再进重绑）。
(function () {
  let inited = false;
  function esc(x) {
    return String(x ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }
  function readRecent() {
    try { return JSON.parse(localStorage.getItem('hc:recent') || '[]'); } catch (e) { return []; }
  }
  function bindGrid(container, items) {
    container.querySelectorAll('.recent-item').forEach((el) => {
      el.addEventListener('click', () => hcBus.emit('play', {
        bvid: el.dataset.bvid, title: el.dataset.title, artist: el.dataset.artist, cover: el.dataset.cover,
      }));
      el.addEventListener('contextmenu', (e) => { e.stopPropagation(); }); // 右键交给全局菜单（自带 data-ctx 冒泡）
    });
    // 清空最近播放（确认后移除 hc:recent 并重绘当前容器为空态/隐藏）
    const clr = container.querySelector('.recent-clear-all');
    if (clr) clr.addEventListener('click', () => {
      if (!window.confirm('确定清空全部最近播放？')) return;
      try { localStorage.removeItem('hc:recent'); } catch (e) {}
      hcBus.emit('toast', { msg: '最近播放已清空' });
      if (container.id === 'recent-all') renderAll();
      else { container.innerHTML = ''; }
    });
    // hx 属性（如「查看全部」链接）是 JS 渲染出来的，需 htmx.process 才生效
    if (window.htmx && container.querySelector('[hx-get]')) htmx.process(container);
  }
  function renderAll() {
    const view = document.getElementById('recent-all');
    if (!view) return false;
    const recent = readRecent();
    if (!recent.length) {
      view.innerHTML =
        '<div class="text-center py-24 text-gray-500 dark:text-gray-400">' +
        '<svg class="w-16 h-16 mx-auto mb-4 text-gray-300 dark:text-gray-600" viewBox="0 0 24 24" fill="currentColor"><path d="M12 3v10.55A4 4 0 1 0 14 17V7h4V3z"/></svg>' +
        '<p class="text-lg font-medium">还没有播放记录</p>' +
        '<p class="text-sm mt-1">听过的歌会出现在这里，最多保留 30 首</p>' +
        '<a href="/hx/search" hx-get="/hx/search" hx-select="#main" hx-target="#main" hx-swap="innerHTML" hx-push-url="true"' +
        ' class="inline-block mt-6 px-8 py-2.5 bg-gradient-to-r from-pink-500 to-violet-500 text-white font-medium rounded-full shadow-md hover:brightness-105 transition-all">去听歌</a>' +
        '</div>';
      bindGrid(view, []);
      return true;
    }
    view.innerHTML =
      '<div class="flex items-center justify-between mb-4">' +
      '<h1 class="text-xl font-bold text-gray-900 dark:text-white">最近播放</h1>' +
      '<div class="flex items-center gap-3">' +
      '<span class="text-sm text-gray-500 dark:text-gray-400">共 ' + recent.length + ' 首</span>' +
      '<button class="recent-clear-all text-sm text-gray-500 dark:text-gray-400 hover:text-red-400 transition-colors">清空</button>' +
      '</div></div>' +
      '<div class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-3">' +
      recent.map((r) =>
        '<div class="recent-item group cursor-pointer min-w-0" data-bvid="' + esc(r.bvid) + '" data-title="' + esc(r.title) + '" data-artist="' + esc(r.artist) + '" data-cover="' + esc(r.cover) + '">' +
        '<img src="' + esc(r.cover) + '" alt="" loading="lazy" referrerpolicy="no-referrer" ' +
        'class="w-full aspect-square object-cover rounded-xl shadow-sm group-hover:shadow-md group-hover:scale-[1.02] transition-all duration-200">' +
        '<p class="text-xs text-gray-700 dark:text-gray-300 truncate mt-1.5 group-hover:text-pink-400 transition-colors">' + esc(r.title) + '</p>' +
        '<p class="text-[10px] text-gray-500 dark:text-gray-400 truncate">' + esc(r.artist) + '</p>' +
        '</div>').join('') +
      '</div>';
    bindGrid(view, recent);
    return true;
  }
  function init() {
    // 独立全页面优先
    if (renderAll()) { inited = false; return; }
    const view = document.getElementById('recent-view');
    if (!view) { inited = false; return; }
    if (inited) return;
    inited = true;

    const recent = readRecent();

    if (!recent.length) {
      view.innerHTML = '';
      return;
    }

    view.innerHTML =
      '<div class="flex items-center justify-between mb-3">' +
      '<div class="flex items-baseline gap-3">' +
      '<h2 class="text-lg font-semibold text-gray-900 dark:text-white">最近播放</h2>' +
      '<span class="text-xs text-gray-500 dark:text-gray-400">共 ' + recent.length + ' 首</span>' +
      '</div>' +
      '<div class="flex items-center gap-3">' +
      '<button class="recent-clear-all text-xs text-gray-500 dark:text-gray-400 hover:text-red-400 transition-colors">清空</button>' +
      '<a href="/hx/recent" hx-get="/hx/recent" hx-select="#main" hx-target="#main" hx-swap="innerHTML" hx-push-url="true"' +
      ' class="text-xs font-medium text-pink-500 hover:text-pink-600 transition-colors">查看全部 →</a>' +
      '</div></div>' +
      '<div class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-3">' +
      recent.slice(0, 10).map((r) =>
        '<div class="recent-item group cursor-pointer min-w-0" data-bvid="' + esc(r.bvid) + '" data-title="' + esc(r.title) + '" data-artist="' + esc(r.artist) + '" data-cover="' + esc(r.cover) + '">' +
        '<img src="' + esc(r.cover) + '" alt="" loading="lazy" referrerpolicy="no-referrer" ' +
        'class="w-full aspect-square object-cover rounded-xl shadow-sm group-hover:shadow-md group-hover:scale-[1.02] transition-all duration-200">' +
        '<p class="text-xs text-gray-700 dark:text-gray-300 truncate mt-1.5 group-hover:text-pink-400 transition-colors">' + esc(r.title) + '</p>' +
        '<p class="text-[10px] text-gray-500 dark:text-gray-400 truncate">' + esc(r.artist) + '</p>' +
        '</div>').join('') +
      '</div>';

    bindGrid(view, recent);
  }
  // 播放/切歌后重渲染（最近播放顺序变了）：不依赖 inited，强制重绘当前容器
  hcBus.on('nowplaying', () => {
    if (document.getElementById('recent-all')) renderAll();
    else if (document.getElementById('recent-view')) { inited = false; init(); }
  });
  document.addEventListener('htmx:afterSwap', init);
  init();
})();