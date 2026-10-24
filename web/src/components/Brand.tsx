// Rivo identity: the symbol (a ring with a tilted-square spark) and the
// wordmark «rıvo». Values follow the brand sheet (RivoID).
import type { CSSProperties } from 'react';

/** Ring + spark. `spark` colors the square; pass `ring` for the ring. */
export function Symbol({ size = 24, ring = '#141A33', spark = '#141A33', heavy, style }: { size?: number; ring?: string; spark?: string; heavy?: boolean; style?: CSSProperties }) {
  // Small sizes get a thicker ring and a bigger spark, as in the icon grid.
  return (
    <svg viewBox="0 0 64 64" width={size} height={size} aria-hidden="true" style={style}>
      {heavy
        ? <><circle cx="30" cy="36" r="15" fill="none" stroke={ring} strokeWidth="11" /><rect x="43" y="5" width="14" height="14" rx="2" fill={spark} transform="rotate(45 50 12)" /></>
        : <><circle cx="30" cy="36" r="16" fill="none" stroke={ring} strokeWidth="9" /><rect x="44" y="6" width="12" height="12" rx="2" fill={spark} transform="rotate(45 50 12)" /></>}
    </svg>
  );
}

/** App-icon tile: lavender square with the night symbol. */
export function AppMark({ size = 32 }: { size?: number }) {
  return (
    <span style={{ width: size, height: size, borderRadius: size * 0.27, background: '#A5B4FC', display: 'grid', placeItems: 'center', flex: 'none' }}>
      <Symbol size={size * 0.7} heavy />
    </span>
  );
}

/** «rıvo» wordmark; `hot` makes the dot mint. Size comes from font-size. */
export function Wordmark({ hot, style }: { hot?: boolean; style?: CSSProperties }) {
  return <span className={hot ? 'wm hot' : 'wm'} style={style} role="img" aria-label="Rivo">r<span className="i">ı</span>v<span className="o" /></span>;
}
