// Interface scale on Ctrl + «+» / Ctrl + «−» / Ctrl + 0 and Ctrl + wheel. The
// desktop window has no browser zoom of its own, and in a browser this keeps
// the scale across restarts.
const steps = [0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2];
const KEY = 'hc.zoom';

function read(): number {
  try { const z = Number(localStorage.getItem(KEY)); return steps.includes(z) ? z : 1; } catch { return 1; }
}

let current = read();
let hintTimer = 0;

function apply(z: number, hint: boolean) {
  current = z;
  document.documentElement.style.zoom = z === 1 ? '' : String(z);
  try { localStorage.setItem(KEY, String(z)); } catch { /* private mode */ }
  if (!hint) return;
  let el = document.getElementById('zoom-hint');
  if (!el) {
    el = document.createElement('div');
    el.id = 'zoom-hint';
    el.setAttribute('role', 'status');
    el.style.cssText = 'position:fixed;top:16px;left:50%;transform:translateX(-50%);z-index:1000;padding:6px 14px;border-radius:999px;background:#111214E6;color:#fff;font:600 13px Inter,sans-serif;pointer-events:none';
    document.body.appendChild(el);
  }
  el.textContent = `${Math.round(z * 100)}%`;
  el.style.display = '';
  clearTimeout(hintTimer);
  hintTimer = window.setTimeout(() => { el!.style.display = 'none'; }, 1200);
}

function step(dir: number) {
  const i = steps.indexOf(current);
  apply(steps[Math.max(0, Math.min(steps.length - 1, (i < 0 ? steps.indexOf(1) : i) + dir))], true);
}

export function installZoom() {
  apply(current, false);
  window.addEventListener('keydown', (e) => {
    if (!(e.ctrlKey || e.metaKey) || e.altKey) return;
    if (e.key === '+' || e.key === '=' || e.code === 'Equal' || e.code === 'NumpadAdd') step(1);
    else if (e.key === '-' || e.code === 'Minus' || e.code === 'NumpadSubtract') step(-1);
    else if (e.key === '0' || e.code === 'Digit0' || e.code === 'Numpad0') apply(1, true);
    else return;
    e.preventDefault();
  });
  window.addEventListener('wheel', (e) => {
    if (!e.ctrlKey) return;
    e.preventDefault();
    step(e.deltaY < 0 ? 1 : -1);
  }, { passive: false });
}
