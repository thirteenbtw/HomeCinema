import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { api, fmtLeft, type Item, type ScanStatus } from '../api';
import { useApp, useAsync } from '../state';
import { t, tr } from '../i18n';
import { Art, ambientFor, paletteFor } from '../components/Art';
import { Icon, PlayGlyph } from '../components/Icon';
import { Section, useAmbient } from '../components/Shell';
import { TopBar } from '../components/TopBar';

type HomeData = { featured: Item[]; continue: Item[]; popular: Item[]; recent: Item[]; genres: string[]; scan: ScanStatus };

export const itemLink = (it: Item) => (it.kind === 'episode' && it.showId ? `/show/${it.showId}` : `/movie/${it.id}`);
export const itemName = (it: Item) => (it.kind === 'episode' ? it.showTitle || it.title : it.title);

export default function Home() {
  const { scan, lastProgress, prefs, openCast } = useApp();
  const { data, error, reload } = useAsync(() => api.get<HomeData>('/api/home'), []);
  const setAmbient = useAmbient();
  const [idx, setIdx] = useState(0);
  const [hover, setHover] = useState(false);
  const [spot, setSpot] = useState({ x: 0, y: 0 });
  const [par, setPar] = useState({ x: 0, y: 0 });
  const raf = useRef(0);
  const live = prefs.bg === 'live';

  // Refresh rows when a scan adds titles or progress changes elsewhere.
  const lastAdded = useRef(0);
  useEffect(() => {
    if (scan && (scan.added !== lastAdded.current || (!scan.scanning && scan.matching === 0))) {
      lastAdded.current = scan.added;
      reload();
    }
  }, [scan?.added, scan?.scanning, scan?.matching]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (lastProgress) reload(); }, [lastProgress?.itemId]); // eslint-disable-line react-hooks/exhaustive-deps

  const featured = data?.featured || [];
  const film = featured[idx % Math.max(featured.length, 1)];
  useEffect(() => {
    if (featured.length < 2) return;
    const id = window.setInterval(() => { if (!hover) setIdx((i) => (i + 1) % featured.length); }, 9000);
    return () => window.clearInterval(id);
  }, [featured.length, hover]);
  useEffect(() => { if (film) setAmbient(ambientFor(itemName(film))); return () => setAmbient(null); }, [film?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const onHero = (e: React.MouseEvent<HTMLElement>) => {
    if (!live) return;
    const r = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - r.left, y = e.clientY - r.top;
    cancelAnimationFrame(raf.current);
    raf.current = requestAnimationFrame(() => {
      setSpot({ x, y });
      setPar({ x: (x / r.width) * 2 - 1, y: (y / r.height) * 2 - 1 });
    });
    setHover(true);
  };
  const mv = (k: number) => (live ? `translate3d(${(par.x * k).toFixed(1)}px,${(par.y * k).toFixed(1)}px,0)` : 'none');
  const st = scan || data?.scan;
  const empty = data && featured.length === 0 && data.recent.length === 0;

  return (
    <main className="main">
      <TopBar />
      {error && <div className="err">{error}</div>}
      {empty && (
        <div className="empty glass">
          <Icon name="folder" size={32} />
          <div style={{ fontWeight: 700, color: 'var(--fg)', fontSize: 'calc(16px * var(--fs))' }}>{t('libEmpty')}</div>
          <div>{t('libEmptyHint')}</div>
          <Link className="btn btn-r" to="/settings#lib"><Icon name="plus" size={16} />{t('addFolder')}</Link>
        </div>
      )}

      {film && <Hero film={film} spot={spot} hover={hover} live={live} mv={mv} onMove={onHero} onLeave={() => setHover(false)}
        dots={featured.length} idx={idx % featured.length} setIdx={setIdx} onCast={() => openCast({ fileId: film.files?.[0]?.id, title: film.title })} />}

      {data && data.genres.length > 0 && (
        <section aria-label={tr('Жанры', 'Genres')} className="scroll-x" style={{ gap: 10 }}>
          {data.genres.slice(0, 14).map((g) => (
            <Link key={g} to={`/library/movies?genre=${encodeURIComponent(g)}`} style={{ flex: '0 0 168px', height: 64, borderRadius: 10, background: paletteFor(g)[0], padding: '10px 12px', display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', color: '#FFFFFF' }}>
              <span style={{ fontSize: 'calc(9px * var(--fs))', fontWeight: 700, letterSpacing: '.12em', color: '#FF9AA0' }}>{t('genre')}</span>
              <span className="disp" style={{ fontSize: 'calc(20px * var(--fs))', fontWeight: 600, lineHeight: 1 }}>{g}</span>
            </Link>
          ))}
        </section>
      )}

      {data && data.continue.length > 0 && (
        <Section id="cont" title={t('continueWatching')} note={t('syncNote')}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(240px,1fr))', gap: 12 }}>
            {data.continue.map((c) => {
              const p = c.progress!;
              const pct = p.duration ? p.position / p.duration : 0;
              const sub = c.kind === 'episode' ? `С${c.season} · Э${c.episode}${c.epTitle ? ' · ' + c.epTitle : ''}` : `${tr('Фильм', 'Movie')}${c.year ? ' · ' + c.year : ''}`;
              return (
                <Link key={c.id} to={`/play/${c.files?.[0]?.id ?? ''}?item=${c.id}`} state={{ itemId: c.id }} style={{ display: 'flex', flexDirection: 'column', gap: 8, color: 'var(--fg)' }}
                  onClick={(e) => { if (!c.files?.length) { e.preventDefault(); } }}>
                  <ContinueCard item={c} pct={pct} />
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, fontSize: 'calc(12px * var(--fs))' }}>
                    <span style={{ fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{sub}</span>
                    <span style={{ color: 'var(--mut)', whiteSpace: 'nowrap' }}>{fmtLeft(p.duration - p.position, t)}</span>
                  </div>
                </Link>
              );
            })}
          </div>
        </Section>
      )}

      {data && data.popular.length > 0 && (
        <Section id="top" title={t('popular')} note={t('popularNote')} more="/library/movies?sort=rating">
          <div className="scroll-x">
            {data.popular.map((p, i) => (
              <Link key={p.id} to={itemLink(p)} style={{ flex: '0 0 auto', display: 'flex', flexDirection: 'column', gap: 6, color: 'var(--fg)' }}>
                <div style={{ display: 'flex', alignItems: 'flex-end' }}>
                  <span className="disp" aria-hidden="true" style={{ fontSize: 'calc(96px * var(--fs))', lineHeight: 0.8, fontWeight: 700, color: 'var(--bg)', WebkitTextStroke: '2px var(--num)', marginRight: -14 }}>{i + 1}</span>
                  <Art src={p.kind === 'episode' ? undefined : p.poster} title={itemName(p)} style={{ width: 120, height: 178, borderRadius: 8, boxShadow: '0 4px 14px var(--sh)' }}
                    label={!p.poster ? <span className="disp" style={{ position: 'absolute', left: 8, right: 8, bottom: 10, fontSize: 'calc(17px * var(--fs))', fontWeight: 600, lineHeight: 1, color: '#FFFFFF' }}>{itemName(p)}</span> : undefined} />
                </div>
                <span style={{ fontSize: 'calc(11px * var(--fs))', color: 'var(--mut)', paddingLeft: 40, maxWidth: 160, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {[p.genres[0], p.year || ''].filter(Boolean).join(' · ')}
                </span>
              </Link>
            ))}
          </div>
        </Section>
      )}

      {data && data.recent.length > 0 && (
        <Section id="recent" title={t('recent')} note={t('recentNote')} more="/library/movies">
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(200px,1fr))', gap: 8 }}>
            {data.recent.map((r) => (
              <Link key={r.id} to={itemLink(r)} style={{ color: '#FFFFFF' }}>
                <Art src={r.backdrop || r.poster} title={itemName(r)} disc="wide" style={{ aspectRatio: '16/10', borderRadius: 8 }}
                  label={<>
                    {(r.backdrop || r.poster) && <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(transparent 40%, #000000B0)' }} />}
                    <span className="disp" style={{ position: 'absolute', left: 10, right: 10, bottom: 26, fontSize: 'calc(18px * var(--fs))', fontWeight: 600, lineHeight: 1 }}>{itemName(r)}</span>
                    <span style={{ position: 'absolute', left: 0, right: 0, bottom: 0, padding: '4px 10px', fontSize: 'calc(10px * var(--fs))', fontWeight: 700, background: 'var(--acc)' }}>{recentTag(r)}</span>
                  </>} />
              </Link>
            ))}
          </div>
        </Section>
      )}

      {st && <ScanBar st={st} />}
    </main>
  );
}

function recentTag(r: Item) {
  if (r.kind === 'episode') return `${tr('Новая серия', 'New episode')} · С${r.season} Э${r.episode}`;
  const days = Math.floor((Date.now() / 1000 - r.addedAt) / 86400);
  const when = days < 1 ? tr('Добавлен сегодня', 'Added today') : days < 2 ? tr('Добавлен вчера', 'Added yesterday') : tr(`Добавлен ${days} дн. назад`, `Added ${days} days ago`);
  const src = r.source === 'torrent' ? tr('Из торрента', 'From torrent') : when;
  return r.quality ? `${src} · ${r.quality}` : src;
}

function ContinueCard({ item, pct }: { item: Item; pct: number }) {
  return (
    <Art src={item.backdrop} title={itemName(item)} disc="wide" style={{ aspectRatio: '16/9', borderRadius: 12 }}
      label={<>
        {item.backdrop && <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(transparent 45%, #000000B0)' }} />}
        <span className="disp" style={{ position: 'absolute', left: 12, bottom: 14, right: 12, fontSize: 'calc(20px * var(--fs))', fontWeight: 600, color: '#FFFFFF', lineHeight: 1 }}>{itemName(item)}</span>
        <span style={{ position: 'absolute', right: 10, top: 10, fontSize: 'calc(11px * var(--fs))', fontWeight: 700, padding: '3px 8px', borderRadius: 999, background: '#000000A0', color: '#FFFFFF' }}>
          {item.progress?.device === 'tv' ? tr('ТВ', 'TV') : tr('ПК', 'PC')}
        </span>
        <div style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: 4, background: '#FFFFFF33' }}><div style={{ height: 4, width: `${pct * 100}%`, background: 'var(--acc)' }} /></div>
      </>} />
  );
}

function Hero({ film, spot, hover, live, mv, onMove, onLeave, dots, idx, setIdx, onCast }: {
  film: Item; spot: { x: number; y: number }; hover: boolean; live: boolean; mv: (k: number) => string;
  onMove: (e: React.MouseEvent<HTMLElement>) => void; onLeave: () => void; dots: number; idx: number; setIdx: (i: number) => void; onCast: () => void;
}) {
  const name = itemName(film);
  const [sky, orb] = paletteFor(name);
  const meta = [film.year || '', ...film.genres.slice(0, 2)].filter(Boolean).join(' · ');
  const fileId = film.files?.[0]?.id;
  const isNew = Date.now() / 1000 - film.addedAt < 7 * 86400;
  return (
    <section aria-label={tr('Рекомендуемое', 'Featured')} onMouseMove={onMove} onMouseLeave={onLeave}
      style={{ position: 'relative', overflow: 'hidden', borderRadius: 20, background: sky, minHeight: 460, display: 'flex', alignItems: 'flex-end', transition: 'background .8s', boxShadow: '0 20px 50px var(--sh)' }}>
      {film.backdrop ? (
        <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-10) }}>
          <img key={film.backdrop} src={film.backdrop} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover', animation: 'fadein .8s' }} />
        </div>
      ) : (
        <>
          <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-8) }}>
            <div style={{ position: 'absolute', right: -60, top: -140, width: 680, height: 680, borderRadius: '50%', background: '#FFFFFF10' }} />
          </div>
          <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-22) }}>
            <div style={{ position: 'absolute', right: 220, top: 90, width: 280, height: 280, borderRadius: '50%', background: orb, transition: 'background .8s' }} />
          </div>
          <div aria-hidden="true" className="lay" style={{ position: 'absolute', left: -40, right: -40, bottom: -30, height: 200, background: '#00000055', transform: mv(-36) }} />
        </>
      )}
      <div aria-hidden="true" style={{ position: 'absolute', width: 440, height: 440, margin: '-220px 0 0 -220px', left: spot.x, top: spot.y, borderRadius: '50%', background: orb, filter: 'blur(70px)', opacity: live && hover ? 0.55 : 0, mixBlendMode: 'screen', transition: 'opacity .4s', pointerEvents: 'none' }} />
      <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(90deg, #000000B0 0%, #00000073 45%, transparent 75%)' }} />

      {dots > 1 && (
        <div style={{ position: 'absolute', right: 20, bottom: 20, display: 'flex', gap: 8, alignItems: 'center', zIndex: 1 }}>
          {Array.from({ length: dots }, (_, i) => (
            <button key={i} onClick={() => setIdx(i)} aria-label={`${tr('Фильм', 'Movie')} ${i + 1}`} style={{ width: i === idx ? 24 : 8, height: 8, borderRadius: 4, border: 0, padding: 0, background: '#FFFFFF', opacity: i === idx ? 1 : 0.45, cursor: 'pointer', transition: 'width .3s' }} />
          ))}
          <button onClick={() => setIdx((idx + 1) % dots)} aria-label={tr('Следующий фильм', 'Next movie')} style={{ marginLeft: 6, width: 32, height: 32, borderRadius: '50%', border: 0, background: '#FFFFFF2E', color: '#FFFFFF', display: 'grid', placeItems: 'center', cursor: 'pointer' }}>
            <Icon name="right" size={14} />
          </button>
        </div>
      )}

      <div style={{ position: 'relative', padding: 40, maxWidth: 560, display: 'flex', flexDirection: 'column', gap: 12, color: '#FFFFFF' }}>
        <div className="disp" style={{ fontSize: 'calc(clamp(40px,6vw,64px) * var(--fs))', lineHeight: 0.92, fontWeight: 600, letterSpacing: '.06em' }}>{name}</div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', fontSize: 'calc(13px * var(--fs))', fontWeight: 600 }}>
          {meta}
          {isNew && <span style={{ fontSize: 'calc(11px * var(--fs))', padding: '3px 8px', borderRadius: 999, background: 'var(--acc)' }}>{film.source === 'torrent' ? tr('Из торрента', 'From torrent') : tr('Новое', 'New')}</span>}
          {film.quality && <span style={{ fontSize: 'calc(11px * var(--fs))', padding: '3px 8px', borderRadius: 999, background: '#FFFFFF2E' }}>{film.quality}</span>}
        </div>
        {film.overview && <p style={{ margin: 0, fontSize: 'calc(14px * var(--fs))', lineHeight: 1.55, color: '#D4D7DC', display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{film.overview}</p>}
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', paddingTop: 6 }}>
          {fileId && <Link className="btn btn-w" to={`/play/${fileId}?item=${film.id}`}><PlayGlyph />{film.progress && !film.progress.watched && film.progress.position > 30 ? t('continue') : t('watch')}</Link>}
          {fileId && <button className="btn btn-g" onClick={onCast}><Icon name="cast" size={18} />{t('toTv')}</button>}
          <Link className="btn btn-g" to={itemLink(film)} style={{ color: '#FFFFFF' }}>{t('details')}</Link>
        </div>
      </div>
    </section>
  );
}

function ScanBar({ st }: { st: ScanStatus }) {
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
