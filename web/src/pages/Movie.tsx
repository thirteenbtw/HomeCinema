import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api, fmtRuntime, fmtSize, fmtTime, type Compat, type Item, type Show, type Title } from '../api';
import { useApp, useAsync } from '../state';
import { t, tr } from '../i18n';
import { Art, ambientFor, paletteFor } from '../components/Art';
import { Icon, PlayGlyph } from '../components/Icon';
import { Modal } from '../components/CastDialog';
import { Section, useAmbient } from '../components/Shell';
import { TopBar } from '../components/TopBar';

type ItemResp = { item: Item; next: Item | null; similar: Title[]; compat: Compat[]; metaLang: string };
type ShowResp = { show: Show; nextUp: Item | null };

export function MoviePage() {
  const { id } = useParams();
  const { data, error, reload } = useAsync(() => api.get<ItemResp>(`/api/items/${id}`), [id]);
  if (error) return <main className="main pad"><div className="err">{error}</div></main>;
  if (!data) return <main className="main pad"><span className="spin" /></main>;
  return <Detail kind="movie" item={data.item} similar={data.similar} compat={data.compat} reload={reload} />;
}

export function ShowPage() {
  const { id } = useParams();
  const { data, error, reload } = useAsync(() => api.get<ShowResp>(`/api/shows/${id}`), [id]);
  if (error) return <main className="main pad"><div className="err">{error}</div></main>;
  if (!data) return <main className="main pad"><span className="spin" /></main>;
  return <Detail kind="show" show={data.show} nextUp={data.nextUp} reload={reload} />;
}

type DetailProps = { kind: 'movie' | 'show'; item?: Item; show?: Show; nextUp?: Item | null; similar?: Title[]; compat?: Compat[]; reload: () => void };

