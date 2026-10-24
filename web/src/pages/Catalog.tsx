import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api, ApiError, qs, type CatalogItem, type CatalogPage } from '../api';
import { useAsync } from '../state';
import { t, tr } from '../i18n';
import { Art } from '../components/Art';
import { Section } from '../components/Shell';
import { TopBar } from '../components/TopBar';
import { Featured, Badge } from '../components/Featured';
import { previewProps } from '../components/Preview';
import { PeopleRow } from './Person';
import { ContinueSection, useHomeData } from '../components/LibraryParts';
import { Icon, PlayGlyph } from '../components/Icon';
import { CardMarks } from '../components/Marks';

type Kind = 'movie' | 'tv';
type Genre = { id: number; name: string };

const rows: Record<Kind, [string, string, string][]> = {
  movie: [
    ['trending', 'Тренды недели', 'Trending this week'], ['now_playing', 'Сейчас в кино', 'In theaters'],
    ['popular', 'Популярные', 'Popular'], ['top_rated', 'Лучшие за всё время', 'Top rated'], ['upcoming', 'Скоро', 'Upcoming'],
  ],
  tv: [
    ['trending', 'Тренды недели', 'Trending this week'], ['on_the_air', 'Сейчас выходят', 'On the air'],
    ['popular', 'Популярные', 'Popular'], ['top_rated', 'Лучшие за всё время', 'Top rated'],
  ],
};

const sorts: [string, string, string][] = [['', 'Популярные', 'Popular'], ['rating', 'По рейтингу', 'Top rated'], ['date', 'Новинки', 'Newest']];

// Home: the catalog of all of TMDB, not only the local library. A title opens
// a page where it can be streamed or downloaded from the search sources.
export default function Catalog() {
  const [params, setParams] = useSearchParams();
  const kind: Kind = params.get('kind') === 'tv' ? 'tv' : 'movie';
  const list = params.get('list') || '';
  const genre = Number(params.get('genre') || 0);
  const sort = params.get('sort') || '';
  const q = params.get('q') || '';
  const home = useHomeData();
  const genres = useAsync(() => api.get<Genre[]>(`/api/catalog/genres?kind=${kind}`), [kind]);

  const set = (patch: Record<string, string | number>) => {
    const n = new URLSearchParams(params);
    for (const [k, v] of Object.entries(patch)) { if (v) n.set(k, String(v)); else n.delete(k); }
    setParams(n);
  };

  const row = rows[kind].find((r) => r[0] === list);
  const grid = q ? { path: `/api/catalog/search${qs({ q })}`, title: `${tr('Поиск', 'Search')}: «${q}»` }
    : genre || sort ? { path: `/api/catalog${qs({ kind, genre, sort })}`, title: genres.data?.find((g) => g.id === genre)?.name || tr('Все', 'All') }
      : list ? { path: `/api/catalog${qs({ kind, list })}`, title: row ? tr(row[1], row[2]) : t('catalog') }
        : null;

  const searching = params.get('search') === '1' || !!q;
  const browsing = !searching && !genre && !list && !sort;
  useEffect(() => { if (q.length >= 2) rememberSearch(q); }, [q]);
  return (
    <main className="main">
      <h1 className="sr">{t('home')}</h1>
      <TopBar catalog />
      {browsing && <Spotlight kind={kind} />}
      {browsing && genres.data && <GenreTiles kind={kind} genres={genres.data} onPick={(id) => set({ genre: id, list: '' })} />}
      {browsing && <ContinueSection items={home.data?.continue || []} />}

      {q && <PeopleRow q={q} />}
      {searching && !q && <SearchStart kind={kind} genres={genres.data || []} onPick={(id) => set({ genre: id, search: '' })} onQuery={(text) => set({ q: text })} />}

      {!searching && !browsing && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {genres.data && genres.data.length > 0 && (
            <div className="scroll-x" role="group" aria-label={tr('Жанры', 'Genres')} style={{ gap: 6 }}>
              <button className="pill ghost" onClick={() => set({ genre: '', list: '', sort: '' })} style={{ flex: 'none' }}><Icon name="left" size={14} />{tr('Подборки', 'Featured')}</button>
              {genres.data.map((g) => (
                <button key={g.id} className={'pill' + (genre === g.id ? ' on' : ' ghost')} aria-pressed={genre === g.id} onClick={() => set({ genre: g.id, list: '' })} style={{ flex: 'none' }}>{g.name}</button>
              ))}
            </div>
          )}
          {(genre > 0 || sort) && (
            <div role="group" aria-label={t('sort')} style={{ display: 'flex', gap: 4 }}>
              {sorts.map(([k, r, e]) => <button key={k} className={'pill' + (sort === k ? ' on' : ' ghost')} aria-pressed={sort === k} onClick={() => set({ sort: k, genre: genre || '' })}>{tr(r, e)}</button>)}
            </div>
          )}
        </div>
      )}

      {genres.error && <CatalogError msg={genres.error} />}
      {!genres.error && !(searching && !q) && (grid ? <Grid key={grid.path} path={grid.path} title={grid.title} /> : rows[kind].map(([l, r, e], i) => {
        const path = `/api/catalog${qs({ kind, list: l })}`;
        const more = `/${qs({ kind: kind === 'tv' ? 'tv' : '', list: l })}`;
        // The design's rhythm: a numbered top, then wide stills, then posters.
        if (i === 0) return <TopRow key={kind + l} path={path} title={tr(r, e)} more={more} />;
        if (i === 1) return <WideRow key={kind + l} path={path} title={tr(r, e)} more={more} tag={kind === 'tv' ? tr('Новые серии', 'New episodes') : tr('Сейчас в кино', 'In theaters')} />;
        return <Row key={kind + l} path={path} title={tr(r, e)} more={more} />;
      }))}

      <p className="note" style={{ margin: 0, fontSize: 'calc(12px * var(--fs))', color: 'var(--mut)' }}>{tr('Данные и постеры — TMDB. Раздачи ищутся в источниках из настроек: JacRed или ваш Jackett / Prowlarr.', 'Data and posters from TMDB. Releases are searched on the sources in settings: JacRed or your own Jackett / Prowlarr.')}</p>
    </main>
  );
}

