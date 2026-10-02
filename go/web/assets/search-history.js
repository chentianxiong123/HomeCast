// 搜索历史（零件/服务器态补充）：localStorage P 态记录 + 搜索框下 chips
// 记录时机：任何 /hx/search?kw= 请求发出前（htmx:beforeRequest）；渲染跟随页面切换
(function () {
  const KEY = 'hc:searches';
  const MAX = 8;
  function load() {
    try { return JSON.parse(localStorage.getItem(KEY) || '[]'); } catch (e) { return []; }
  }
  function save(list) {
    try { localStorage.setItem(KEY, JSON.stringify(list)); } catch (e) {}
  }
  function record(kw) {
    kw = (kw || '').trim();
    if (!kw) return;
    let list = load().filter((k) => k !== kw);
    list.unshift(kw);
    save(list.slice(0, MAX));
  }
  function clear(one) {
    let list = load();
    if (one) list = list.filter((k) => k !== one);
    else list = [];
    save(list);
    render();
  }
  function render() {
    const box = document.getElementById('search-history');
    if (!box) return;
    const list = load();
    if (!list.length) { box.innerHTML = ''; return; }
    box.innerHTML =
      '<div class="flex flex-wrap items-center justify-center gap-2 mt-4">' +
      list.map((k) =>
        '<button class="search-his-chip px-3 py-1.5 rounded-full text-sm bg-gray-800/70 border border-gray-700 text-gray-300 hover:border-pink-500/60 hover:text-pink-300 transition-colors" data-kw="' +
        k.replace(/"/g, '&quot;') + '">' + k.replace(/</g, '&lt;') + '</button>'
      ).join('') +
      '<button class="search-his-clear px-2 py-1.5 text-xs text-gray-600 hover:text-red-400 transition-colors" title="清空历史">清空</button></div>';
    box.querySelectorAll('.search-his-chip').forEach((b) => {
      b.addEventListener('click', () => {
        const inp = document.querySelector('input[name=kw]');
        if (inp) { inp.value = b.dataset.kw; inp.dispatchEvent(new Event('input', { bubbles: true })); }
      });
    });
    box.querySelector('.search-his-clear').addEventListener('click', () => clear());
  }
  document.addEventListener('htmx:beforeRequest', (e) => {
    const d = e.detail && (e.detail.requestConfig || e.detail);
    const path = (d && (d.path || (d.requestConfig && d.requestConfig.path))) || '';
    if (path.includes('/hx/search')) {
      const params = (d && (d.parameters || (d.requestConfig && d.requestConfig.parameters))) || {};
      record(params.kw || '');
    }
  });
  document.addEventListener('htmx:afterSwap', render);
  render();
})();