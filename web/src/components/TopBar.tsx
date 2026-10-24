import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useApp } from '../state';
import { t, tr } from '../i18n';
import { Icon } from './Icon';

// Header from the design: back/forward, glass search with type filter, live
// background toggle and the current TV. On Home the search goes to the
// catalog (all of TMDB) instead of the local library.
export function TopBar({ types = true, catalog = false }: { types?: boolean; catalog?: boolean }) {
  const nav = useNavigate();
  const { prefs, setPrefs, devices, cast } = useApp();
  const [params] = useSearchParams();
  const [q, setQ] = useState(params.get('q') || '');
  const [kind, setKind] = useState(params.get('kind') || '');
  const input = useRef<HTMLInputElement>(null);
  const bgOn = prefs.bg !== 'off';
  const playing = cast[0];
  const tv = playing ? playing.deviceName : devices.find((d) => d.online)?.name;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); input.current?.focus(); }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => { if (catalog) setQ(params.get('q') || ''); }, [catalog, params]);

  const go = (k = kind, text = q) => {
    if (catalog) { nav(text.trim() ? `/?q=${encodeURIComponent(text.trim())}` : '/'); return; }
    if (!text.trim()) return;
    nav(`/search?q=${encodeURIComponent(text.trim())}${k ? '&kind=' + k : ''}`);
  };

  return (
    <header style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 12, paddingTop: 16 }}>
      <div style={{ flex: '1 1 120px', display: 'flex', gap: 4 }}>
        <button className="ri" aria-label={t('back')} onClick={() => nav(-1)}><Icon name="left" /></button>
        <button className="ri" aria-label={t('forward')} onClick={() => nav(1)}><Icon name="right" /></button>
      </div>
      <div style={{ flex: '0 1 520px', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
        <form role="search" className="glass" onSubmit={(e) => { e.preventDefault(); go(); }}
          style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 10, height: 44, padding: '0 16px', borderRadius: 999, border: '1px solid var(--line)', boxShadow: '0 6px 20px var(--sh)', color: 'var(--mut)' }}>
          <Icon name="search" size={18} />
          <label className="sr" htmlFor="q">{tr('Поиск', 'Search')}</label>
          <input id="q" ref={input} value={q} onChange={(e) => setQ(e.target.value)} placeholder={catalog ? tr('Найти фильм или сериал', 'Find a movie or show') : t('searchPh')}
            style={{ flex: 1, minWidth: 0, border: 0, outline: 'none', background: 'transparent', font: "500 calc(14px * var(--fs)) 'Inter',sans-serif", color: 'var(--fg)' }} />
          {catalog && q ? <button type="button" className="ri" style={{ width: 28, height: 28 }} aria-label={tr('Очистить', 'Clear')} onClick={() => { setQ(''); nav('/'); }}><Icon name="close" size={14} /></button>
            : <span className="mono" style={{ fontSize: 'calc(11px * var(--fs))' }}>Ctrl K</span>}
        </form>
        {types && !catalog && (
          <div role="group" aria-label={tr('Тип', 'Type')} className="glass" style={{ display: 'flex', gap: 4, padding: 4, borderRadius: 999, border: '1px solid var(--line)', boxShadow: '0 6px 20px var(--sh)' }}>
            {[['', t('all')], ['movie', t('movies')], ['show', t('shows')], ['people', t('people')]].map(([k, label]) => (
              <button key={k} className={'pill' + (kind === k ? ' on' : ' ghost')} aria-pressed={kind === k}
                onClick={() => { setKind(k); if (q.trim()) go(k); }}>{label}</button>
            ))}
          </div>
        )}
      </div>
      <div style={{ flex: '1 1 120px', display: 'flex', justifyContent: 'flex-end', gap: 6, flexWrap: 'wrap' }}>
        <button className="pill" onClick={() => setPrefs({ bg: bgOn ? 'off' : 'live' })} aria-pressed={bgOn} style={{ height: 40 }}>
          <Icon name="sparkle" size={16} style={{ color: 'var(--acct)' }} />{bgOn ? t('liveBg') : t('bgOff')}
        </button>
        <Link className="pill" to="/devices" style={{ height: 40 }}>
          <Icon name="cast" size={16} style={{ color: 'var(--acct)' }} />{tv || t('devices')}
        </Link>
      </div>
    </header>
  );
}
