// 设置页（零件）：集中展示全部 P 态开关（无黑盒——当前值全部可见），
// 修改即写 localStorage + 广播 hc:* 事件（对应孤岛各取其值）
(function () {
  let inited = false;
  function init() {
  const view = document.getElementById('settings-view');
  if (!view) { inited = false; return; }
  if (inited) return;
  inited = true;
  const envDesktop = window.hcEnv === 'desktop';

  const S = (k, d) => { try { return localStorage.getItem(k) ?? d; } catch (e) { return d; } };
  const W = (k, v) => { try { localStorage.setItem(k, v); } catch (e) {} };

  const LY_SIZES = [14, 16, 18, 22, 26];
  const SUB_SIZES = [16, 20, 24, 28, 32];
  const MODES = ['order', 'loop', 'single', 'random'];
  const MODE_NAMES = { order: '顺序播放', loop: '列表循环', single: '单曲循环', random: '随机播放' };
  const SPEEDS = [0.75, 1, 1.25, 1.5, 2];

  function render() {
    const subOn = envDesktop ? false : S('hc:subtitle', '1') !== '0';
    const quality = S('hc:quality', '192');
    const speed = S('hc:speed', '1');
    const mode = S('hc:mode', 'order');
    const lyIdx = Math.max(0, LY_SIZES.indexOf(parseInt(S('hc:lysize', '18'), 10)));
    const subIdx = Math.max(0, SUB_SIZES.indexOf(parseInt(S('hc:subsize', '24'), 10)));
    const lybg = S('hc:lybg', 'cover');
    const lytime = S('hc:lytime', '0') === '1';
    const hisCount = (JSON.parse(S('hc:searches', '[]')) || []).length;

    view.innerHTML =
      '<div class="max-w-2xl mx-auto">' +
      '<h2 class="text-xl font-bold text-gray-900 dark:text-white mb-6">设置</h2>' +

      // 字幕
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<div class="flex items-center justify-between">' +
      '<div><p class="font-medium text-gray-900 dark:text-white">网页字幕条</p>' +
      '<p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">桌面壳环境默认关（由桌面歌词挂件承担）</p></div>' +
      '<label class="relative inline-flex items-center cursor-pointer">' +
      '<input type="checkbox" class="sr-only peer" id="set-sub" ' + (subOn ? 'checked' : '') + '>' +
      '<div class="w-11 h-6 bg-gray-600 peer-checked:bg-pink-500 rounded-full transition-colors"></div>' +
      '<div class="dot absolute w-4 h-4 bg-white rounded-full left-1 transition-transform peer-checked:translate-x-5 pointer-events-none"></div>' +
      '</label></div></div>' +

      // 音质
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<p class="font-medium text-gray-900 dark:text-white mb-2">音质</p>' +
      '<div class="flex space-x-2">' +
      [64, 128, 192].map((q) =>
        '<button class="set-q px-4 py-1.5 rounded-full text-sm font-medium transition-colors ' +
        (parseInt(quality, 10) === q ? 'bg-gradient-to-r from-pink-500 to-violet-500 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300') +
        '" data-q="' + q + '">' + q + 'k</button>').join('') +
      '</div></div>' +

      // 速度
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<p class="font-medium text-gray-900 dark:text-white mb-2">播放速度</p>' +
      '<div class="flex space-x-2">' +
      SPEEDS.map((v) =>
        '<button class="set-spd px-4 py-1.5 rounded-full text-sm font-medium transition-colors ' +
        (parseFloat(speed) === v ? 'bg-gradient-to-r from-pink-500 to-violet-500 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300') +
        '" data-v="' + v + '">' + v + 'x</button>').join('') +
      '</div></div>' +

      // 播放模式
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<p class="font-medium text-gray-900 dark:text-white mb-2">播放模式</p>' +
      '<p class="text-sm text-gray-500 dark:text-gray-400 mb-2">当前：<span class="text-pink-400 font-medium">' + (MODE_NAMES[mode] || '顺序播放') + '</span>（点按钮切换）</p>' +
      '<div class="flex space-x-2">' +
      MODES.map((m) =>
        '<button class="set-mode px-4 py-1.5 rounded-full text-sm font-medium transition-colors ' +
        (mode === m ? 'bg-gradient-to-r from-pink-500 to-violet-500 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300') +
        '" data-m="' + m + '">' + MODE_NAMES[m] + '</button>').join('') +
      '</div></div>' +

      // 歌词字号
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<p class="font-medium text-gray-900 dark:text-white mb-2">歌词字号</p>' +
      '<div class="flex space-x-2">' +
      LY_SIZES.map((v, i) =>
        '<button class="set-ly px-4 py-1.5 rounded-full text-sm font-medium transition-colors ' +
        (lyIdx === i ? 'bg-gradient-to-r from-pink-500 to-violet-500 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300') +
        '" data-i="' + i + '">' + v + 'px</button>').join('') +
      '</div></div>' +

      // 歌词背景模式
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<p class="font-medium text-gray-900 dark:text-white mb-2">歌词背景</p>' +
      '<div class="flex space-x-2">' +
      '<button class="set-lybg px-4 py-1.5 rounded-full text-sm font-medium transition-colors ' +
      (lybg === 'cover' ? 'bg-gradient-to-r from-pink-500 to-violet-500 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300') +
      '" data-v="cover">封面模糊</button>' +
      '<button class="set-lybg px-4 py-1.5 rounded-full text-sm font-medium transition-colors ' +
      (lybg === 'solid' ? 'bg-gradient-to-r from-pink-500 to-violet-500 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300') +
      '" data-v="solid">纯色</button>' +
      '</div></div>' +

      // 歌词时间显示
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<div class="flex items-center justify-between">' +
      '<div><p class="font-medium text-gray-900 dark:text-white">歌词时间显示</p>' +
      '<p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">歌词行旁显示时间戳</p></div>' +
      '<label class="relative inline-flex items-center cursor-pointer">' +
      '<input type="checkbox" class="sr-only peer" id="set-lytime" ' + (lytime ? 'checked' : '') + '>' +
      '<div class="w-11 h-6 bg-gray-600 peer-checked:bg-pink-500 rounded-full transition-colors"></div>' +
      '<div class="dot absolute w-4 h-4 bg-white rounded-full left-1 transition-transform peer-checked:translate-x-5 pointer-events-none"></div>' +
      '</label></div></div>' +

      // 字幕字号（横向字幕条）
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<p class="font-medium text-gray-900 dark:text-white mb-1">字幕字号（横向字幕条）</p>' +
      '<p class="text-xs text-gray-500 dark:text-gray-400 mb-2">当前字号：' + SUB_SIZES[subIdx] + 'px · 拖动字幕条可移动位置</p>' +
      '<div class="flex space-x-2">' +
      SUB_SIZES.map((v, i) =>
        '<button class="set-sub px-4 py-1.5 rounded-full text-sm font-medium transition-colors ' +
        (subIdx === i ? 'bg-gradient-to-r from-pink-500 to-violet-500 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300') +
        '" data-i="' + i + '">' + v + 'px</button>').join('') +
      '</div></div>' +

      // 数据
      '<div class="card p-5 mb-4 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<p class="font-medium text-gray-900 dark:text-white mb-2">数据</p>' +
      '<div class="flex items-center justify-between">' +
      '<p class="text-sm text-gray-500 dark:text-gray-400">搜索历史（' + hisCount + ' 条）</p>' +
      '<button class="set-clrhis px-3 py-1.5 rounded-full text-sm text-gray-400 hover:text-red-400 border border-gray-700 transition-colors">清空历史</button>' +
      '</div></div>' +

      // 关于
      '<div class="card p-5 bg-white dark:bg-gray-800 rounded-2xl border border-gray-100 dark:border-gray-700">' +
      '<p class="font-medium text-gray-900 dark:text-white mb-1">关于</p>' +
      '<p class="text-xs text-gray-500 dark:text-gray-400">HomeCast Go + htmx · 环境：' + (envDesktop ? '桌面壳（hcEnv=desktop，字幕走桌面挂件）' : '网页') + '</p>' +
      '</div>' +
      '</div>';

    // 绑定
    const sub = document.getElementById('set-sub');
    if (sub) sub.addEventListener('change', () => hcBus.emit('subtitle-toggle', { on: sub.checked }));
    view.querySelectorAll('.set-q').forEach((b) => b.addEventListener('click', () => {
      const q = parseInt(b.dataset.q, 10); W('hc:quality', String(q)); hcBus.emit('quality', { q }); render();
    }));
    view.querySelectorAll('.set-spd').forEach((b) => b.addEventListener('click', () => {
      const v = parseFloat(b.dataset.v); W('hc:speed', String(v)); hcBus.emit('speed', { v }); render();
    }));
    view.querySelectorAll('.set-mode').forEach((b) => b.addEventListener('click', () => {
      const m = b.dataset.m; W('hc:mode', m); hcBus.emit('mode', { m }); render();
    }));
    view.querySelectorAll('.set-ly').forEach((b) => b.addEventListener('click', () => {
      const i = parseInt(b.dataset.i, 10); W('hc:lysize', String(LY_SIZES[i])); hcBus.emit('lysize', { idx: i }); render();
    }));
    view.querySelectorAll('.set-sub').forEach((b) => b.addEventListener('click', () => {
      const i = parseInt(b.dataset.i, 10); W('hc:subsize', String(SUB_SIZES[i])); hcBus.emit('subsize', { idx: i }); render();
    }));
    view.querySelectorAll('.set-lybg').forEach((b) => b.addEventListener('click', () => {
      W('hc:lybg', b.dataset.v); hcBus.emit('lybg', { v: b.dataset.v }); render();
    }));
    const lytimeCtl = document.getElementById('set-lytime');
    if (lytimeCtl) lytimeCtl.addEventListener('change', () => {
      W('hc:lytime', lytimeCtl.checked ? '1' : '0'); hcBus.emit('lytime', { on: lytimeCtl.checked }); render();
    });
    const clr = view.querySelector('.set-clrhis');
    if (clr) clr.addEventListener('click', () => {
      try { localStorage.removeItem('hc:searches'); } catch (e) {}
      hcBus.emit('toast', { msg: '搜索历史已清空' });
      render();
    });
  }

  render();
  }
  document.addEventListener('htmx:afterSwap', init);
  init();
})();