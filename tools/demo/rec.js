const { chromium } = require('playwright');
(async () => {
  const b = await chromium.launch();
  const ctx = await b.newContext({ viewport: { width: 1280, height: 800 }, locale: 'ru-RU', recordVideo: { dir: 'vid', size: { width: 1280, height: 800 } } });
  await ctx.addInitScript(() => { try { localStorage.setItem('hc.prefs', JSON.stringify({ theme: 'dark' })); } catch {} });
  const p = await ctx.newPage();
  const scroll = async (dy, ms = 900) => { await p.evaluate(async ([dy, ms]) => { const y0 = scrollY, t0 = performance.now(); await new Promise(r => { const f = t => { const k = Math.min(1, (t - t0) / ms); const e = k < .5 ? 2 * k * k : 1 - Math.pow(-2 * k + 2, 2) / 2; scrollTo(0, y0 + dy * e); k < 1 ? requestAnimationFrame(f) : r(); }; requestAnimationFrame(f); }); }, [dy, ms]); };
  await p.goto('http://127.0.0.1:8096/'); await p.waitForTimeout(2200);
  const next = p.getByRole('button', { name: 'Дальше' });
  await next.click(); await p.waitForTimeout(1600);
  await next.click(); await p.waitForTimeout(1600);
  await scroll(620); await p.waitForTimeout(700);
  let target = p.locator('a[href*="/catalog/movie/105"]:visible').first();
  let box = await target.boundingBox();
  await scroll(box.y - 330); await p.waitForTimeout(400);
  box = await target.boundingBox();
  await p.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 20 });
  await target.hover(); await p.waitForTimeout(2400);
  await target.click(); await p.waitForTimeout(2300);
  await scroll(420); await p.waitForTimeout(1600);
  await scroll(-420, 600); await p.waitForTimeout(400);
  
  await p.getByText('Есть в библиотеке').first().click({timeout:4000}).catch(e=>console.log('noclick')); await p.waitForTimeout(1200);
  await p.waitForTimeout(800); await p.getByRole('link', { name: /Продолжить/ }).first().click().catch(()=>console.log('nocont')); await p.waitForTimeout(1200);
  await p.evaluate(() => { const v = document.querySelector('video'); if (v) { v.currentTime = 3; v.play().catch(()=>{}); } });
  await p.mouse.move(640, 420, { steps: 5 }); await p.waitForTimeout(3500);
  await ctx.close(); await b.close();
})();
