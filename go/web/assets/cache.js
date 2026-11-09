// 歌曲缓存（后端落盘版）：逻辑一份住 Go 后端（SQLite 元数据 + 磁盘文件），
// 前端只经 /api/v1/cache/* 接口读状态/清缓存；播放由 Stream 端点优先走缓存。
// 保留 hcCache 接口（get/put/sizeMB/clear/cacheAfterPlaying/limitMB）兼容 player.js 旧调用，
// 但 get 命中判定改由后端 Stream 完成（前端不再存 blob，避免重复缓存）。
(function () {
  const BASE = '/api/v1/cache';

  // 上限（MB）：localStorage hc:cachelimit 为旧键；后端 settings 持久化为权威。
  // 启动时同步一次：后端有则用后端，否则写后端默认。
  function limitMB() {
    const v = parseInt(localStorage.getItem('hc:cachelimit') || '500', 10);
    return isNaN(v) || v < 0 ? 500 : v;
  }

  // 已用大小（MB，两位小数）——后端统计
  async function sizeMB() {
    try {
      const r = await fetch(BASE + '/stats');
      const d = await r.json();
      const mb = (d.data && d.data.used_mb) || 0;
      return Math.round(mb * 100) / 100; // 两位小数
    } catch (e) { return 0; }
  }

  // 清空缓存：mode=all 全清 / unfav 只清未收藏
  async function clear(mode) {
    try { await fetch(BASE + '/clear?mode=' + (mode || 'all'), { method: 'POST' }); } catch (e) {}
  }

  // 同步后端上限（设置页切换 / 启动校准）
  async function syncLimit(mb) {
    try {
      await fetch(BASE + '/limit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ limit_mb: mb }),
      });
    } catch (e) {}
  }

  // 兼容旧接口：get 不再本地取 blob（后端 Stream 已优先缓存）。返回 null 让
  // player.js 走网络流，后端负责回写缓存。保留以便旧调用不报错。
  async function get() { return null; }

  // 兼容旧接口：put 不再本地存（后端 Stream 已处理）。
  async function put() {}

  // 兼容旧接口：播放超 60s 触发——后端 Stream 已边播边缓存，此处 no-op。
  async function cacheAfterPlaying() {}

  window.hcCache = { get, put, sizeMB, clear, cacheAfterPlaying, limitMB, syncLimit };
})();
