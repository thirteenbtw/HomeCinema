const { chromium } = require('playwright');
(async () => {
  const b = await chromium.launch();
  const p = await b.newPage({ viewport: { width: 1280, height: 420 }, deviceScaleFactor: 2 });
  await p.goto('http://127.0.0.1:8096/'); await p.waitForTimeout(1500);
  await p.evaluate(() => {
    const ps = [105, 106, 101, 201, 115, 103];
    document.body.innerHTML = `
<div style="position:fixed;inset:0;background:#141A33;overflow:hidden;font-family:Inter,sans-serif">
  <div style="position:absolute;inset:0;background-image:radial-gradient(#A5B4FC33 1.2px,transparent 1.3px);background-size:22px 22px"></div>
  <div style="position:absolute;left:-120px;top:-160px;width:620px;height:620px;border-radius:50%;background:radial-gradient(#5B6EE155,transparent 65%)"></div>
  <div style="position:absolute;left:72px;top:92px;display:flex;flex-direction:column;gap:22px;color:#E7ECF7">
    <div style="display:flex;align-items:center;gap:26px">
      <span style="width:104px;height:104px;border-radius:28px;background:#A5B4FC;display:grid;place-items:center">
        <svg viewBox="0 0 64 64" width="73" height="73"><circle cx="30" cy="36" r="15" fill="none" stroke="#141A33" stroke-width="11"/><rect x="43" y="5" width="14" height="14" rx="2" fill="#141A33" transform="rotate(45 50 12)"/></svg>
      </span>
      <style>.wm .i::after{top:.075em!important}</style><span class="wm hot" style="font-size:132px;color:#E7ECF7">r<span class="i">ı</span>v<span class="o"></span></span>
    </div>
    <div style="font-family:'Inter Tight',Inter,sans-serif;font-weight:800;font-size:40px;letter-spacing:-.03em">Your movies. Any screen.</div>
    <div style="font-size:19px;color:#A5B4FC;max-width:520px;line-height:1.45">Домашний медиасервер для Windows: библиотека, плеер, Google Cast и торренты в одном приложении</div>
  </div>
  <div style="position:absolute;right:10px;top:40px;width:640px;height:420px">
    ${ps.map((id, i) => `<img src="/api/catalog-img/w342/p${id}.jpg" style="position:absolute;width:170px;height:255px;border-radius:14px;box-shadow:0 24px 48px #0008;left:${i * 92}px;top:${[60, 20, 70, 10, 55, 25][i]}px;transform:rotate(${[-8, -3, 4, -2, 6, 2][i]}deg);outline:1px solid #ffffff14">`).join('')}
  </div>
</div>`;
  });
  await p.waitForTimeout(1500);
  await p.screenshot({ path: 'shots/banner.png' });
  await b.close();
})();
