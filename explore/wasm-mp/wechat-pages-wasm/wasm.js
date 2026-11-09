// mp-html + wasm 结合：wasm 跑 Go 逻辑（搜 B站 → 渲 HTML 片段 → 回调传回），mp-html 渲染
// 小程序侧注入全局 fetch（wx.request 适配）→ Go http.Get 自动走它（Go net/http 内置 fetch 桥）
// ① 小程序没有 TextDecoder/TextEncoder/crypto/performance，wasm_exec.js require 时顶层 throw → 先注入 polyfill
if (typeof globalThis.TextDecoder === 'undefined') {
  globalThis.TextDecoder = class {
    constructor() {}
    decode(buf) {
      const u8 = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
      let s = '', i = 0;
      while (i < u8.length) {
        const b = u8[i];
        if (b < 0x80) { s += String.fromCharCode(b); i++; }
        else if (b < 0xe0) { s += String.fromCharCode(((b & 0x1f) << 6) | (u8[i + 1] & 0x3f)); i += 2; }
        else if (b < 0xf0) { s += String.fromCharCode(((b & 0x0f) << 12) | ((u8[i + 1] & 0x3f) << 6) | (u8[i + 2] & 0x3f)); i += 3; }
        else { const cp = ((b & 0x07) << 18) | ((u8[i + 1] & 0x3f) << 12) | ((u8[i + 2] & 0x3f) << 6) | (u8[i + 3] & 0x3f); s += String.fromCodePoint(cp); i += 4; }
      }
      return s;
    }
  };
}
if (typeof globalThis.performance === 'undefined') {
  globalThis.performance = { now: () => Date.now() };
}
if (typeof globalThis.crypto === 'undefined') {
  globalThis.crypto = {
    getRandomValues: (arr) => { for (let i = 0; i < arr.length; i++) arr[i] = Math.floor(Math.random() * 256); return arr; }
  };
}
if (typeof globalThis.TextEncoder === 'undefined') {
  globalThis.TextEncoder = class {
    constructor() {}
    encode(str) {
      const bytes = [];
      for (const ch of String(str)) {
        const cp = ch.codePointAt(0);
        if (cp < 0x80) bytes.push(cp);
        else if (cp < 0x800) bytes.push(0xc0 | (cp >> 6), 0x80 | (cp & 0x3f));
        else if (cp < 0x10000) bytes.push(0xe0 | (cp >> 12), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
        else bytes.push(0xf0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3f), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
      }
      return new Uint8Array(bytes);
    }
  };
}
// tinygo 的 wasm_exec.js 用 global（小程序无 global/window/self）→ 注入；global.fs stub（跳过 require node:fs）
if (typeof globalThis.global === 'undefined') {
  globalThis.global = globalThis;
}
if (typeof globalThis.fs === 'undefined') {
  globalThis.fs = {
    constants: { O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 64, O_APPEND: 8 },
    readFileSync: () => new Uint8Array(0),
    writeFileSync: () => {},
    readdirSync: () => [],
    statSync: () => ({ isFile: () => true, isDirectory: () => false })
  };
}
require('../../wasm/wasm_exec.js'); // tinygo 配套：含 wasi_snapshot_preview1 stub + 设 globalThis.Go

Page({
  data: { kw: '', html: '', loading: false, ready: false, err: '' },

  onLoad() {
    try { this.initWasm(); }
    catch (e) { this.showErr('onLoad: ' + (e && e.message || e)); }
  },

  showErr(msg) {
    console.log('WASM 错误:', msg);
    this.setData({ err: msg });
    wx.showToast({ title: msg.slice(0, 30), icon: 'none' });
  },

  // ① 注入 fetch（wx.request 适配）→ Go 的 http.Get 自动用
  injectFetch() {
    globalThis.fetch = (url, opt) => new Promise((resolve, reject) => {
      const u = typeof url === 'string' ? url : String(url);
      wx.request({
        url: u,
        method: (opt && opt.method || 'GET').toUpperCase(),
        header: (opt && opt.headers) || {},
        data: (opt && opt.body) || undefined,
        success: (r) => resolve({
          status: r.statusCode,
          headers: r.header || {},
          text: async () => r.data,
          json: async () => (typeof r.data === 'string' ? JSON.parse(r.data) : r.data)
        }),
        fail: reject
      });
    });
  },

  // ② 加载 wasm（WXWebAssembly + Go 运行时胶水）
  initWasm() {
    this.injectFetch();
    if (typeof WXWebAssembly === 'undefined') { this.showErr('WXWebAssembly 未定义（基础库需 ≥2.13）'); return; }
    if (typeof globalThis.Go === 'undefined') { this.showErr('globalThis.Go 未定义（wasm_exec.js 没加载）'); return; }
    globalThis.WebAssembly = WXWebAssembly; // wasm_exec.js 的 instanceof 检查用
    const Go = globalThis.Go;
    const go = new Go();
    WXWebAssembly.instantiate('/wasm/backend.wasm.br', go.importObject).then((res) => {
      go.run(res.instance);
      this.setData({ ready: true });
      console.log('WASM 就绪');
    }).catch((e) => {
      this.showErr('instantiate: ' + (e && e.message || e));
    });
  },

  onInput(e) { this.setData({ kw: e.detail.value }); },

  // ③ 搜索：调 Go 回调（传 kw + 回调）→ Go 搜 B站 → 渲 HTML → 回调(html) → mp-html 渲
  onSearch() {
    const kw = (this.data.kw || '').trim();
    if (!kw) return;
    if (typeof globalThis.__searchBili === 'undefined') { this.showErr('__searchBili 未定义（wasm 未就绪）'); return; }
    this.setData({ loading: true, html: '' });
    globalThis.__searchBili(kw, (html) => {
      this.setData({ loading: false, html: html || '' });
    });
  },

  onLink(e) {
    const href = (e.detail && e.detail.href) || '';
    const m = href.match(/bvid=([^&]+)/);
    if (m) {
      console.log('播放 bvid=', m[1]);
      wx.showToast({ title: '播放 ' + m[1].slice(0, 15), icon: 'none' });
    }
    return true;
  }
});
