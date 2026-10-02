// 最近播放（零件）：队列页顶部横排小卡（localStorage hc:recent，最多 10 显示）。
// 页面渲染型孤岛：afterSwap 重初始化（离开页面重置，再进重绑）。
(function () {
  let inited = false;
  function init() {
    const view = document.getElementById('recent-view');
    if (!view) { inited = false; return; }
    if (inited) return;
    inited = true;

    let recent = [];
    try { recent = JSON.parse(localStorage.getItem('hc:recent') || '[]'); } catch (e) {}

    if (!recent.length) {
      view.innerHTML = '';
      return;
    }

    function esc(x) {
      return String(x ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
    }

    view.innerHTML =
      '<div class="flex items-center justify-between mb-3">' +
      '<h2 class="text-lg font-semibold text-gray-900 dark:text-white">最近播放</h2>' +
      '<span class="text-xs text-gray-500 dark:text-gray-400">共 ' + recent.length + ' 首</span>' +
      '</div>' +
      '<div class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-3">' +
      recent.slice(0, 10).map((r) =>
        '<div class="recent-item group cursor-pointer min-w-0" data-bvid="' + esc(r.bvid) + '" data-title="' + esc(r.title) + '" data-artist="' + esc(r.artist) + '" data-cover="' + esc(r.cover) + '">' +
        '<img src="' + esc(r.cover) + '" alt="" loading="lazy" referrerpolicy="no-referrer" ' +
        'class="w-full aspect-square object-cover rounded-xl shadow-sm group-hover:shadow-md group-hover:scale-[1.02] transition-all duration-200">' +
        '<p class="text-xs text-gray-700 dark:text-gray-300 truncate mt-1.5 group-hover:text-pink-400 transition-colors">' + esc(r.title) + '</p>' +
        '<p class="text-[10px] text-gray-500 dark:text-gray-400 truncate">' + esc(r.artist) + '</p>' +
        '</div>').join('') +
      '</div>';

    view.querySelectorAll('.recent-item').forEach((el) => {
      el.addEventListener('click', () => hcBus.emit('play', {
        bvid: el.dataset.bvid, title: el.dataset.title, artist: el.dataset.artist, cover: el.dataset.cover,
      }));
      el.addEventListener('contextmenu', (e) => { e.stopPropagation(); }); // 右键交给全局菜单（自带 data-ctx 冒泡）
    });
  }
  document.addEventListener('htmx:afterSwap', init);
  init();
})();