import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api, ApiError, qs, type CatalogItem, type CatalogPage } from '../api';
import { useAsync } from '../state';
import { t, tr } from '../i18n';
import { Art } from '../components/Art';
import { Section } from '../components/Shell';
import { TopBar } from '../components/TopBar';
import { Featured, Badge } from '../components/Featured';
import { ContinueSection, useHomeData } from '../components/LibraryParts';
import { PlayGlyph } from '../components/Icon';

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

  const browsing = !q && !genre && !list && !sort;
  return (
    <main className="main">
      <h1 className="sr">{t('home')}</h1>
      <TopBar catalog />
      {browsing && <Spotlight kind={kind} />}
      {browsing && <ContinueSection items={home.data?.continue || []} />}

      {!q && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
            <div role="group" aria-label={tr('Тип', 'Type')} style={{ display: 'flex', gap: 4 }}>
              {([['movie', t('movies')], ['tv', t('shows')]] as [Kind, string][]).map(([k, l]) => (
                <button key={k} className={'pill' + (kind === k ? ' on' : '')} aria-pressed={kind === k} onClick={() => set({ kind: k === 'tv' ? 'tv' : '', genre: '', list: '' })}>{l}</button>
              ))}
            </div>
            {(genre > 0 || sort) && (
              <div role="group" aria-label={t('sort')} style={{ display: 'flex', gap: 4, marginLeft: 'auto' }}>
                {sorts.map(([k, r, e]) => <button key={k} className={'pill' + (sort === k ? ' on' : ' ghost')} aria-pressed={sort === k} onClick={() => set({ sort: k, genre: genre || '' })}>{tr(r, e)}</button>)}
              </div>
            )}
          </div>
          {genres.data && genres.data.length > 0 && (
            <div className="scroll-x" role="group" aria-label={tr('Жанры', 'Genres')} style={{ gap: 6 }}>
              <button className={'pill' + (!genre && !list ? ' on' : ' ghost')} onClick={() => set({ genre: '', list: '', sort: '' })} style={{ flex: 'none' }}>{tr('Подборки', 'Featured')}</button>
              {genres.data.map((g) => (
                <button key={g.id} className={'pill' + (genre === g.id ? ' on' : ' ghost')} aria-pressed={genre === g.id} onClick={() => set({ genre: g.id, list: '' })} style={{ flex: 'none' }}>{g.name}</button>
              ))}
            </div>
          )}
        </div>
      )}

      {genres.error && <CatalogError msg={genres.error} />}
      {!genres.error && (grid ? <Grid key={grid.path} path={grid.path} title={grid.title} /> : rows[kind].map(([l, r, e]) => (
        <Row key={kind + l} path={`/api/catalog${qs({ kind, list: l })}`} title={tr(r, e)} more={`/${qs({ kind: kind === 'tv' ? 'tv' : '', list: l })}`} />
      )))}

      <p style={{ margin: 0, fontSize: 'calc(12px * var(--fs))', color: 'var(--mut)' }}>{tr('Данные и постеры — TMDB. Раздачи ищутся в источниках из настроек: JacRed или ваш Jackett / Prowlarr.', 'Data and posters from TMDB. Releases are searched on the sources in settings: JacRed or your own Jackett / Prowlarr.')}</p>
    </main>
  );
}

// Spotlight: trending titles with artwork as the featured carousel.
function Spotlight({ kind }: { kind: Kind }) {
  const { data } = useAsync(() => api.get<CatalogPage>(`/api/catalog${qs({ kind, list: 'trending' })}`), [kind]);
  const slides = (data?.items || []).filter((x) => x.backdrop).slice(0, 6).map((x) => ({
    key: x.kind + x.id, name: x.title, backdrop: x.backdrop.replace('/w780/', '/w1280/'), overview: x.overview,
    meta: [x.year || '', x.kind === 'tv' ? tr('Сериал', 'Show') : tr('Фильм', 'Movie')].filter(Boolean).join(' · '),
    badges: <>{x.rating > 0 && <Badge>★ {x.rating.toFixed(1)}</Badge>}<Badge accent>{tr('В тренде', 'Trending')}</Badge></>,
    actions: <Link className="btn btn-w" to={`/catalog/${x.kind}/${x.id}`}><PlayGlyph />{t('watch')}</Link>,
  }));
  if (!data) return <div style={{ height: 'clamp(340px, 42vw, 460px)', borderRadius: 20, background: 'var(--bg2)', flex: 'none' }} />;
  return <Featured slides={slides} label={tr('В тренде', 'Trending')} />;
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
    <Link to={`/catalog/${x.kind}/${x.id}`} className="poster">
      <Art className="pimg" src={x.poster} title={x.title} style={{ aspectRatio: '2/3', borderRadius: 12 }}
        label={
          <>
            {!x.poster && <span className="disp" style={{ position: 'absolute', left: 12, right: 12, bottom: 14, color: '#FFFFFF', fontSize: 'calc(20px * var(--fs))', fontWeight: 600, lineHeight: 1 }}>{x.title}</span>}
            {x.rating > 0 && <span className="tag" style={{ position: 'absolute', top: 8, left: 8, background: x.rating >= 7 ? '#2F9E62' : x.rating >= 5.5 ? '#000000A0' : '#8E3B4F', color: '#FFFFFF' }}>{x.rating.toFixed(1)}</span>}
          </>
        } />
      <div>
        <div style={{ fontSize: 'calc(14px * var(--fs))', fontWeight: 700, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }} title={x.title}>{x.title}</div>
        <div className="sub">{[x.year || '', showKind ? (x.kind === 'tv' ? tr('сериал', 'show') : tr('фильм', 'movie')) : ''].filter(Boolean).join(' · ')}</div>
      </div>
    </Link>
  );
}
