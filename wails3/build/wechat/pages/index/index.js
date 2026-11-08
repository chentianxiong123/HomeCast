// HomeCast 微信小程序壳逻辑层
// 职责：搜索 → 后端吐 HTML 片段 → mp-html 渲原生；点击卡片 → 播放音频
// 业务逻辑 100% 在 Go 后端，本文件只是"事件的接法"
const { BASE } = require('../../utils/config');

Page({
  data: {
    keyword: '',
    html: '',
    loading: false
  },

  onInput(e) {
    this.setData({ keyword: e.detail.value });
  },

  // 搜索：调 /hx/search-mp 拿纯展示 HTML 片段，喂 mp-html
  async onSearch() {
    const kw = this.data.keyword.trim();
    if (!kw) return;
    this.setData({ loading: true, html: '' });
    try {
      const res = await this.request(`${BASE}/hx/search-mp?kw=${encodeURIComponent(kw)}`);
      this.setData({ html: res.data, loading: false });
    } catch (e) {
      this.setData({ loading: false });
      wx.showToast({ title: '搜索失败：' + (e.errMsg || e), icon: 'none', duration: 2500 });
    }
  },

  // mp-html 的 link 事件：点击 <a href="hc-play://play?bvid=..."> → 解析 bvid → 播放
  onLink(e) {
    const href = (e.detail && e.detail.href) || '';
    const m = href.match(/bvid=([^&]+)/);
    if (!m) return;
    this.play(m[1]);
    return true; // 阻止 mp-html 默认"复制链接"行为
  },

  // 播放：InnerAudioContext 直接播后端代理流（/api/v1/music/stream/{bvid}）
  play(bvid) {
    if (this.audio) this.audio.destroy();
    this.audio = wx.createInnerAudioContext();
    this.audio.src = `${BASE}/api/v1/music/stream/${bvid}`;
    this.audio.onError((e) => {
      wx.showToast({ title: '播放失败：' + (e.errMsg || ''), icon: 'none' });
    });
    this.audio.play();
    wx.showToast({ title: '播放中…', icon: 'none' });
  },

  request(url) {
    return new Promise((resolve, reject) => {
      wx.request({ url, success: resolve, fail: reject });
    });
  },

  onUnload() {
    if (this.audio) this.audio.destroy();
  }
});
