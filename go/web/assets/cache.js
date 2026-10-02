// 歌曲缓存（零件）：IndexedDB 存音频 blob + LRU + 上限（YesPlayMusic automaticallyCacheSongs 同款）。
// 纯前端零后端：bvid → {data: Blob, size, lastUsed}；超上限按 lastUsed 淘汰最旧。
(function () {
  const DB = 'hc-cache';
  const STORE = 'songs';

  // 上限（MB）：localStorage hc:cachelimit；'0'=关
  function limitMB() {
    const v = parseInt(localStorage.getItem('hc:cachelimit') || '500', 10);
    return isNaN(v) || v < 0 ? 500 : v;
  }

  function openDB() {
    return new Promise((resolve, reject) => {
      const r = indexedDB.open(DB, 1);
      r.onupgradeneeded = () => {
        if (!r.result.objectStoreNames.contains(STORE)) {
          r.result.createObjectStore(STORE, { keyPath: 'bvid' });
        }
      };
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => reject(r.error);
    });
  }

  function tx(db, mode, fn) {
    return new Promise((resolve, reject) => {
      const t = db.transaction(STORE, mode);
      const req = fn(t.objectStore(STORE));
      t.oncomplete = () => resolve(req ? req.result : undefined);
      t.onerror = () => reject(t.error);
      t.onabort = () => reject(t.error);
    });
  }

  // 取缓存（音质需匹配，否则不算命中；命中刷新 lastUsed）
  async function get(bvid, quality) {
    if (!bvid || limitMB() === 0) return null;
    try {
      const db = await openDB();
      const row = await tx(db, 'readwrite', (st) => st.get(bvid));
      if (row && row.data && row.quality === quality) {
        row.lastUsed = Date.now();
        await tx(db, 'readwrite', (st) => st.put(row));
        return row.data;
      }
      return null;
    } catch (e) { return null; }
  }

  // 缓存当前音质的流（存 quality 标记；切音质后按需重新缓存）
  async function put(bvid, blob, quality) {
    if (!bvid || !blob || limitMB() === 0) return;
    try {
      const db = await openDB();
      await tx(db, 'readwrite', (st) => st.put({ bvid, quality: quality || '', data: blob, size: blob.size, lastUsed: Date.now() }));
      await trimIfNeeded();
    } catch (e) {}
  }

  // LRU 淘汰：总量超上限时删最旧，直到达标
  async function trimIfNeeded() {
    const limit = limitMB();
    if (limit === 0) return;
    try {
      const db = await openDB();
      const rows = await tx(db, 'readonly', (st) => st.getAll());
      const limitBytes = limit * 1024 * 1024;
      let total = rows.reduce((s, r) => s + (r.size || 0), 0);
      if (total <= limitBytes) return;
      rows.sort((a, b) => (a.lastUsed || 0) - (b.lastUsed || 0)); // 最旧在前
      const del = tx(db, 'readwrite', (st) => st);
      for (const r of rows) {
        if (total <= limitBytes) break;
        del.delete(r.bvid);
        total -= r.size || 0;
      }
    } catch (e) {}
  }

  // 已用大小（MB，两位小数）
  async function sizeMB() {
    try {
      const db = await openDB();
      const rows = await tx(db, 'readonly', (st) => st.getAll());
      const total = rows.reduce((s, r) => s + (r.size || 0), 0);
      return Math.round((total / 1024 / 1024) * 100) / 100;
    } catch (e) { return 0; }
  }

  async function clear() {
    try {
      const db = await openDB();
      await tx(db, 'readwrite', (st) => st.clear());
    } catch (e) {}
  }

  // 后台缓存：播放超过 60s 且未缓存时，拉全量音频存库（防并发）
  let caching = {};
  async function cacheAfterPlaying(bvid, streamURL) {
    if (!bvid || !streamURL || caching[bvid] || limitMB() === 0) return;
    const have = await get(bvid);
    if (have) return;
    caching[bvid] = true;
    try {
      const resp = await fetch(streamURL);
      if (!resp.ok) return;
      const blob = await resp.blob();
      if (blob && blob.size > 1024 * 64) { // 防坏流/极小响应
        await put(bvid, blob, (streamURL.match(/quality=(\d+)/) || [0, ''])[1]);
        hcBus.emit('cache-changed');
      }
    } catch (e) {} finally {
      delete caching[bvid];
    }
  }

  window.hcCache = { get, put, sizeMB, clear, cacheAfterPlaying, limitMB };
})();