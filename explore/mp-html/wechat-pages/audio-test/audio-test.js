const { BASE } = require('../../utils/config');

// 音频生死实验：InnerAudioContext 播真实 B站流（/api/v1/music/stream/{bvid}）
Page({
  data: { bvid: 'BV1dVqiY8EtF', playing: false, pos: 0, dur: 0, err: '' },

  onBvid(e) { this.setData({ bvid: e.detail.value }); },

  play() {
    const bvid = (this.data.bvid || '').trim();
    if (!bvid) return;
    if (!this.audio) {
      this.audio = wx.createInnerAudioContext();
      this.audio.volume = 1; // 满音量
      this.audio.onPlay(() => this.setData({ playing: true, err: '' }));
      this.audio.onPause(() => this.setData({ playing: false }));
      this.audio.onEnded(() => this.setData({ playing: false }));
      this.audio.onError((e) => this.setData({ err: 'audio error: ' + JSON.stringify(e) }));
      this.audio.onTimeUpdate(() => this.setData({ pos: this.audio.currentTime || 0, dur: this.audio.duration || 0 }));
    }
    this.setData({ err: '' });
    this.audio.src = BASE + '/api/v1/music/stream/' + bvid;
    this.audio.play();
    this.setData({ bvid });
  },

  pause() { if (this.audio) this.audio.pause(); },

  onUnload() { if (this.audio) { this.audio.stop(); this.audio.destroy(); } }
});
