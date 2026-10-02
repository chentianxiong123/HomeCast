// 投屏反馈零件（壳级常驻）：收 hc:cast-result 更新投送按钮文案（可重绘不存状态）
(function () {
  hcBus.on('cast-result', (d) => {
    const btn = document.querySelector('.cast-play-btn[data-udn="' + d.udn + '"]');
    if (!btn) return;
    const span = btn.querySelector('span');
    if (span) span.textContent = d.ok ? '✓ 已投送' : d.msg || '失败';
    btn.classList.toggle('opacity-60', !d.ok);
    setTimeout(() => {
      if (span) span.textContent = '投送';
      btn.classList.remove('opacity-60');
    }, 2500);
  });
})();