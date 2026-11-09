const { BASE } = require('../../utils/config');

// 纯 mp-html 渲染 UI：后端吐 HTML 片段 → mp-html 渲（卡片/播放条/歌词）→ 500ms 刷新动态
// 不注重功能：音频/交互先不管，先把 UI 结构 + 动态渲染做出来
Page({
  data: { kw: '', cards: '', bar: '', lyric: '', err: '' },

  onLoad() {
    this.timer = setInterval(() => this.refresh(), 500);
    this.refresh();
  },

  onUnload() { clearInterval(this.timer); },

  onInput(e) { this.setData({ kw: e.detail.value }); },

  onSearch() {
    const kw = (this.data.kw || '').trim();
    if (!kw) return;
    this.setData({ err: '' });
    wx.request({
      url: BASE + '/hx/search-mp?kw=' + encodeURIComponent(kw),
      success: (r) => this.setData({ cards: r.data || '' }),
      fail: (e) => this.setData({ err: '搜索失败: ' + (e && e.errMsg || e) })
    });
  },

  refresh() {
    wx.request({ url: BASE + '/hx/player-bar', success: (r) => this.setData({ bar: r.data || '' }) });
    wx.request({ url: BASE + '/hx/lyric-panel', success: (r) => this.setData({ lyric: r.data || '' }) });
  }
});
