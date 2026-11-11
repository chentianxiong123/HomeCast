// homecast 播放控制全键位 E2E：真实浏览器 + 真实点击（模拟用户操作每个按钮），
// 覆盖：播放/暂停/恢复、上一首/下一首、播放模式循环、音量/静音、seek 全范围、
// 切歌时长更新、歌词面板开关、进度条 UI 实时性。
// 用 Playwright MCP 的 filename 参数加载：跑之前先起服务（hcexp 或 go run ./cmd/server），
// 需队列里有 >=2 首歌（自动切歌/手动切歌的边界都能测）。
async (page) => {
  const out = { steps: [], errors: [] };
  const step = (name, ok, detail) => out.steps.push({ name, ok, detail: detail == null ? '' : detail });
  page.on('console', (m) => { if (m.type() === 'error') out.errors.push(m.text()); });
  page.on('pageerror', (e) => out.errors.push('PAGEERROR: ' + e.message));

  const click = async (sel) => page.evaluate((s) => {
    const el = document.querySelector(s);
    if (!el) return 'missing:' + s;
    el.click();
    return 'clicked';
  }, sel);
  const wait = (ms) => page.waitForTimeout(ms);

  // 1. 首页加载（初始状态：有歌 + dock 可见）
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto('http://127.0.0.1:28976/', { waitUntil: 'domcontentloaded', timeout: 60000 });
  await wait(800);
  const init = await page.evaluate(() => {
    const p = window.hcState.get();
    return { hasSong: !!p.bvid, qLen: (p.queue || []).length, dock: !!document.getElementById('player-dock'),
      qLen: (p.queue || []).length, title: document.getElementById('d-title').textContent };
  });
  step('1.首页有歌+dock+队列', init.hasSong && init.dock && init.qLen >= 2,
    'qLen=' + init.qLen + ' title=' + init.title);

  // 2. 播放（若未播则点播放）
  await page.evaluate(() => { const a = document.querySelector('audio'); if (a && a.paused) a.play().catch(() => {}); });
  await wait(2500);
  const playing = await page.evaluate(() => {
    const a = document.querySelector('audio');
    return { playing: !!(a && !a.paused && a.currentTime > 0.5), t: a ? a.currentTime : 0,
      cur: document.getElementById('d-cur').textContent,
      dur: document.getElementById('d-dur').textContent };
  });
  step('2.真实播放(进度条和时长有值)', playing.playing && playing.cur !== '0:00' && playing.dur !== '0:00',
    't=' + playing.t.toFixed(1) + ' cur=' + playing.cur + ' dur=' + playing.dur);

  // 3. 暂停 / 恢复（d-toggle 真实点击）
  await click('#d-toggle'); await wait(1200);
  const paused = await page.evaluate(() => { const a = document.querySelector('audio'); return a.paused; });
  step('3a.点击暂停→audio.paused', paused === true, 'paused=' + paused);
  await click('#d-toggle'); await wait(1200);
  const resumed = await page.evaluate(() => { const a = document.querySelector('audio'); return !a.paused; });
  step('3b.再点恢复→在播', resumed === true, 'resumed=' + resumed);

  // 4. 手动切歌 d-next：bvid 变 + 时长能更新（防 3:04 卡死回归）
  const b0 = await page.evaluate(() => window.hcState.get().bvid);
  const dur0 = await page.evaluate(() => window.hcState.get().duration);
  await click('#d-next'); await wait(3000);
  const n1 = await page.evaluate(() => {
    const p = window.hcState.get();
    const a = document.querySelector('audio');
    return { bvid: p.bvid, dur: p.duration, audioDur: a.duration || 0,
      durText: document.getElementById('d-dur').textContent, src: a.src.slice(-30) };
  });
  step('4a.下一首→bvid变', n1.bvid !== b0, b0.slice(-6) + '→' + n1.bvid.slice(-6));
  // 等 audio 元数据就绪（外网流加载网络延迟，最多 8s）
  let audioReady = await page.evaluate(async () => {
    const a = document.querySelector('audio');
    for (let i = 0; i < 80; i++) { if (a.duration > 0) break; await new Promise(r => setTimeout(r, 100)); }
    return a.duration || 0;
  });
  const nDur = await page.evaluate(() => window.hcState.get().duration);
  step('4b.切歌后时长即时更新', Math.abs(nDur - dur0) > 0.5 &&
    Math.abs(audioReady - nDur) < 2, 'dur=' + nDur.toFixed(1) + ' audioDur=' + audioReady.toFixed(1));

  // 5. 上一首 d-prev
  const b1 = n1.bvid;
  await click('#d-prev'); await wait(2500);
  const p1 = await page.evaluate(() => window.hcState.get().bvid);
  step('5.上一首→bvid变回', p1 === b0, b1.slice(-6) + '→' + p1.slice(-6));

  // 6. seek 全范围（change 真实拖动松手事件）：30/70/95%，seek 后不跳末尾不切歌
  const bSeek = await page.evaluate(() => window.hcState.get().bvid); // seek 前当前歌
  const seekResults = [];
  for (const v of [300, 700, 950]) {
    await page.evaluate((val) => {
      const el = document.getElementById('d-progress');
      el.value = val;
      el.dispatchEvent(new Event('change', { bubbles: true }));
    }, v);
    await wait(2000);
    const s = await page.evaluate(() => {
      const a = document.querySelector('audio');
      const p = window.hcState.get();
      return { t: a.currentTime, dur: a.duration || 1, bvid: p.bvid, ended: a.ended,
        prog: document.getElementById('d-progress').value };
    });
    seekResults.push(s);
    step('6.seek ' + v + '‰→位置落点', Math.abs(s.t / s.dur - v / 1000) < 0.08,
      'sec=' + s.t.toFixed(1) + '/' + s.dur.toFixed(1) + ' ended=' + s.ended);
    step('6b.seek 后不切歌不跳末尾', !s.ended && s.bvid === bSeek, 'bvid=' + s.bvid.slice(-6) + ' expect=' + bSeek.slice(-6));
  }

  // 7. seek 后进度条 UI 恢复实时（seeking 标志重置回归：seek 后切歌时长要更新）
  await click('#d-next'); await wait(2500);
  const audioReady2 = await page.evaluate(async () => {
    const a = document.querySelector('audio');
    for (let i = 0; i < 80; i++) { if (a.duration > 0) break; await new Promise(r => setTimeout(r, 100)); }
    return a.duration ? a.duration.toFixed(0) : 0;
  });
  const afterSeekCut = await page.evaluate(() => {
    const p = window.hcState.get();
    return { durText: document.getElementById('d-dur').textContent,
      prog: document.getElementById('d-progress').value };
  });
  step('7.seek后切歌时长仍更新', audioReady2 > 0, JSON.stringify(Object.assign(afterSeekCut, {audioDur: audioReady2})));

  // 8. 播放模式循环切换（d-mode 真实点击，order→loop）
  const m0 = await page.evaluate(() => window.hcState.get().play_mode);
  await click('#d-mode'); await wait(800);
  const m1 = await page.evaluate(() => window.hcState.get().play_mode);
  step('8.模式切换生效', m1 !== m0, m0 + '→' + m1);

  // 9. 音量滑块（input 事件）→ audio.volume 变
  await page.evaluate(() => {
    const v = document.getElementById('d-vol');
    v.value = 50; v.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await wait(500);
  const vol = await page.evaluate(() => document.querySelector('audio').volume);
  step('9.音量滑块→volume 0.5', Math.abs(vol - 0.5) < 0.01, 'volume=' + vol);

  // 10. 静音（d-mute 真实点击）→ muted
  await click('#d-mute'); await wait(500);
  const muted = await page.evaluate(() => document.querySelector('audio').muted);
  step('10.静音→muted', muted === true, 'muted=' + muted);
  await click('#d-mute'); await wait(500); // 恢复

  // 11. 歌词面板开关（d-cover 真实点击）
  await click('#d-cover'); await wait(1000);
  const lyOpen = await page.evaluate(() => {
    const p = document.getElementById('lyric-panel');
    return p ? !p.classList.contains('ly-hidden') : false;
  });
  step('11a.歌词面板打开', lyOpen === true, '');
  await click('#d-cover'); await wait(800);
  const lyClosed = await page.evaluate(() => {
    const p = document.getElementById('lyric-panel');
    return p ? p.classList.contains('ly-hidden') : false;
  });
  step('11b.再点关闭', lyClosed === true, '');

  // 12. 控制台零真实报错
  const realErrors = out.errors.filter(e => !e.includes('cdn.tailwindcss.com') && !e.includes('BVold1')
    && !e.includes('Autoplay') && !e.includes('play() failed'));
  step('12.控制台无真实报错', realErrors.length === 0, JSON.stringify(realErrors.slice(0, 5)));

  return out;
}