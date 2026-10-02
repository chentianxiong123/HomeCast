// 快捷键（零件）：空格播放暂停、←/→ 切歌、M 静音；输入框聚焦时忽略
// 只广播 hc: 事件，播放器逻辑不在此处（孤岛隔离）
(function () {
  const isTyping = (e) => {
    const t = e.target;
    return t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable);
  };
  document.addEventListener('keydown', (e) => {
    if (isTyping(e)) return;
    switch (e.code) {
      case 'Space':
        e.preventDefault();
        hcBus.emit('toggle');
        break;
      case 'ArrowLeft':
        hcBus.emit('prev');
        break;
      case 'ArrowRight':
        hcBus.emit('next');
        break;
      case 'KeyM':
        hcBus.emit('mute');
        break;
      // 多媒体键（键盘上的播放/暂停等）
      case 'MediaPlayPause':
        e.preventDefault();
        hcBus.emit('toggle');
        break;
      case 'MediaTrackPrevious':
        hcBus.emit('prev');
        break;
      case 'MediaTrackNext':
        hcBus.emit('next');
        break;
    }
  });
})();