function Detail({ kind, item, show, nextUp, similar = [], compat = [], reload }: DetailProps) {
  const { prefs, openCast, toast } = useApp();
  const nav = useNavigate();
  const setAmbient = useAmbient();
  const [par, setPar] = useState({ x: 0, y: 0, sx: 0, sy: 0, hover: false });
  const [audio, setAudio] = useState<number | undefined>(undefined);
  const [sub, setSub] = useState<number>(-1);
  const [matching, setMatching] = useState(false);
  const [season, setSeason] = useState<number | null>(null);
  const live = prefs.bg === 'live';

  const title = kind === 'movie' ? item!.title : show!.title;
  const year = kind === 'movie' ? item!.year : show!.year;
  const genres = kind === 'movie' ? item!.genres : show!.genres;
  const backdrop = kind === 'movie' ? item!.backdrop : show!.backdrop;
  const overview = kind === 'movie' ? item!.overview : show!.overview;
  const cast = (kind === 'movie' ? item!.cast : show!.cast) || [];
  const rating = kind === 'movie' ? item!.rating : show!.rating;
  const fav = kind === 'movie' ? item!.favorite : show!.favorite;
  const later = kind === 'movie' ? item!.later : show!.later;
  const target = kind === 'movie' ? item! : nextUp || undefined;
  const file = target?.files?.[0];
  const prog = target?.progress;
  const resume = prog && !prog.watched && prog.position > 30 ? prog.position : 0;
  const [sky, orb] = paletteFor(title);

  useEffect(() => { setAmbient(ambientFor(title)); return () => setAmbient(null); }, [title]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (show?.seasons?.length && season === null) setSeason(nextUp?.season ?? show.seasons[0].number);
  }, [show, nextUp, season]);
  useEffect(() => {
    const def = file?.audio?.find((a) => a.default) || file?.audio?.[0];
    setAudio(def?.index);
  }, [file?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const mv = (k: number) => (live ? `translate3d(${(par.x * k).toFixed(1)}px,${(par.y * k).toFixed(1)}px,0)` : 'none');
  const onHero = (e: React.MouseEvent<HTMLElement>) => {
    if (!live) return;
    const r = e.currentTarget.getBoundingClientRect();
    setPar({ x: ((e.clientX - r.left) / r.width) * 2 - 1, y: ((e.clientY - r.top) / r.height) * 2 - 1, sx: e.clientX - r.left, sy: e.clientY - r.top, hover: true });
  };

  const playUrl = (f: number, it: number, start = resume) => `/play/${f}?item=${it}${start ? '&start=' + Math.floor(start) : ''}${audio !== undefined ? '&audio=' + audio : ''}${sub >= 0 ? '&sub=' + sub : ''}`;
  const toggleList = async (list: 'favorites' | 'later', on: boolean) => {
    await api.post(`/api/lists/${list}`, kind === 'movie' ? { itemId: item!.id, on } : { showId: show!.id, on });
    toast(on ? (list === 'favorites' ? tr('Добавлено в избранное', 'Added to favorites') : tr('Добавлено в «Посмотреть позже»', 'Added to Watch later')) : tr('Убрано из списка', 'Removed from list'));
    reload();
  };
  const markWatched = async () => {
    const watched = kind === 'movie' ? !item!.progress?.watched : show!.unwatched > 0;
    await api.post(kind === 'movie' ? `/api/items/${item!.id}/watched` : `/api/shows/${show!.id}/watched`, { watched });
    toast(watched ? t('watched') : tr('Отметка снята', 'Unmarked'));
    reload();
  };
  const isWatched = kind === 'movie' ? !!item!.progress?.watched : show!.unwatched === 0;
  const runtime = kind === 'movie' ? item!.runtime : 0;
  const meta = [year || '', runtime ? fmtRuntime(runtime) : '', ...genres.slice(0, 2)].filter(Boolean).join(' · ');
  const seasons = show?.seasons || [];
  const curSeason = seasons.find((s) => s.number === season) || seasons[0];

  return (
    <main className="main" style={{ gap: 28 }}>
      <TopBar types={false} />
      <section onMouseMove={onHero} onMouseLeave={() => setPar((p) => ({ ...p, hover: false }))}
        style={{ position: 'relative', overflow: 'hidden', borderRadius: 20, background: sky, minHeight: 440, display: 'flex', flexDirection: 'column', justifyContent: 'space-between', padding: '20px 40px 36px', color: '#FFFFFF', boxShadow: '0 20px 50px var(--sh)' }}>
        {backdrop ? (
          <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-10) }}><img src={backdrop} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} /></div>
        ) : (
          <>
            <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-8) }}><div style={{ position: 'absolute', right: -60, top: -140, width: 720, height: 720, borderRadius: '50%', background: '#FFFFFF10' }} /></div>
            <div aria-hidden="true" className="lay" style={{ position: 'absolute', inset: -40, transform: mv(-22) }}><div style={{ position: 'absolute', right: 200, top: 90, width: 280, height: 280, borderRadius: '50%', background: 'var(--acc)' }} /></div>
            <div aria-hidden="true" className="lay" style={{ position: 'absolute', left: -40, right: -40, bottom: -30, height: 190, background: '#00000055', transform: mv(-36) }} />
          </>
        )}
        <div aria-hidden="true" style={{ position: 'absolute', width: 440, height: 440, margin: '-220px 0 0 -220px', left: par.sx, top: par.sy, borderRadius: '50%', background: orb, filter: 'blur(70px)', opacity: live && par.hover ? 0.5 : 0, mixBlendMode: 'screen', transition: 'opacity .4s', pointerEvents: 'none' }} />
        <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(90deg,#000000B8 0%,#00000073 50%,transparent 80%)' }} />
        <nav aria-label={tr('Путь', 'Breadcrumb')} style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, fontWeight: 600 }}>
          <Link to="/" style={{ display: 'inline-flex', alignItems: 'center', gap: 6, height: 28, padding: '0 10px', borderRadius: 999, background: '#FFFFFF2E', color: '#FFFFFF' }}><Icon name="left" size={14} />{t('home')}</Link>
          <span style={{ color: '#C9CCD1' }}>›</span><span>{title}{year ? ` (${year})` : ''}</span>
        </nav>
        <div style={{ position: 'relative', display: 'flex', flexDirection: 'column', gap: 12, maxWidth: 640, marginTop: 24 }}>
          <div className="disp" style={{ fontSize: 'clamp(44px,7vw,80px)', lineHeight: 0.9, fontWeight: 600, letterSpacing: '.05em' }}>{title}</div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', fontSize: 13, fontWeight: 600 }}>
            {meta}
            {item?.cert && <span style={{ fontSize: 11, padding: '2px 7px', border: '1px solid #FFFFFF66', borderRadius: 4 }}>{item.cert}</span>}
            {rating > 0 && <span style={{ fontSize: 11, padding: '2px 7px', borderRadius: 4, background: '#01B4E4', color: '#06222C' }}>TMDB {rating.toFixed(1)}</span>}
            {item?.quality && <span style={{ fontSize: 11, padding: '2px 7px', borderRadius: 4, background: '#FFFFFF2E' }}>{item.quality}</span>}
            {show && <span style={{ fontSize: 11, padding: '2px 7px', borderRadius: 4, background: '#FFFFFF2E' }}>{seasons.length} {tr('сез.', 'seasons')} · {show.episodes} {t('episodes')}</span>}
          </div>
          {prog && prog.duration > 0 && !prog.watched && prog.position > 30 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, maxWidth: 380, fontSize: 12, color: '#D4D7DC' }}>
              <div style={{ flex: 1, height: 4, borderRadius: 2, background: '#FFFFFF33' }}><div style={{ width: `${(prog.position / prog.duration) * 100}%`, height: 4, borderRadius: 2, background: 'var(--acc)' }} /></div>
              {fmtTime(prog.position)} {tr('из', 'of')} {fmtTime(prog.duration)}
            </div>
          )}
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', paddingTop: 4 }}>
            {file && target && <Link className="btn btn-w" to={playUrl(file.id, target.id)}><PlayGlyph />{resume ? t('continue') : t('watch')}{kind === 'show' && target.season ? ` С${target.season}·Э${target.episode}` : ''}</Link>}
            {file && <button className="btn btn-g" onClick={() => openCast({ fileId: file.id, start: resume, title, audio, sub })}><Icon name="cast" size={18} />{t('toTv')}</button>}
            <button className="btn btn-g" aria-pressed={later} onClick={() => toggleList('later', !later)}><Icon name={later ? 'check' : 'plus'} size={18} />{later ? t('inListOn') : t('inList')}</button>
            <button className="btn btn-g" aria-label={t('addFav')} aria-pressed={fav} onClick={() => toggleList('favorites', !fav)} style={{ width: 44, padding: 0, background: fav ? 'var(--acc)' : undefined }}><Icon name="heart" size={18} /></button>
            <button className="btn btn-g" aria-label={t('markWatched')} aria-pressed={isWatched} onClick={markWatched} style={{ width: 44, padding: 0, background: isWatched ? '#2F9E62' : undefined }}><Icon name="check" size={18} /></button>
          </div>
        </div>
      </section>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 32 }}>
        <div style={{ flex: '999 1 480px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 28 }}>
          <p style={{ margin: 0, fontSize: 14, lineHeight: 1.65, color: 'var(--fg2)', maxWidth: 680 }}>{overview || t('noOverview')}</p>

          {show && seasons.length > 0 && (
            <section style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
                <h2 className="h2" style={{ marginRight: 8 }}>{t('seasons')}</h2>
                {seasons.map((s) => <button key={s.number} className={'pill' + (s.number === curSeason?.number ? ' on' : '')} onClick={() => setSeason(s.number)}>{s.number ? `${t('season')} ${s.number}` : tr('Спецвыпуски', 'Specials')}</button>)}
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {curSeason?.episodes.map((e) => {
                  const p = e.progress;
                  const pct = p && p.duration ? (p.watched ? 1 : p.position / p.duration) : 0;
                  const ef = e.files?.[0];
                  return (
                    <div key={e.id} className="glass" style={{ display: 'flex', gap: 14, alignItems: 'center', padding: 10, borderRadius: 14, border: '1px solid var(--line)' }}>
                      <Art src={e.backdrop} title={title + e.episode} disc="wide" style={{ width: 160, aspectRatio: '16/9', borderRadius: 8, flex: 'none' }}
                        label={<div style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: 3, background: '#FFFFFF33' }}><div style={{ height: 3, width: `${pct * 100}%`, background: 'var(--acc)' }} /></div>} />
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontWeight: 700, fontSize: 14 }}>{e.episode}. {e.epTitle || `${tr('Серия', 'Episode')} ${e.episode}`}{p?.watched && <span className="tag" style={{ marginLeft: 8, background: 'var(--ok-bg)', color: 'var(--ok-fg)' }}>{t('watched')}</span>}</div>
                        <div className="sub" style={{ display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden', marginTop: 4 }}>{e.overview || [e.quality, e.runtime ? fmtRuntime(e.runtime) : ''].filter(Boolean).join(' · ')}</div>
                      </div>
                      {ef && <Link className="btn btn-l sm" to={`/play/${ef.id}?item=${e.id}${p && !p.watched && p.position > 30 ? '&start=' + Math.floor(p.position) : ''}`} aria-label={`${t('watch')} ${e.episode}`}><PlayGlyph size={14} /></Link>}
                      {ef && <button className="btn btn-l sm" aria-label={`${t('toTv')} ${e.episode}`} onClick={() => openCast({ fileId: ef.id, title: `${title} · С${e.season}Э${e.episode}`, start: p && !p.watched ? p.position : 0 })}><Icon name="cast" size={16} /></button>}
                    </div>
                  );
                })}
              </div>
            </section>
          )}

          {cast.length > 0 && (
            <Section title={t('cast')} note={tr('Наведите — фото станет цветным', 'Hover to colorize')}>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(120px,1fr))', gap: 10 }}>
                {cast.slice(0, 12).map((a) => (
                  <Link key={a.name + a.role} to={`/search?q=${encodeURIComponent(a.name)}&kind=people`} className="castcard" style={{ position: 'relative', aspectRatio: '3/4', borderRadius: 10, overflow: 'hidden', background: '#3A3B3E', color: '#FFFFFF' }}>
                    {a.photo ? <img src={a.photo} alt="" loading="lazy" style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover' }} />
                      : <span className="disp" aria-hidden="true" style={{ position: 'absolute', left: 0, right: 0, top: '28%', textAlign: 'center', fontSize: 36, fontWeight: 600, color: '#FFFFFF59' }}>{initials(a.name)}</span>}
                    <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(transparent 50%,#000000C0)' }} />
                    <span style={{ position: 'absolute', left: 10, right: 10, bottom: 22, fontSize: 12, fontWeight: 700 }}>{a.name}</span>
                    <span style={{ position: 'absolute', left: 10, right: 10, bottom: 8, fontSize: 11, color: '#D9DBDF', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{a.role}</span>
                  </Link>
                ))}
              </div>
            </Section>
          )}

          {similar.length > 0 && (
            <Section title={t('similar')} note={t('similarNote')}>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(200px,1fr))', gap: 8 }}>
                {similar.map((r) => (
                  <Link key={r.id} to={`/movie/${r.id}`} style={{ color: '#FFFFFF' }}>
                    <Art src={r.backdrop} title={r.title} disc="wide" style={{ aspectRatio: '16/10', borderRadius: 8 }}
                      label={<><div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(transparent 50%,#000000A0)' }} /><span className="disp" style={{ position: 'absolute', left: 10, bottom: 10, right: 10, fontSize: 18, fontWeight: 600, lineHeight: 1 }}>{r.title}</span></>} />
                  </Link>
                ))}
              </div>
            </Section>
          )}
        </div>

        <aside style={{ flex: '1 1 320px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 12 }}>
          {file && (
            <section className="card solid">
              <h2 className="h2" style={{ fontSize: 14 }}>{t('tracks')}</h2>
              <label className="fld">{t('audio')}
                <select className="sel" value={audio ?? ''} onChange={(e) => setAudio(Number(e.target.value))}>
                  {(file.audio || []).map((a, i) => <option key={a.index} value={a.index}>{trackName(a.lang, a.title, i)} — {codecName(a.codec)}{a.channels ? ' ' + channels(a.channels) : ''}</option>)}
                </select>
              </label>
              <label className="fld">{t('subtitles')}
                <select className="sel" value={sub} onChange={(e) => setSub(Number(e.target.value))}>
                  <option value={-1}>{t('off')}</option>
                  {(file.subs || []).map((s, i) => <option key={i} value={i}>{trackName(s.lang, s.title, i)} — {s.codec.replace('_subtitle', '').toUpperCase()}, {s.external ? tr('внешние', 'external') : tr('встроенные', 'embedded')}</option>)}
                </select>
              </label>
            </section>
          )}
          {compat.length > 0 && (
            <section className="card solid">
              <h2 className="h2" style={{ fontSize: 14 }}>{t('whereplays')}</h2>
              {compat.map((d) => (
                <div key={d.name} style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 13 }}>
                  <span style={{ width: 8, height: 8, borderRadius: '50%', background: d.status === 'direct' ? '#2F9E62' : '#E08A00', flex: 'none' }} />
                  <span style={{ flex: 1, fontWeight: 600 }}>{d.name}</span>
                  <span style={{ color: 'var(--mut2)', textAlign: 'right' }}>{d.mode}</span>
                </div>
              ))}
            </section>
          )}
          {file && (
            <section className="card solid">
              <h2 className="h2" style={{ fontSize: 14 }}>{t('file')}</h2>
              <div className="mono" style={{ fontSize: 11, color: 'var(--mut2)', wordBreak: 'break-all' }}>{file.name}</div>
              <div style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '6px 14px', fontSize: 12 }}>
                <span style={{ color: 'var(--mut)' }}>{t('video')}</span><span>{codecName(file.videoCodec)}{file.bitDepth > 8 ? ` ${file.bitDepth}-bit` : ''} · {file.width}×{file.height}{file.hdr ? ' · ' + file.hdr : ''}</span>
                <span style={{ color: 'var(--mut)' }}>{t('audio')}</span><span>{(file.audio || []).length} {tr('дорожки', 'tracks')}</span>
                <span style={{ color: 'var(--mut)' }}>{t('subtitles')}</span><span>{(file.subs || []).length}{(file.subs || []).some((s) => s.external) ? `, ${tr('из них', 'incl.')} ${(file.subs || []).filter((s) => s.external).length} ${tr('внешние', 'external')}` : ''}</span>
                <span style={{ color: 'var(--mut)' }}>{t('size')}</span><span className="mono">{fmtSize(file.size)}</span>
              </div>
              <button className="btn btn-o sm" onClick={() => setMatching(true)} style={{ alignSelf: 'flex-start' }}>{t('wrongMatch')}</button>
            </section>
          )}
          {!file && show && <button className="btn btn-o sm" onClick={() => setMatching(true)}>{t('wrongMatch')}</button>}
        </aside>
      </div>
      {matching && <MatchDialog kind={kind} id={kind === 'movie' ? item!.id : show!.id} initial={title} year={year} onClose={() => setMatching(false)} onDone={() => { setMatching(false); reload(); nav(0); }} />}
    </main>
  );
}