// Spotlight: trending titles with artwork as the featured carousel.
function Spotlight({ kind }: { kind: Kind }) {
  const { data } = useAsync(() => api.get<CatalogPage>(`/api/catalog${qs({ kind, list: 'trending' })}`), [kind]);
  const slides = (data?.items || []).filter((x) => x.backdrop).slice(0, 6).map((x) => ({
    key: x.kind + x.id, name: x.title, backdrop: x.backdrop.replace('/w780/', '/w1280/'), overview: x.overview, trailer: { kind: x.kind, id: x.id },
    meta: [x.year || '', x.kind === 'tv' ? tr('Сериал', 'Show') : tr('Фильм', 'Movie')].filter(Boolean).join(' · '),
    badges: <>{x.rating > 0 && <Badge>★ {x.rating.toFixed(1)}</Badge>}<Badge accent>{tr('В тренде', 'Trending')}</Badge></>,
    actions: <Link className="btn btn-w" to={`/catalog/${x.kind}/${x.id}`}><PlayGlyph />{t('watch')}</Link>,
  }));
  if (!data) return <div style={{ aspectRatio: '2.2 / 1', minHeight: 340, maxHeight: 'min(640px, calc(var(--vh) - 220px))', borderRadius: 20, background: 'var(--bg2)', flex: 'none' }} />;
  return <Featured slides={slides} label={tr('В тренде', 'Trending')} />;
}

// GenreTiles: dark tiles with a still from the genre's most popular title.
function GenreTiles({ kind, genres, onPick }: { kind: Kind; genres: Genre[]; onPick: (id: number) => void }) {
  return (
    <section aria-label={tr('Жанры', 'Genres')} className="scroll-x" style={{ gap: 10, flex: 'none' }}>
      {genres.slice(0, 16).map((g) => <GenreTile key={g.id} kind={kind} g={g} onPick={onPick} />)}
    </section>
  );
}

function GenreTile({ kind, g, onPick }: { kind: Kind; g: Genre; onPick: (id: number) => void }) {
  const { data } = useAsync(() => api.get<CatalogPage>(`/api/catalog${qs({ kind, genre: g.id })}`), [kind, g.id]);
  const still = data?.items.find((x) => x.backdrop)?.backdrop;
  return (
    <button className="gtile" onClick={() => onPick(g.id)}>
      {still && <img src={still} alt="" loading="lazy" />}
      <span className="gtile-k">{t('genre')}</span>
      <span className="disp gtile-n">{g.name}</span>
    </button>
  );
}

