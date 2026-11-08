// web-view 薄壳：整站加载 HomeCast H5，业务/播放器/歌词全在 Go 后端 + go:embed 前端
const { BASE } = require('../../utils/config');

Page({
  data: {
    url: `${BASE}/`
  },
  onMsg(e) {
    // web-view 消息（如需 web 端 → 小程序通信，在此处理）
    console.log('webview msg', e.detail);
  }
});
