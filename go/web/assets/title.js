// 标题呈现（零件）：B站视频标题 → 只显示括号内容（《》（）【】「」及半角[] 内的文字全提取拼接）。
// 无括号 → 原题。点击 .hx-title 展开/收起完整名（data-short/data-full）。
// 与 Go 端 hx.titleDisplay 同规则（前后端一致）。
(function () {
  window.hcTitle = function (t) {
    const s = String(t == null ? '' : t);
    const parts = [];
    const re = /《([^》]+)》|（([^）]+)）|【([^】]+)】|「([^」]+)」|\[([^\]]+)\]/g;
    let m;
    while ((m = re.exec(s)) !== null) {
      const c = (m[1] || m[2] || m[3] || m[4] || m[5] || '').trim();
      if (c) parts.push(c);
    }
    return parts.length ? parts.join(' · ') : s;
  };

  function bindTitles(root) {
    root.querySelectorAll('.hx-title').forEach((el) => {
      if (el.dataset.hxTitleBound) return;
      el.dataset.hxTitleBound = '1';
      el.addEventListener('click', (e) => {
        e.stopPropagation();
        const full = el.dataset.full || '';
        const short = el.dataset.short || '';
        if (!full) return;
        if (el.textContent === full) {
          el.textContent = short;
          el.classList.remove('whitespace-normal', 'break-words');
          el.classList.add('truncate');
        } else {
          el.textContent = full;
          el.classList.remove('truncate');
          el.classList.add('whitespace-normal', 'break-words');
        }
      });
    });
  }
  document.addEventListener('htmx:afterSwap', () => bindTitles(document));
  bindTitles(document);
})();