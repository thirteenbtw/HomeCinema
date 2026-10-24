// Interface scale on Ctrl + «+» / Ctrl + «−» / Ctrl + 0 and Ctrl + wheel, and a
// separate text size. The desktop window has no browser zoom of its own, and
// in a browser this keeps both across restarts. --zoom lets layouts size
// themselves to the window (var(--vh)), since zoom also scales vh units.
const steps = [0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2];
export const textSteps = [0.85, 1, 1.15, 1.3, 1.5];
const KEY = 'hc.zoom';
const TEXT_KEY = 'hc.textScale';

function read(key: string, allowed: number[]): number {
  try { const z = Number(localStorage.getItem(key)); return allowed.includes(z) ? z : 1; } catch { return 1; }
}
function save(key: string, v: number) {
  try { localStorage.setItem(key, String(v)); } catch { /* private mode */ }
}

let current = read(KEY, steps);
let text = read(TEXT_KEY, textSteps);
let hintTimer = 0;

function hint(label: string) {
  let el = document.getElementById('zoom-hint');
  if (!el) {
    el = document.createElement('div');
    el.id = 'zoom-hint';
    el.setAttribute('role', 'status');
    el.style.cssText = 'position:fixed;top:16px;left:50%;transform:translateX(-50%);z-index:1000;padding:6px 14px;border-radius:999px;background:#111214E6;color:#fff;font:600 13px Inter,sans-serif;pointer-events:none';
    document.body.appendChild(el);
  }
  el.textContent = label;
  el.style.display = '';
  clearTimeout(hintTimer);
  hintTimer = window.setTimeout(() => { el!.style.display = 'none'; }, 1200);
}

function apply(z: number, show: boolean) {
  current = z;
  const root = document.documentElement.style;
  root.zoom = z === 1 ? '' : String(z);
  root.setProperty('--zoom', String(z));
  save(KEY, z);
  if (show) hint(`${Math.round(z * 100)}%`);
}

export function textScale() { return text; }

export function setTextScale(v: number) {
  text = v;
  document.documentElement.style.setProperty('--fs', String(v));
  save(TEXT_KEY, v);
}

function step(dir: number) {
  const i = steps.indexOf(current);
  apply(steps[Math.max(0, Math.min(steps.length - 1, (i < 0 ? steps.indexOf(1) : i) + dir))], true);
}

export function installZoom() {
  apply(current, false);
  setTextScale(text);
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
