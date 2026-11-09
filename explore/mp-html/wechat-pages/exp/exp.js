// 实验：mp-html 高频刷新（500ms setData）测渲染流畅度
// 播放条（节点少）+ 歌词面板（12 行节点多）同时刷，测多节点 mp-html 性能
const { BASE } = require('../../utils/config');

Page({
  data: {
    bar: '',
    lyric: ''
  },
  timer: null,
  onLoad() {
    this.poll();
    this.timer = setInterval(() => this.poll(), 500);
  },
  onUnload() {
    if (this.timer) clearInterval(this.timer);
  },
  poll() {
    wx.request({
      url: `${BASE}/hx/player-bar`,
      success: (res) => this.setData({ bar: res.data }),
      fail: (e) => console.log('bar fail', e.errMsg)
    });
    wx.request({
      url: `${BASE}/hx/lyric-panel`,
      success: (res) => this.setData({ lyric: res.data }),
      fail: (e) => console.log('lyric fail', e.errMsg)
    });
  },
  onLink(e) {
    const href = (e.detail && e.detail.href) || '';
    console.log('linktap action:', href);
    return true;
  }
});