function initials(n: string) { return n.split(/\s+/).map((x) => x[0]).slice(0, 2).join('').toUpperCase(); }
const langNames: Record<string, string> = { rus: 'Русский', ru: 'Русский', eng: 'English', en: 'English', ukr: 'Українська', jpn: '日本語', ger: 'Deutsch', fre: 'Français', spa: 'Español' };
function trackName(lang: string, title: string, i: number) {
  const l = langNames[lang] || (lang && lang !== 'und' ? lang.toUpperCase() : `${tr('Дорожка', 'Track')} ${i + 1}`);
  return title ? `${l} (${title})` : l;
}
function codecName(c: string) { return ({ hevc: 'HEVC', h264: 'H.264', ac3: 'DD', eac3: 'DD+', dts: 'DTS', truehd: 'TrueHD', aac: 'AAC', av1: 'AV1', vp9: 'VP9', opus: 'Opus', flac: 'FLAC', mp3: 'MP3' } as Record<string, string>)[c] || c.toUpperCase(); }
function channels(n: number) { return ({ 1: '1.0', 2: '2.0', 6: '5.1', 8: '7.1' } as Record<number, string>)[n] || `${n}ch`; }

type Cand = { id: number; title: string; name: string; release_date: string; first_air_date: string; poster_path: string; overview: string };

