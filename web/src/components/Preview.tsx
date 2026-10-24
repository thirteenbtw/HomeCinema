import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from 'react';
import { createPortal } from 'react-dom';
import { Link, useLocation } from 'react-router-dom';
import type { CatalogItem } from '../api';
import { t, tr } from '../i18n';
import { PlayGlyph } from './Icon';
import { MuteButton, YouTubeCover, useTrailer } from './Trailer';

// Hover preview from the design: rest the pointer on a poster and a larger
// card grows out of it with the still, then the muted trailer, a short
// description and the main actions.

type Box = { left: number; top: number; width: number; height: number };
type State = { x: CatalogItem; box: Box } | null;

let state: State = null;
let openTimer = 0;
let closeTimer = 0;
const subs = new Set<() => void>();
const set = (s: State) => { state = s; subs.forEach((f) => f()); };
const subscribe = (f: () => void) => { subs.add(f); return () => subs.delete(f); };

const canHover = () => window.matchMedia('(hover: hover) and (pointer: fine)').matches;
const zoom = () => parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--zoom')) || 1;

function close(delay = 120) {
  window.clearTimeout(openTimer);
  window.clearTimeout(closeTimer);
  closeTimer = window.setTimeout(() => set(null), delay);
}

// previewProps goes on a poster link.
export function previewProps(x: CatalogItem) {
  return {
    onMouseEnter: (e: React.MouseEvent<HTMLElement>) => {
      if (!canHover()) return;
      const link = e.currentTarget;
      const el = link.querySelector('.pimg') || link;
      window.clearTimeout(openTimer);
      window.clearTimeout(closeTimer);
      openTimer = window.setTimeout(() => {
        if (!link.isConnected || !link.matches(':hover')) return;
        const r = el.getBoundingClientRect();
        const z = zoom();
        set({ x, box: { left: r.left / z, top: r.top / z, width: r.width / z, height: r.height / z } });
      }, 650);
    },
    onMouseLeave: () => close(),
    onClick: () => close(0),
  };
}

export function PreviewLayer() {
  const s = useSyncExternalStore(subscribe, () => state);
  const loc = useLocation();
  useEffect(() => { set(null); }, [loc.pathname, loc.search]);
  useEffect(() => {
    const off = () => close(0);
    window.addEventListener('scroll', off, true);
    window.addEventListener('blur', off);
    return () => { window.removeEventListener('scroll', off, true); window.removeEventListener('blur', off); };
  }, []);
  if (!s) return null;
  return createPortal(<Card key={s.x.kind + s.x.id} x={s.x} box={s.box} />, document.querySelector('.app') || document.body);
}

function Card({ x, box }: { x: CatalogItem; box: Box }) {
  const ref = useRef<HTMLDivElement>(null);
  const [muted, setMuted] = useState(true);
  const [playing, setPlaying] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const key = useTrailer({ kind: x.kind, id: x.id }, true, 300);
  const width = Math.min(380, Math.max(300, box.width * 1.9));
  const link = `/catalog/${x.kind}/${x.id}`;

  // Center on the poster, inside the window.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const z = zoom();
    const vw = window.innerWidth / z, vh = window.innerHeight / z;
    const h = el.offsetHeight;
    const left = Math.min(Math.max(12, box.left + box.width / 2 - width / 2), vw - width - 12);
    const top = Math.min(Math.max(12, box.top + box.height / 2 - h / 2), vh - h - 12);
    setPos({ left, top });
  }, [box.left, box.top, box.width, box.height, width]);

  const origin = pos ? `${box.left + box.width / 2 - pos.left}px ${box.top + box.height / 2 - pos.top}px` : 'center';
  return (
    <div ref={ref} className="pv" role="dialog" aria-label={x.title}
      onMouseEnter={() => window.clearTimeout(closeTimer)} onMouseLeave={() => close()}
      style={{ width, left: pos?.left ?? -9999, top: pos?.top ?? 0, transformOrigin: origin, visibility: pos ? 'visible' : 'hidden' }}>
      <Link to={link} className="pv-media" tabIndex={-1} onClick={() => close(0)}>
        {x.backdrop ? <img src={x.backdrop} alt="" /> : x.poster ? <img src={x.poster} alt="" style={{ objectFit: 'contain' }} /> : null}
        {key && <YouTubeCover ytKey={key} muted={muted} onPlaying={() => setPlaying(true)} />}
        {playing && <MuteButton muted={muted} onToggle={() => setMuted(!muted)} style={{ position: 'absolute', right: 10, bottom: 10, zIndex: 2 }} />}
      </Link>
      <div style={{ padding: '12px 16px 16px', display: 'flex', flexDirection: 'column', gap: 8 }}>
        <div className="disp" style={{ fontSize: 'calc(22px * var(--fs))', fontWeight: 600, lineHeight: 1, letterSpacing: '.03em' }}>{x.title}</div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap', fontSize: 'calc(12px * var(--fs))', fontWeight: 600, color: 'var(--mut)' }}>
          {x.rating > 0 && <span style={{ padding: '2px 7px', borderRadius: 4, background: x.rating >= 7 ? '#2F9E62' : 'var(--chip)', color: x.rating >= 7 ? '#FFFFFF' : 'var(--fg)' }}>★ {x.rating.toFixed(1)}</span>}
          <span>{[x.year || '', x.kind === 'tv' ? tr('Сериал', 'Show') : tr('Фильм', 'Movie')].filter(Boolean).join(' · ')}</span>
          {x.origTitle && x.origTitle !== x.title && <span style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: '100%' }}>{x.origTitle}</span>}
        </div>
        {x.overview && <p style={{ margin: 0, fontSize: 'calc(13px * var(--fs))', lineHeight: 1.5, color: 'var(--mut2)', display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{x.overview}</p>}
        <div style={{ display: 'flex', gap: 8, paddingTop: 4 }}>
          <Link className="btn btn-r sm" to={link} onClick={() => close(0)}><PlayGlyph />{t('watch')}</Link>
          <Link className="btn btn-l sm" to={link} onClick={() => close(0)}>{t('details')}</Link>
        </div>
      </div>
    </div>
  );
}
