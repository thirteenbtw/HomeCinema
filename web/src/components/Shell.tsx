import { Component, createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useApp } from '../state';
import { t, tr } from '../i18n';
import { Icon } from './Icon';
import { CastDialog } from './CastDialog';
import { installDragScroll } from './dragScroll';
import { installTorrentDrop } from './torrentDrop';

// Pages tint the ambient background (e.g. with the featured poster's colors).
const AmbientCtx = createContext<(colors: string[] | null) => void>(() => {});
export const useAmbient = () => useContext(AmbientCtx);

const themeOrder = ['light', 'dark', 'oled'] as const;
const themeLabels = { light: 'Светлая', dark: 'Тёмная', oled: 'OLED' };

export function Shell() {
  const { prefs, setPrefs, profile, torrents } = useApp();
  const [colors, setColors] = useState<string[] | null>(null);
  const [n, setN] = useState({ x: 0, y: 0 });
  const raf = useRef(0);
  const pending = useRef<{ x: number; y: number } | null>(null);
  const loc = useLocation();
  const nav = useNavigate();
  const live = prefs.bg === 'live' && !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const bgOn = prefs.bg !== 'off';

  useEffect(() => () => cancelAnimationFrame(raf.current), []);
  useEffect(() => installDragScroll(), []);
  useEffect(() => installTorrentDrop(() => nav('/torrents')), [nav]);
  useEffect(() => { window.scrollTo(0, 0); }, [loc.pathname]);

  const onMove = (e: React.MouseEvent) => {
    if (!live) return;
    pending.current = { x: (e.clientX / window.innerWidth) * 2 - 1, y: (e.clientY / window.innerHeight) * 2 - 1 };
    if (raf.current) return;
    raf.current = requestAnimationFrame(() => { raf.current = 0; if (pending.current) setN(pending.current); });
  };
  const mv = (k: number) => (live ? `translate3d(${(n.x * k).toFixed(1)}px,${(n.y * k).toFixed(1)}px,0)` : 'none');
  const c = (prefs.posterTint && colors) || ['var(--acc)', '#2D6FB5', '#E0832A'];
  const active = torrents?.items.filter((x) => ['downloading', 'streaming', 'meta'].includes(x.status)).length || 0;
  const theme = prefs.theme;

  return (
    <AmbientCtx.Provider value={setColors}>
      <div className={`app th-${theme} ac-${prefs.accent}`} onMouseMove={onMove}>
        {bgOn && (
          <div aria-hidden="true" style={{ position: 'fixed', inset: 0, pointerEvents: 'none', opacity: `calc(var(--amb) * ${(prefs.intensity / 100).toFixed(2)})`, overflow: 'hidden' }}>
            <div className="blob" style={{ width: 620, height: 620, left: '18%', top: -220, background: c[0], transform: mv(60) }} />
            <div className="blob" style={{ width: 520, height: 520, right: -120, top: 380, background: c[1], transform: mv(-80) }} />
            <div className="blob" style={{ width: 560, height: 560, left: -160, top: 760, background: c[2], transform: mv(40) }} />
          </div>
        )}
        <nav aria-label={tr('Разделы', 'Sections')} style={{ flex: '0 0 64px', padding: '16px 10px', borderRight: '1px solid var(--line)', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4, position: 'sticky', top: 0, height: 'var(--vh)', overflowY: 'auto', overflowX: 'hidden', scrollbarWidth: 'none', zIndex: 2 }}>
          <Link to="/" aria-label="HomeCinema" className="nav-extra" style={{ width: 32, height: 32, borderRadius: 9, background: 'var(--acc)', display: 'grid', placeItems: 'center', marginBottom: 16 }}>
            <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path d="M8 5l11 7-11 7z" fill="#FFFFFF" /></svg>
          </Link>
          <Nav to="/" icon="home" label={t('home')} end />
          <Nav to="/library/movies" icon="film" label={t('movies')} />
          <Nav to="/library/shows" icon="tv" label={t('shows')} />
          <Nav to="/catalog" icon="compass" label={t('catalog')} />
          <div aria-hidden="true" className="nav-extra" style={{ width: 24, height: 1, background: 'var(--line)', margin: '8px 0' }} />
          <Nav to="/torrents" icon="download" label={active ? `${t('torrents')}, ${active} ${tr('активных', 'active')}` : t('torrents')} dot={active > 0} />
          <Nav to="/devices" icon="cast" label={t('devices')} />
          <Nav to="/library/favorites" icon="heart" label={t('favorites')} extra />
          <Nav to="/library/later" icon="clock" label={t('later')} extra />
          <div style={{ marginTop: 'auto', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8 }} className="nav-bottom">
            <button className="ri nav-extra" onClick={() => setPrefs({ theme: themeOrder[(themeOrder.indexOf(theme) + 1) % 3] })}
              aria-label={`${t('theme')}: ${themeLabels[theme]}. ${tr('Переключить', 'Switch')}`} title={`${t('theme')}: ${themeLabels[theme]}`}>
              <Icon name="theme" />
            </button>
            <Nav to="/settings" icon="sliders" label={t('settings')} />
            <Link to="/profiles" aria-label={`${t('profile')}: ${profile?.name || ''}`} className="nav-extra"
              style={{ width: 32, height: 32, borderRadius: '50%', background: profile?.color || 'var(--acc)', color: '#FFFFFF', font: "700 calc(13px * var(--fs)) 'Inter',sans-serif", display: 'grid', placeItems: 'center' }}>
              {(profile?.name || '?').slice(0, 1).toUpperCase()}
            </Link>
          </div>
        </nav>
        <PageBoundary key={loc.pathname}><Outlet /></PageBoundary>
        <CastDialog />
      </div>
    </AmbientCtx.Provider>
  );
}

// PageBoundary keeps a crash in one page from blanking the whole app; it
// resets when the user navigates elsewhere.
class PageBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };
  static getDerivedStateFromError(error: Error) { return { error }; }
  componentDidCatch(error: Error) { console.error(error); }
  render() {
    if (!this.state.error) return this.props.children;
    return (
      <main className="main pad">
        <h1 className="h1">{tr('Что-то пошло не так', 'Something went wrong')}</h1>
        <div className="err">{this.state.error.message}</div>
        <div className="row"><button className="btn btn-l sm" onClick={() => this.setState({ error: null })}>{tr('Попробовать снова', 'Try again')}</button></div>
      </main>
    );
  }
}

function Nav({ to, icon, label, end, dot, extra }: { to: string; icon: string; label: string; end?: boolean; dot?: boolean; extra?: boolean }) {
  return (
    <NavLink to={to} end={end} className={({ isActive }) => 'ri' + (isActive ? ' on' : '') + (extra ? ' nav-extra' : '')} aria-label={label} title={label}>
      <Icon name={icon} />
      {dot && <span style={{ position: 'absolute', top: 8, right: 8, width: 8, height: 8, borderRadius: '50%', background: 'var(--acc)', border: '2px solid var(--bg)' }} />}
    </NavLink>
  );
}

export function Section({ title, note, more, children, id }: { title: string; note?: string; more?: string; children: ReactNode; id?: string }) {
  return (
    <section aria-labelledby={id} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div style={{ display: 'flex', alignItems: 'flex-end', gap: 12 }}>
        <div>
          <h2 id={id} className="h2">{title}</h2>
          {note && <div className="sub">{note}</div>}
        </div>
        {more && <Link to={more} style={{ marginLeft: 'auto', fontSize: 'calc(13px * var(--fs))', fontWeight: 600 }}>{t('seeAll')}</Link>}
      </div>
      {children}
    </section>
  );
}
