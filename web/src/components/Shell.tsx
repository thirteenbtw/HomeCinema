import { Component, createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useApp } from '../state';
import { t, tr } from '../i18n';
import { Icon } from './Icon';
import { AppMark } from './Brand';
import { CastDialog } from './CastDialog';
import { Dots } from './Dots';
import { PreviewLayer } from './Preview';
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
  const loc = useLocation();
  const nav = useNavigate();
  const bgOn = prefs.bg !== 'off';

  useEffect(() => installDragScroll(), []);
  useEffect(() => installTorrentDrop(() => nav('/torrents')), [nav]);
  useEffect(() => { window.scrollTo(0, 0); }, [loc.pathname]);

  const c = (prefs.posterTint && colors) || ['var(--acc)', 'var(--acc)'];
  const active = torrents?.items.filter((x) => ['downloading', 'streaming', 'meta'].includes(x.status)).length || 0;
  const theme = prefs.theme;
  // The page scrollbar belongs to <html>, so it needs the theme tokens too.
  useEffect(() => {
    const cl = document.documentElement.classList;
    const add = [`th-${theme}`, `ac-${prefs.accent}`];
    cl.add(...add);
    return () => cl.remove(...add);
  }, [theme, prefs.accent]);

  return (
    <AmbientCtx.Provider value={setColors}>
      <div className={`app th-${theme} ac-${prefs.accent}`}>
        {bgOn && <Dots mode={prefs.bg === 'static' ? 'static' : 'live'} intensity={prefs.intensity} colors={c} />}
        <nav aria-label={tr('Разделы', 'Sections')} style={{ flex: '0 0 64px', padding: '16px 10px', borderRight: '1px solid var(--line)', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4, position: 'sticky', top: 0, height: 'var(--vh)', overflowY: 'auto', overflowX: 'hidden', scrollbarWidth: 'none', zIndex: 2 }}>
          <Link to="/" aria-label="Rivo" className="nav-extra" style={{ display: 'grid', marginBottom: 16 }}>
            <AppMark size={34} />
          </Link>
          <Nav to="/" icon="home" label={t('home')} end />
          <Nav to="/library/movies" icon="film" label={t('movies')} />
          <Nav to="/library/shows" icon="tv" label={t('shows')} />
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
        <PreviewLayer />
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
