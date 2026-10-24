import { useEffect, useRef } from 'react';
import { Link } from 'react-router-dom';
import { api, fmtLeft, type Item, type ScanStatus } from '../api';
import { useApp, useAsync } from '../state';
import { t, tr } from '../i18n';
import { Art } from './Art';
import { Icon, PlayGlyph } from './Icon';
import { Section } from './Shell';
import { Badge, type Slide } from './Featured';

// Pieces of the local library shown on Home (continue watching) and on the
// Movies / Shows tabs (featured titles, scan status).

export type HomeData = { featured: Item[]; continue: Item[]; popular: Item[]; recent: Item[]; genres: string[]; scan: ScanStatus };

export const itemLink = (it: Item) => (it.kind === 'episode' && it.showId ? `/show/${it.showId}` : `/movie/${it.id}`);
export const itemName = (it: Item) => (it.kind === 'episode' ? it.showTitle || it.title : it.title);

// useHomeData loads /api/home and refreshes it when a scan adds titles or
// playback progress changes elsewhere.
export function useHomeData() {
  const { scan, lastProgress } = useApp();
  const res = useAsync(() => api.get<HomeData>('/api/home'), []);
  const lastAdded = useRef(0);
  useEffect(() => {
    if (scan && (scan.added !== lastAdded.current || (!scan.scanning && scan.matching === 0))) {
      lastAdded.current = scan.added;
      res.reload();
    }
  }, [scan?.added, scan?.scanning, scan?.matching]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (lastProgress) res.reload(); }, [lastProgress?.itemId]); // eslint-disable-line react-hooks/exhaustive-deps
  return res;
}

// localSlides turns featured library items into carousel slides.
export function localSlides(items: Item[], openCast: (x: { fileId?: number; title: string }) => void): Slide[] {
  return items.map((film) => {
    const fileId = film.files?.[0]?.id;
    const isNew = Date.now() / 1000 - film.addedAt < 7 * 86400;
    return {
      key: String(film.id), name: itemName(film), backdrop: film.backdrop, overview: film.overview,
      meta: [film.year || '', ...film.genres.slice(0, 2)].filter(Boolean).join(' · '),
      badges: <>
        {isNew && <Badge accent>{film.source === 'torrent' ? tr('Из торрента', 'From torrent') : tr('Новое', 'New')}</Badge>}
        {film.quality && <Badge>{film.quality}</Badge>}
      </>,
      actions: <>
        {fileId && <Link className="btn btn-w" to={`/play/${fileId}?item=${film.id}`}><PlayGlyph />{film.progress && !film.progress.watched && film.progress.position > 30 ? t('continue') : t('watch')}</Link>}
        {fileId && <button className="btn btn-g" onClick={() => openCast({ fileId, title: film.title })}><Icon name="cast" size={18} />{t('toTv')}</button>}
        <Link className="btn btn-g" to={itemLink(film)} style={{ color: '#FFFFFF' }}>{t('details')}</Link>
      </>,
    };
  });
}

export function ContinueSection({ items }: { items: Item[] }) {
  if (!items.length) return null;
  return (
    <Section id="cont" title={t('continueWatching')} note={t('syncNote')}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(240px,1fr))', gap: 12 }}>
        {items.slice(0, 8).map((c) => {
          const p = c.progress!;
          const pct = p.duration ? p.position / p.duration : 0;
          const sub = c.kind === 'episode' ? `С${c.season} · Э${c.episode}${c.epTitle ? ' · ' + c.epTitle : ''}` : `${tr('Фильм', 'Movie')}${c.year ? ' · ' + c.year : ''}`;
          return (
            <Link key={c.id} to={`/play/${c.files?.[0]?.id ?? ''}?item=${c.id}`} state={{ itemId: c.id }} style={{ display: 'flex', flexDirection: 'column', gap: 8, color: 'var(--fg)' }}
              onClick={(e) => { if (!c.files?.length) { e.preventDefault(); } }}>
              <Art src={c.backdrop} title={itemName(c)} disc="wide" style={{ aspectRatio: '16/9', borderRadius: 12 }}
                label={<>
                  {c.backdrop && <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(transparent 45%, #000000B0)' }} />}
                  <span className="disp" style={{ position: 'absolute', left: 12, bottom: 14, right: 12, fontSize: 'calc(20px * var(--fs))', fontWeight: 600, color: '#FFFFFF', lineHeight: 1 }}>{itemName(c)}</span>
                  <span style={{ position: 'absolute', right: 10, top: 10, fontSize: 'calc(11px * var(--fs))', fontWeight: 700, padding: '3px 8px', borderRadius: 999, background: '#000000A0', color: '#FFFFFF' }}>
                    {c.progress?.device === 'tv' ? tr('ТВ', 'TV') : tr('ПК', 'PC')}
                  </span>
                  <div style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: 4, background: '#FFFFFF33' }}><div style={{ height: 4, width: `${pct * 100}%`, background: 'var(--acc)' }} /></div>
                </>} />
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, fontSize: 'calc(12px * var(--fs))' }}>
                <span style={{ fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{sub}</span>
                <span style={{ color: 'var(--mut)', whiteSpace: 'nowrap' }}>{fmtLeft(p.duration - p.position, t)}</span>
              </div>
            </Link>
          );
        })}
      </div>
    </Section>
  );
}

export function ScanBar({ st }: { st: ScanStatus }) {
  const pct = st.total ? st.done / st.total : 1;
  return (
    <div className="glass" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 14, padding: '12px 16px', borderRadius: 12, border: '1px solid var(--line)', fontSize: 'calc(12px * var(--fs))', color: 'var(--mut2)' }}>
      <Icon name="refresh" size={16} style={{ color: 'var(--acct)' }} />
      {st.scanning ? (
        <span>{t('scanning')} <span className="mono" style={{ color: 'var(--fg)' }}>{st.library}</span>{st.added ? ` — ${tr('найдено', 'found')} ${st.added} ${tr('новых файлов', 'new files')}` : ''}</span>
      ) : st.matching > 0 ? (
        <span>{tr(`Ожидают данных TMDB: ${st.matching}`, `Waiting for TMDB: ${st.matching}`)} · <Link to="/settings#lib">{tr('ключ TMDB', 'TMDB key')}</Link></span>
      ) : (
        <span>{st.error ? st.error : tr('Библиотека актуальна, папки отслеживаются', 'Library is up to date, folders are watched')}</span>
      )}
      <div style={{ flex: '1 1 140px', height: 4, borderRadius: 2, background: 'var(--line2)' }}><div style={{ width: `${pct * 100}%`, height: 4, borderRadius: 2, background: 'var(--acc)' }} /></div>
      <span className="mono">{st.movies} {tr('фильмов', 'movies')} · {st.shows} {tr('сериалов', 'shows')}</span>
    </div>
  );
}
