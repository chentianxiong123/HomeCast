// homecast E2E 冒烟：真实浏览器 + 真实外网（B站），验证核心用户流程
// 用 Playwright MCP 的 filename 参数加载：跑之前先起服务（/tmp/hc_server）
async (page) => {
  const out = { steps: [], errors: [] };
  const step = (name, ok, detail) => out.steps.push({ name, ok, detail: detail == null ? '' : detail });
  page.on('console', (m) => { if (m.type() === 'error') out.errors.push(m.text()); });
  page.on('pageerror', (e) => out.errors.push('PAGEERROR: ' + e.message));

  // 1. 首页加载
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto('http://127.0.0.1:28976/', { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForTimeout(600);
  step('1.首页加载(搜索框+dock)', await page.evaluate(() =>
    !!document.querySelector('input[name="kw"]') && !!document.getElementById('player-dock')), '');

  // 2. 真实搜索（外网 B 站）——header 有桌面/移动两个搜索框，只操作可见的那个；
  //    只 fill 触发 input debounce(600ms) 的 hx-get，不要按 Enter（会整页导航到服务端渲染，慢）
  const kw = page.locator('input[name="kw"]:visible');
  await kw.fill('周杰伦');
  await page.waitForTimeout(3000);
  const search = await page.evaluate(() => {
    const rows = document.querySelectorAll('#results .hx-title');
    const t = rows[0] ? rows[0].textContent : '';
    const full = rows[0] ? rows[0].dataset.full : '';
    return { rows: rows.length, firstShort: t, firstFull: full,
      shortWorks: t.length > 0 && (!full || t.length < full.length) };
  });
  step('2.搜索出结果>=5', search.rows >= 5, 'rows=' + search.rows);
  step('2b.标题短名清洗', search.shortWorks,
    'short=' + search.firstShort + ' | full=' + search.firstFull);

  // 3. 播放（真实音频流）
  await page.evaluate(() => localStorage.setItem('hc:subtitle', '0'));
  await page.evaluate(() => { const b = document.querySelector('#results .hx-play'); if (b) b.click(); });
  await page.waitForTimeout(5000);
  const play = await page.evaluate(() => {
    const a = document.querySelector('audio');
    const d = document.getElementById('d-title');
    return { hasAudio: !!a, playing: !!(a && !a.paused && a.currentTime > 0),
      dockTitle: d ? d.textContent : null, hoverTitle: d ? d.title : null,
      srcOk: !!(a && a.src && a.src.includes('/api/v1/music/stream/')) };
  });
  step('3.点击播放→真实音频在放', play.playing,
    'dock=' + play.dockTitle + ' hover=' + play.hoverTitle + ' srcOk=' + play.srcOk);

  // 4. 队列页
  await page.goto('http://127.0.0.1:28976/hx/queue', { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForTimeout(800);
  const queue = await page.evaluate(() => {
    const t = document.querySelector('#queue-view .q-title');
    return { qty: document.querySelectorAll('#queue-view .q-title').length,
      short: t ? t.textContent : null, full: t ? t.dataset.full : null };
  });
  step('4.队列有当前歌', queue.qty >= 1 && !!queue.short, 'qty=' + queue.qty + ' short=' + queue.short);
  await page.evaluate(() => { const t = document.querySelector('#queue-view .q-title'); if (t) t.click(); });
  await page.waitForTimeout(150);
  const qExpand = await page.evaluate(() => {
    const t = document.querySelector('#queue-view .q-title');
    return t ? t.textContent === t.dataset.full : false;
  });
  step('4b.队列标题点击展开全名', qExpand, '');

  // 5. 收藏增删闭环（真实 API）——用 localStorage 里刚播的歌（免再触发 B 站搜索渲染）
  const fav1 = await page.evaluate(async () => {
    let q = { list: [] };
    try { q = JSON.parse(localStorage.getItem('hc:queue') || '{"list":[]}'); } catch (e) {}
    const song = q.list && q.list[0];
    if (!song || !song.bvid) return { err: 'no song in queue', q: q.list };
    const bvid = song.bvid;
    const res = await fetch('/api/v1/fav/list').then(r => r.json());
    const before = res.data.length;
    await fetch('/api/v1/fav/add', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ bvid, title: song.title, artist: song.artist, cover: song.cover }),
    });
    const mid = await fetch('/api/v1/fav/list').then(r => r.json());
    await fetch('/api/v1/fav/' + bvid, { method: 'DELETE' });
    const after = await fetch('/api/v1/fav/list').then(r => r.json());
    return { before, mid: mid.data.length, after: after.data.length };
  });
  step('5.收藏增删闭环(add→list→del)', !fav1.err && fav1.mid === fav1.before + 1 && fav1.after === fav1.before,
    JSON.stringify(fav1));

  // 6. 主题切换持久化
  await page.goto('http://127.0.0.1:28976/hx/settings', { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForTimeout(800);
  const theme1 = await page.evaluate(() => {
    const dark = document.querySelector('[data-theme="dark"]');
    if (dark) dark.click();
    return { stored: localStorage.getItem('hc:theme'), htmlDark: document.documentElement.classList.contains('dark') };
  });
  step('6.主题切dark持久化', theme1.stored === 'dark' && theme1.htmlDark, JSON.stringify(theme1));
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(500);
  const theme2 = await page.evaluate(() => ({
    htmlDark: document.documentElement.classList.contains('dark'),
    stored: localStorage.getItem('hc:theme'),
  }));
  step('6b.刷新后主题保持', theme2.htmlDark && theme2.stored === 'dark', JSON.stringify(theme2));

  // 7. 移动端视图
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('http://127.0.0.1:28976/hx/queue', { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForTimeout(800);
  const mob = await page.evaluate(() => ({
    overflow: document.documentElement.scrollWidth > window.innerWidth + 1,
    dockVisible: !!document.getElementById('player-dock'),
    queueRows: document.querySelectorAll('#queue-view .q-title').length,
    bodyW: document.documentElement.scrollWidth, vw: window.innerWidth,
  }));
  step('7.移动端无横向溢出', !mob.overflow, 'bodyW=' + mob.bodyW + ' vw=' + mob.vw);
  step('7b.移动端dock/队列渲染', mob.queueRows >= 1 && mob.dockVisible, JSON.stringify(mob));

  const realErrors = out.errors.filter(e => !e.includes('cdn.tailwindcss.com') && !e.includes('BVold1'));
  step('8.控制台无真实报错', realErrors.length === 0, JSON.stringify(realErrors.slice(0, 5)));
  return out;
}