import { useEffect, useRef, type CSSProperties } from 'react';

// Dotted background: a quiet grid of dots over the theme color. "live" lights
// the dots around the cursor in the accent (or poster) color; "static" is the
// plain grid. Intensity scales how visible the dots are.
export function Dots({ mode, intensity, colors }: { mode: 'live' | 'static'; intensity: number; colors: string[] }) {
  const ref = useRef<HTMLDivElement>(null);
  const live = mode === 'live' && !window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  useEffect(() => {
    const el = ref.current;
    if (!live || !el) return;
    let raf = 0;
    let x = 0, y = 0;
    const onMove = (e: MouseEvent) => {
      x = e.clientX; y = e.clientY;
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        const z = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--zoom')) || 1;
        el.style.setProperty('--mx', `${x / z}px`);
        el.style.setProperty('--my', `${y / z}px`);
        el.style.setProperty('--glow', '1');
      });
    };
    const onLeave = () => el.style.setProperty('--glow', '0');
    window.addEventListener('mousemove', onMove);
    document.addEventListener('mouseleave', onLeave);
    return () => { cancelAnimationFrame(raf); window.removeEventListener('mousemove', onMove); document.removeEventListener('mouseleave', onLeave); };
  }, [live]);

  const style = { '--k': (intensity / 100).toFixed(2), '--t1': colors[0], '--t2': colors[1] || colors[0] } as CSSProperties;
  return (
    <div ref={ref} aria-hidden="true" className={`dots ${live ? 'dots-live' : 'dots-static'}`} style={style}>
      <div className="dots-base" />
      {live && <div className="dots-tint dots-t1" />}
    </div>
  );
}