function MatchDialog({ kind, id, initial, year, onClose, onDone }: { kind: 'movie' | 'show'; id: number; initial: string; year: number; onClose: () => void; onDone: () => void }) {
  const [q, setQ] = useState(initial);
  const [y, setY] = useState(year ? String(year) : '');
  const [res, setRes] = useState<Cand[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const search = async () => {
    setErr(null); setBusy(true);
    try { setRes(await api.get<Cand[]>(`/api/match/search?kind=${kind}&q=${encodeURIComponent(q)}&year=${y}`)); } catch (e: any) { setErr(e.message); } finally { setBusy(false); }
  };
  useEffect(() => { search(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const pick = async (tmdbId: number) => {
    setBusy(true);
    try { await api.post(`/api/${kind === 'movie' ? 'items' : 'shows'}/${id}/match`, { tmdbId }); onDone(); } catch (e: any) { setErr(e.message); setBusy(false); }
  };
  return (
    <Modal title={tr('Исправить совпадение', 'Fix match')} onClose={onClose} wide>
      <form className="row" onSubmit={(e) => { e.preventDefault(); search(); }}>
        <input className="inp" style={{ flex: '1 1 240px' }} value={q} onChange={(e) => setQ(e.target.value)} aria-label={tr('Название', 'Title')} />
        <input className="inp" style={{ width: 90 }} value={y} onChange={(e) => setY(e.target.value)} placeholder={tr('Год', 'Year')} aria-label={tr('Год', 'Year')} />
        <button className="btn btn-d sm" disabled={busy}>{tr('Искать', 'Search')}</button>
      </form>
      {err && <div className="err">{err}</div>}
      {res && res.length === 0 && <div className="sub">{t('nothing')}</div>}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {res?.slice(0, 10).map((c) => (
          <button key={c.id} className="btn btn-l" disabled={busy} onClick={() => pick(c.id)} style={{ justifyContent: 'flex-start', height: 'auto', padding: 10, borderRadius: 12, textAlign: 'left', whiteSpace: 'normal' }}>
            {c.poster_path ? <img src={"https://image.tmdb.org/t/p/w92" + c.poster_path} alt="" style={{ width: 46, height: 69, objectFit: 'cover', borderRadius: 6, flex: 'none' }} /> : <span style={{ width: 46, height: 69, background: 'var(--line2)', borderRadius: 6, flex: 'none' }} />}
            <span style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <span>{c.title || c.name} <span className="sub">{(c.release_date || c.first_air_date || '').slice(0, 4)}</span></span>
              <span className="sub" style={{ fontWeight: 500, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{c.overview}</span>
            </span>
          </button>
        ))}
      </div>
    </Modal>
  );
}
