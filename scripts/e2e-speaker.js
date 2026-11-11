// homecast 音箱控制全链路 E2E：真实浏览器 + 真实点击 + 真实音箱设备（LX05A 等），
// 覆盖：投送(播放) → 切音箱模式 → 暂停/恢复真调音箱 API → 切歌真投下一曲 →
// 音量走音箱 API → 切回 web 自动续播 → 控制台零报错。
// 用 Playwright MCP 的 filename 参数加载：跑之前先起服务，并需音箱设备在线、登录态有效。
// 非音箱环境可跳过（脚本会在投送按钮缺失时标记 SKIP 而非 FAIL，方便 CI 无音箱环境跑）。
async (page) => {
  const out = { steps: [], errors: [], skipped: [] };
  const step = (name, ok, detail) => out.steps.push({ name, ok, detail: detail == null ? '' : detail });
  const skip = (name, why) => out.skipped.push(name + ' (' + why + ')');
  page.on('console', (m) => { if (m.type() === 'error') out.errors.push(m.text()); });
  page.on('pageerror', (e) => out.errors.push('PAGEERROR: ' + e.message));
  const wait = (ms) => page.waitForTimeout(ms);
  // 400 收集：page.on('response') 全覆盖（fetch/XHR/img/audio 媒体资源），记录 >=400 的 URL
  let badReqs = [];
  page.on('response', (res) => { if (res.status() >= 400) badReqs.push(res.status() + ' ' + res.url()); });

  // 1. 首页确认有歌
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto('http://127.0.0.1:28976/', { waitUntil: 'domcontentloaded', timeout: 60000 });
  await wait(800);
  const init = await page.evaluate(() => {
    const p = window.hcState.get();
    return { hasSong: !!p.bvid, source: p.source, title: document.getElementById('d-title').textContent };
  });
  step('1.首页有歌', init.hasSong, 'title=' + init.title);

  // 2. 投屏页面导航（真实点击 nav 链接）
  await page.evaluate(() => {
    const l = Array.from(document.querySelectorAll('a[href="/hx/cast"]'));
    if (l.length) l[0].click();
  });
  await wait(3000);
  const devBtns = await page.evaluate(() =>
    Array.from(document.querySelectorAll('.sp-play-btn')).map((b) => b.dataset.did));
  step('2.投屏页加载出音箱设备', devBtns.length > 0, 'devices=' + JSON.stringify(devBtns));
  if (!devBtns.length) { skip('3~9 音箱控制', '无音箱设备在线'); return out; }

  // 3. 投送音箱：优先已验证可投的 LX05A(108654953)，其次按序尝试
  const did = devBtns.includes('108654953') ? '108654953' : devBtns[0];
  await page.evaluate((d) => {
    const b = Array.from(document.querySelectorAll('.sp-play-btn')).find((x) => x.dataset.did === d);
    if (b) b.click();
  }, did);
  await wait(5000);
  const cast = await page.evaluate(() => {
    const p = window.hcState.get();
    return { source: p.source, spkDid: p.spk_did || '', playing: p.playing };
  });
  step('3.投送→切音箱模式', cast.source === 'speaker' && cast.spkDid === did,
    'source=' + cast.source + ' did=' + cast.spkDid);
  if (cast.source !== 'speaker') {
    skip('4~9 音箱控制面', '投送失败（设备 ' + did + ' 可能不支持投流，或登录态失效）');
    return out;
  }

  // 4. 暂停（d-toggle 真实点击）→ 音箱 pause + pos 静止
  const p1 = await page.evaluate(() => window.hcState.get().position);
  await page.evaluate(() => document.getElementById('d-toggle').click());
  await wait(1500);
  const pa1 = await page.evaluate(() => {
    const p = window.hcState.get();
    return { playing: p.playing, pos: p.position };
  });
  await wait(2500);
  const pa2 = await page.evaluate(() => window.hcState.get().position);
  step('4a.音箱模式暂停→playing=false', pa1.playing === false, 'playing=' + pa1.playing);
  step('4b.暂停后进度静止', Math.abs(pa2 - pa1.pos) < 0.5,
    'pos ' + pa1.pos.toFixed(1) + '→' + pa2.toFixed(1));

  // 5. 恢复（d-toggle 再点）→ pos 继续走
  await page.evaluate(() => document.getElementById('d-toggle').click());
  await wait(1500);
  const r1 = await page.evaluate(() => window.hcState.get().position);
  await wait(2500);
  const r2 = await page.evaluate(() => window.hcState.get().position);
  step('5.恢复→进度继续(音箱真在播)', r2 > r1 + 1,
    'pos ' + r1.toFixed(1) + '→' + r2.toFixed(1));

  // 6. 切歌（d-next 真实点击）→ bvid 变 + 音箱真投下一曲
  // 注意：读后端权威 /api/v1/state（Alpine store 走 SSE 250ms 低频可能滞后，后端才是 SSOT）
  const b0 = await page.evaluate(async () => (await fetch('/api/v1/state').then((r) => r.json())).data.bvid);
  await page.evaluate(() => document.getElementById('d-next').click());
  await wait(4000);
  const n1 = await page.evaluate(async () => {
    const d = (await fetch('/api/v1/state').then((r) => r.json())).data;
    return { bvid: d.bvid, title: d.title, pos: d.position };
  });
  step('6.切歌→bvid变(音箱重投)', n1.bvid !== b0 && n1.pos < 10,
    b0.slice(-6) + '→' + n1.bvid.slice(-6) + ' pos=' + n1.pos.toFixed(1));

  // 7. 音量滑块（input 事件）音箱模式走 API（不崩即可；实际音量设备端生效，本地无断言）
  await page.evaluate(() => {
    const v = document.getElementById('d-vol');
    v.value = 40; v.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await wait(1000);
  step('7.音箱模式音量滑块不崩', true, 'set 40→音箱 API');

  // 8. 音箱模式徽标可见
  const badge = await page.evaluate(() => {
    const b = document.getElementById('d-spk-mode');
    return { exists: !!b, visible: b ? !b.classList.contains('hidden') : false };
  });
  step('8.音箱徽标显示', badge.exists && badge.visible, '');

  // 9. 切回 web（真实点击徽标）→ source=web + playing 保留 + audio 自动续播
  await page.evaluate(() => document.getElementById('d-spk-mode').click());
  await wait(3000);
  const w1 = await page.evaluate(() => {
    const p = window.hcState.get();
    const a = document.querySelector('audio');
    return { source: p.source, playing: p.playing, audioPaused: a.paused,
      audioT: a.currentTime, srcOk: a.src.includes('/api/v1/music/stream/') };
  });
  await wait(2000);
  const w2 = await page.evaluate(() => document.querySelector('audio').currentTime);
  step('9a.切回web→source=web', w1.source === 'web', 'source=' + w1.source);
  step('9b.切回web→playing保留+audio续播', w1.playing === true && w1.audioPaused === false &&
    w1.srcOk && w2 > w1.audioT, 't ' + w1.audioT.toFixed(1) + '→' + w2.toFixed(1));

  // 10. 控制台零真实报错
  const realErrors = out.errors.filter((e) => !e.includes('cdn.tailwindcss.com') && !e.includes('BVold1')
    && !e.includes('Autoplay'));
  step('10.控制台无真实报错', realErrors.length === 0 && badReqs.length === 0,
    'console=' + JSON.stringify(realErrors.slice(0, 3)) + ' 4xx=' + JSON.stringify(badReqs.slice(0, 5)));

  return out;
}