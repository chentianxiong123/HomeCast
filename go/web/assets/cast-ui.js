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

  // 音箱投送反馈（speaker_panel 内 .sp-play-btn）
  hcBus.on('speaker-result', (d) => {
    const btn = document.querySelector('.sp-play-btn[data-did="' + d.did + '"]');
    if (!btn) return;
    const span = btn.querySelector('span');
    if (span) span.textContent = d.ok ? '✓ 音箱播放中' : d.msg || '失败';
    setTimeout(() => {
      if (span) span.textContent = '投送当前歌';
    }, 2500);
  });
})();