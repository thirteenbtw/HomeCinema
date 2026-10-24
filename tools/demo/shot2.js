const { chromium } = require('playwright');
const B = 'http://127.0.0.1:8096';
(async () => {
  const b = await chromium.launch();
  async function ctxFor(theme) {
    const ctx = await b.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, locale: 'ru-RU' });
    await ctx.addInitScript(t => { try { localStorage.setItem('hc.prefs', JSON.stringify({ theme: t })); } catch {} }, theme);
    return ctx;
  }
  const jobs = JSON.parse(process.argv[2]);
  for (const [theme, path, name, action] of jobs) {
    const ctx = await ctxFor(theme); const p = await ctx.newPage();
    await p.goto(B + path); await p.waitForTimeout(2500);
    if (action) await eval(action);
    await p.screenshot({ path: `shots/${name}.png` });
    await ctx.close();
  }
  await b.close();
})();
