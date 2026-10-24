import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useApp } from '../state';
import { tr } from '../i18n';
import { ambientFor, paletteFor } from './Art';
import { Icon } from './Icon';
import { useAmbient } from './Shell';

export type Slide = { key: string; name: string; backdrop?: string; meta: string; badges?: ReactNode; overview?: string; actions: ReactNode };

// Featured carousel from the design: a large backdrop with the title, a few
// actions and dots; rotates every 9 s unless the pointer is over it.
export function Featured({ slides, label }: { slides: Slide[]; label: string }) {
  const { prefs } = useApp();
  const setAmbient = useAmbient();
  const [idx, setIdx] = useState(0);
  const [hover, setHover] = useState(false);
  const [spot, setSpot] = useState({ x: 0, y: 0 });
  const [par, setPar] = useState({ x: 0, y: 0 });
  const raf = useRef(0);
  const live = prefs.bg === 'live';
  const n = slides.length;
  const i = n ? idx % n : 0;
  const s = slides[i];

  useEffect(() => {
    if (n < 2) return;
    const id = window.setInterval(() => { if (!hover) setIdx((x) => (x + 1) % n); }, 9000);
    return () => window.clearInterval(id);
  }, [n, hover]);
  useEffect(() => { if (s) setAmbient(ambientFor(s.name)); return () => setAmbient(null); }, [s?.key]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!s) return null;

  const onMove = (e: React.MouseEvent<HTMLElement>) => {
    setHover(true);
    if (!live) return;
    const r = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - r.left, y = e.clientY - r.top;
    cancelAnimationFrame(raf.current);
    raf.current = requestAnimationFrame(() => {
      setSpot({ x, y });
      setPar({ x: (x / r.width) * 2 - 1, y: (y / r.height) * 2 - 1 });
    });
  };
  const mv = (k: number) => (live ? `translate3d(${(par.x * k).toFixed(1)}px,${(par.y * k).toFixed(1)}px,0)` : 'none');
  const [sky, orb] = paletteFor(s.name);

  return (
    <section aria-label={label} onMouseMove={onMove} onMouseLeave={() => setHover(false)}
      style={{ position: 'relative', overflow: 'hidden', borderRadius: 20, background: sky, minHeight: 'clamp(340px, 42vw, 460px)', display: 'flex', alignItems: 'flex-end', transition: 'background .8s', boxShadow: '0 20px 50px var(--sh)', flex: 'none' }}>
      {s.backdrop ? (
        <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-10) }}>
          <img key={s.backdrop} src={s.backdrop} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover', animation: 'fadein .8s' }} />
        </div>
      ) : (
        <>
          <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-8) }}>
            <div style={{ position: 'absolute', right: -60, top: -140, width: 680, height: 680, borderRadius: '50%', background: '#FFFFFF10' }} />
          </div>
          <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-22) }}>
            <div style={{ position: 'absolute', right: 220, top: 90, width: 280, height: 280, borderRadius: '50%', background: orb, transition: 'background .8s' }} />
          </div>
        </>
      )}
      <div aria-hidden="true" style={{ position: 'absolute', width: 440, height: 440, margin: '-220px 0 0 -220px', left: spot.x, top: spot.y, borderRadius: '50%', background: orb, filter: 'blur(70px)', opacity: live && hover ? 0.45 : 0, mixBlendMode: 'screen', transition: 'opacity .4s', pointerEvents: 'none' }} />
      <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(90deg, #000000B8 0%, #00000073 45%, transparent 75%), linear-gradient(0deg, #00000080 0%, transparent 45%)' }} />

      {n > 1 && (
        <div className="feat-dots" style={{ position: 'absolute', right: 20, bottom: 20, display: 'flex', gap: 8, alignItems: 'center', zIndex: 1 }}>
          {slides.map((x, j) => (
            <button key={x.key} onClick={() => setIdx(j)} aria-label={`${x.name}`} aria-current={j === i} style={{ width: j === i ? 24 : 8, height: 8, borderRadius: 4, border: 0, padding: 0, background: '#FFFFFF', opacity: j === i ? 1 : 0.45, cursor: 'pointer', transition: 'width .3s' }} />
          ))}
          <button onClick={() => setIdx((i + 1) % n)} aria-label={tr('Дальше', 'Next')} style={{ marginLeft: 6, width: 32, height: 32, borderRadius: '50%', border: 0, background: '#FFFFFF2E', color: '#FFFFFF', display: 'grid', placeItems: 'center', cursor: 'pointer' }}>
            <Icon name="right" size={14} />
          </button>
        </div>
      )}

      <div style={{ position: 'relative', padding: 'clamp(24px, 3vw, 40px)', maxWidth: 600, display: 'flex', flexDirection: 'column', gap: 12, color: '#FFFFFF' }}>
        <div className="disp" style={{ fontSize: 'calc(clamp(36px,5vw,60px) * var(--fs))', lineHeight: 0.95, fontWeight: 600, letterSpacing: '.04em', overflowWrap: 'anywhere' }}>{s.name}</div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', fontSize: 'calc(13px * var(--fs))', fontWeight: 600 }}>
          {s.meta}{s.badges}
        </div>
        {s.overview && <p style={{ margin: 0, fontSize: 'calc(14px * var(--fs))', lineHeight: 1.55, color: '#D4D7DC', display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{s.overview}</p>}
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', paddingTop: 6 }}>{s.actions}</div>
      </div>
    </section>
  );
}

export function Badge({ children, accent }: { children: ReactNode; accent?: boolean }) {
  return <span style={{ fontSize: 'calc(11px * var(--fs))', padding: '3px 8px', borderRadius: 999, background: accent ? 'var(--acc)' : '#FFFFFF2E' }}>{children}</span>;
}
