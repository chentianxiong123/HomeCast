// 轻提示（零件）：收 hc:toast → 顶部浮条 2s 消失（可重绘不存状态）
(function () {
  let t = null;
  hcBus.on('toast', (d) => {
    if (t) t.remove();
    t = document.createElement('div');
    t.className =
      'fixed top-16 left-1/2 -translate-x-1/2 z-[70] px-4 py-2 rounded-full bg-gray-800/95 border border-gray-700 text-sm text-gray-100 shadow-xl';
    t.textContent = d && d.msg ? d.msg : '';
    document.body.appendChild(t);
    setTimeout(() => { t.remove(); t = null; }, 2000);
  });
})();
// 打开 B 站原页面（零件）：收 hc:open-bili → 新标签（供 dock 标题等复用）
hcBus.on('open-bili', (d) => {
  if (d && d.bvid) window.open('https://www.bilibili.com/video/' + d.bvid, '_blank');
});
