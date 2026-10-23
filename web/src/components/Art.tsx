// Poster / backdrop with the design's abstract fallback (dark ground + a soft
// disc) for titles that have no TMDB artwork yet.
import type { CSSProperties, ReactNode } from 'react';

const palettes: [string, string][] = [
  ['#1C2420', '#2E5E4E'], ['#221A1E', '#8E3B4F'], ['#241D14', '#C97A2B'], ['#121A26', '#2D5B8C'],
  ['#1F1F27', '#6E5FB0'], ['#1A2124', '#7FA6B3'], ['#17140F', '#C8232D'], ['#2A1E24', '#5A2F41'],
  ['#1F2A1E', '#3C5A35'], ['#25221A', '#6B5426'], ['#1E2A33', '#2F4A5C'], ['#15201B', '#4E8F5E'],
];

export function hashOf(s: string) {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return Math.abs(h);
}

export function paletteFor(title: string): [string, string] {
  return palettes[hashOf(title) % palettes.length];
}

// ambientFor gives three background tints for a title (hero, ambient blobs).
export function ambientFor(title: string): string[] {
  const h = hashOf(title);
  const p = palettes[h % palettes.length];
  const q = palettes[(h >> 3) % palettes.length];
  return [p[1], q[1], palettes[(h >> 6) % palettes.length][1]];
}

type Props = {
  src?: string; title: string; style?: CSSProperties; label?: ReactNode; disc?: 'poster' | 'wide' | 'hero'; children?: ReactNode; className?: string;
};

export function Art({ src, title, style, label, disc = 'poster', children, className }: Props) {
  const [bg, fg] = paletteFor(title);
  const top = ['35%', '-20%', '45%', '5%', '30%', '-10%', '20%'][hashOf(title) % 7];
  const discStyle: CSSProperties = disc === 'poster'
    ? { width: '130%', left: '-15%', top }
    : { width: '60%', left: '30%', top: '-20%' };
  return (
    <div className={className} style={{ position: 'relative', overflow: 'hidden', background: bg, ...style }}>
      {src ? (
        <img src={src} alt="" loading="lazy" style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover' }} />
      ) : (
        <div aria-hidden="true" style={{ position: 'absolute', aspectRatio: '1', borderRadius: '50%', background: fg, ...discStyle }} />
      )}
      {label}
      {children}
    </div>
  );
}

export function Bar({ pct, style }: { pct: number; style?: CSSProperties }) {
  return (
    <div className="bar" style={style}>
      <div style={{ width: `${Math.max(0, Math.min(100, pct * 100)).toFixed(1)}%` }} />
    </div>
  );
}
