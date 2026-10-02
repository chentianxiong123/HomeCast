// 列表当前播放高亮（零件）：收 hc:nowplaying → 卡片 ring 高亮，可重绘不存状态
(function () {
  let last = null;
  function highlight(bvid) {
    document.querySelectorAll('.is-playing').forEach((c) => c.classList.remove('is-playing'));
    if (!bvid) return;
    document.querySelectorAll('.hx-play[data-bvid="' + bvid + '"]').forEach((btn) => {
      const card = btn.closest('.group');
      if (card) card.classList.add('is-playing');
    });
  }
  hcBus.on('nowplaying', (d) => { last = d.bvid; highlight(d.bvid); });
  document.addEventListener('htmx:afterSwap', () => highlight(last));
  highlight();
})();