// TopRow: the week's top ten with large outlined numbers.
function TopRow({ path, title, more }: { path: string; title: string; more: string }) {
  const { data, error } = useAsync(() => api.get<CatalogPage>(path), [path]);
  if (error) return null;
  return (
    <Section title={title} more={more}>
      {!data ? <div style={{ height: 230, display: 'grid', placeItems: 'center' }}><span className="spin" /></div> : (
        <div className="scroll-x" style={{ gap: 18 }}>
          {data.items.slice(0, 10).map((x, i) => (
            <Link key={x.kind + x.id} to={`/catalog/${x.kind}/${x.id}`} className="poster" style={{ flex: 'none', gap: 6 }} {...previewProps(x)}>
              <div style={{ display: 'flex', alignItems: 'flex-end' }}>
                <span className="disp topnum" aria-hidden="true">{i + 1}</span>
                <Art className="pimg" src={x.poster} title={x.title} style={{ width: 130, aspectRatio: '2/3', borderRadius: 10, boxShadow: '0 6px 18px var(--sh)' }}
                  label={!x.poster ? <span className="disp" style={{ position: 'absolute', left: 8, right: 8, bottom: 10, fontSize: 'calc(17px * var(--fs))', fontWeight: 600, lineHeight: 1, color: '#FFFFFF' }}>{x.title}</span> : undefined} />
              </div>
              <div style={{ paddingLeft: 44, maxWidth: 180 }}>
                <div style={{ fontSize: 'calc(12px * var(--fs))', fontWeight: 600, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{x.title}</div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </Section>
  );
}

// WideRow: wide stills with a colored label, for what is new right now.
function WideRow({ path, title, more, tag }: { path: string; title: string; more: string; tag: string }) {
  const { data, error } = useAsync(() => api.get<CatalogPage>(path), [path]);
  if (error) return null;
  return (
    <Section title={title} more={more}>
      {!data ? <div style={{ height: 180, display: 'grid', placeItems: 'center' }}><span className="spin" /></div> : (
        <div className="scroll-x" style={{ gap: 10 }}>
          {data.items.filter((x) => x.backdrop).slice(0, 12).map((x) => (
            <Link key={x.kind + x.id} to={`/catalog/${x.kind}/${x.id}`} className="poster" style={{ flex: 'none', width: 260 }} {...previewProps(x)}>
              <Art className="pimg" src={x.backdrop} title={x.title} disc="wide" style={{ aspectRatio: '16/9', borderRadius: 10 }}
                label={<>
                  <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(transparent 40%, #000000B8)' }} />
                  <span className="disp" style={{ position: 'absolute', left: 10, right: 10, bottom: 26, fontSize: 'calc(18px * var(--fs))', fontWeight: 600, lineHeight: 1, color: '#FFFFFF', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{x.title}</span>
                  <span style={{ position: 'absolute', left: 0, right: 0, bottom: 0, padding: '4px 10px', fontSize: 'calc(10px * var(--fs))', fontWeight: 700, background: 'var(--acc)', color: '#FFFFFF' }}>{tag}{x.year ? ` · ${x.year}` : ''}</span>
                </>} />
            </Link>
          ))}
        </div>
      )}
    </Section>
  );
}

const recentKey = 'hc.recentSearches';
function recentSearches(): string[] {
  try { return JSON.parse(localStorage.getItem(recentKey) || '[]'); } catch { return []; }
}
function rememberSearch(q: string) {
  const list = [q, ...recentSearches().filter((x) => x.toLowerCase() !== q.toLowerCase())].slice(0, 8);
  try { localStorage.setItem(recentKey, JSON.stringify(list)); } catch { /* private mode */ }
}

const duo = ['#5B6EE1', '#2D6FB5', '#7B3FC4', '#1F8A86', '#B4235F', '#C46A1A'];

// SearchStart: what the search shows before anything is typed — recent
// queries and big genre tiles in two-tone color.
function SearchStart({ kind, genres, onPick, onQuery }: { kind: Kind; genres: Genre[]; onPick: (id: number) => void; onQuery: (q: string) => void }) {
  const [recent, setRecent] = useState(recentSearches);
  return (
    <>
      {recent.length > 0 && (
        <Section title={tr('Недавние запросы', 'Recent searches')}>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
            {recent.map((r) => <button key={r} className="pill" onClick={() => onQuery(r)}><Icon name="clock" size={14} />{r}</button>)}
            <button className="pill ghost" style={{ color: 'var(--acct)' }} onClick={() => { try { localStorage.removeItem(recentKey); } catch { /* ignore */ } setRecent([]); }}>{tr('Очистить', 'Clear')}</button>
          </div>
        </Section>
      )}
      <Section title={tr('Жанры', 'Genres')}>
        <div className="grid-duo">
          {genres.map((g, i) => <DuoTile key={g.id} kind={kind} g={g} color={duo[i % duo.length]} onPick={onPick} />)}
        </div>
      </Section>
    </>
  );
}

function DuoTile({ kind, g, color, onPick }: { kind: Kind; g: Genre; color: string; onPick: (id: number) => void }) {
  const { data } = useAsync(() => api.get<CatalogPage>(`/api/catalog${qs({ kind, genre: g.id })}`), [kind, g.id]);
  const art = data?.items.find((x) => x.poster)?.poster;
  return (
    <button className="duo" style={{ '--duo': color } as React.CSSProperties} onClick={() => onPick(g.id)}>
      {art && <img src={art} alt="" loading="lazy" />}
      <span>{g.name}</span>
    </button>
  );
}

function CatalogError({ msg }: { msg: string }) {
  return (
    <div className="empty">
      {msg}
      {msg.includes('TMDB') && <Link className="btn btn-l sm" to="/settings">{tr('Открыть настройки', 'Open settings')}</Link>}
    </div>
  );
}

function Row({ path, title, more }: { path: string; title: string; more: string }) {
  const { data, error } = useAsync(() => api.get<CatalogPage>(path), [path]);
  if (error) return null;
  return (
    <Section title={title} more={more}>
      {!data ? <div style={{ height: 260, display: 'grid', placeItems: 'center' }}><span className="spin" /></div> : (
        <div className="scroll-x">
          {data.items.map((x) => <div key={x.kind + x.id} style={{ width: 150, flex: 'none' }}><CatalogCard x={x} /></div>)}
        </div>
      )}
    </Section>
  );
}

function Grid({ path, title }: { path: string; title: string }) {
  const [items, setItems] = useState<CatalogItem[]>([]);
  const [page, setPage] = useState(0);
  const [pages, setPages] = useState(1);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const load = async (p: number) => {
    setLoading(true);
    try {
      const r = await api.get<CatalogPage>(path + (path.includes('?') ? '&' : '?') + 'page=' + p);
      setItems((old) => {
        const seen = new Set(old.map((x) => x.kind + x.id));
        return [...old, ...r.items.filter((x) => !seen.has(x.kind + x.id))];
      });
      setPage(p); setPages(r.pages); setErr(null);
    } catch (e) { setErr(e instanceof ApiError ? e.message : String(e)); } finally { setLoading(false); }
  };
  useEffect(() => { load(1); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Section title={title}>
      {err && <CatalogError msg={err} />}
      {!loading && !err && items.length === 0 && <div className="empty">{t('nothing')}</div>}
      <div className="grid-posters">{items.map((x) => <CatalogCard key={x.kind + x.id} x={x} showKind={path.includes('/search')} />)}</div>
      {loading && <span className="spin" />}
      {!loading && page < pages && items.length > 0 && <button className="btn btn-l sm" style={{ alignSelf: 'center' }} onClick={() => load(page + 1)}>{tr('Показать ещё', 'Show more')}</button>}
    </Section>
  );
}

export function CatalogCard({ x, showKind }: { x: CatalogItem; showKind?: boolean }) {
  return (
    <Link to={`/catalog/${x.kind}/${x.id}`} className="poster" {...previewProps(x)}>
      <Art className="pimg" src={x.poster} title={x.title} style={{ aspectRatio: '2/3', borderRadius: 12 }}
        label={
          <>
            {!x.poster && <span className="disp" style={{ position: 'absolute', left: 12, right: 12, bottom: 14, color: '#FFFFFF', fontSize: 'calc(20px * var(--fs))', fontWeight: 600, lineHeight: 1 }}>{x.title}</span>}
            {x.rating > 0 && <span className="tag" style={{ position: 'absolute', top: 8, left: 8, background: x.rating >= 7 ? '#2F9E62' : x.rating >= 5.5 ? '#000000A0' : '#8E3B4F', color: '#FFFFFF' }}>{x.rating.toFixed(1)}</span>}
            <CardMarks x={x} />
          </>
        } />
      <div>
        <div style={{ fontSize: 'calc(14px * var(--fs))', fontWeight: 700, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }} title={x.title}>{x.title}</div>
        <div className="sub">{[x.year || '', showKind ? (x.kind === 'tv' ? tr('сериал', 'show') : tr('фильм', 'movie')) : ''].filter(Boolean).join(' · ')}</div>
      </div>
    </Link>
  );
}
