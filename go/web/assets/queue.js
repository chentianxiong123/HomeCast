// 队列视图（孤岛/C 态画布）：收 hc:queue → 渲染播放队列（自动上下文 + 插队）
// 数据在 player 进程内，页面只画（能重绘就别同步）
(function () {
  let list = [], idx = -1;

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  function render() {
    const view = document.getElementById('queue-view');
    if (!view) return;
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
      '<span class="text-sm text-gray-500 dark:text-gray-400">' + list.length + ' 首</span></div>' +
      '<div class="grid gap-3">';
    list.forEach((q, i) => {
      const active = i === idx;
      html +=
        '<div class="group flex items-center space-x-4 p-4 bg-white dark:bg-gray-800 rounded-2xl shadow-sm transition-all duration-200 ' +
        (active ? 'is-playing border border-pink-500/50' : 'border border-gray-100 dark:border-gray-700') + '">' +
        '<div class="relative flex-shrink-0">' +
        '<img src="' + esc(q.cover) + '" alt="" loading="lazy" referrerpolicy="no-referrer" class="w-20 h-14 object-cover rounded-xl shadow-sm">' +
        '<button class="hx-play absolute inset-0 bg-black/30 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center cursor-pointer" data-bvid="' + esc(q.bvid) + '">' +
        '<svg class="w-6 h-6 text-white" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg></button></div>' +
        '<div class="flex-1 min-w-0">' +
        '<p class="text-base font-semibold text-gray-900 dark:text-white truncate">' + esc(q.title) + '</p>' +
        '<p class="text-sm text-gray-500 dark:text-gray-400 truncate">' + esc(q.artist) + '</p></div>' +
        (active ? '<span class="flex-shrink-0 text-pink-400 text-sm">正在播放</span>' : '') +
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
  }

  hcBus.on('queue', (d) => { list = d.list || []; idx = d.idx; render(); });
  document.addEventListener('htmx:afterSwap', render);
})();