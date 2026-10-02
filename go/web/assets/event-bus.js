// 事件总线（零件）：孤岛间只走 CustomEvent(hc:*)，不互相 import
// 广播不存储：监听方各取所需，无人听则事件自然消亡
(function () {
  const P = 'hc:';
  window.hcBus = {
    on(type, fn) { window.addEventListener(P + type, (e) => fn(e.detail, e)); },
    emit(type, detail) { window.dispatchEvent(new CustomEvent(P + type, { detail: detail || {} })); },
  };
})();
