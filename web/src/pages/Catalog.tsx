import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api, ApiError, qs, type CatalogItem, type CatalogPage } from '../api';
import { useAsync } from '../state';
import { t, tr } from '../i18n';
import { Art } from '../components/Art';
import { Icon } from '../components/Icon';
import { Section } from '../components/Shell';

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

// Catalog: all of TMDB, not only the local library. A title opens a page
// where it can be streamed or downloaded from the user's own indexers.
export default function Catalog() {
  const [params, setParams] = useSearchParams();
  const kind: Kind = params.get('kind') === 'tv' ? 'tv' : 'movie';
  const list = params.get('list') || '';
  const genre = Number(params.get('genre') || 0);
  const sort = params.get('sort') || '';
  const q = params.get('q') || '';
  const [text, setText] = useState(q);
  useEffect(() => setText(q), [q]);
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

  return (
    <main className="main pad">
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', gap: 16 }}>
        <div>
          <h1 className="h1">{t('catalog')}</h1>
          <div className="sub">{tr('Все фильмы и сериалы TMDB — смотрите сразу во время загрузки или скачивайте в библиотеку', 'Every movie and show on TMDB — watch while it downloads or save it to the library')}</div>
        </div>
        <form role="search" className="glass" onSubmit={(e) => { e.preventDefault(); set({ q: text.trim(), list: '', genre: '', sort: '' }); }}
          style={{ marginLeft: 'auto', flex: '0 1 360px', display: 'flex', alignItems: 'center', gap: 10, height: 44, padding: '0 16px', borderRadius: 999, border: '1px solid var(--line)', color: 'var(--mut)' }}>
          <Icon name="search" size={18} />
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder={tr('Найти в каталоге', 'Search the catalog')} aria-label={tr('Поиск в каталоге', 'Search the catalog')}
            style={{ flex: 1, minWidth: 0, border: 0, outline: 'none', background: 'transparent', font: "500 14px 'Inter',sans-serif", color: 'var(--fg)' }} />
          {q && <button type="button" className="ri" style={{ width: 28, height: 28 }} aria-label={tr('Очистить', 'Clear')} onClick={() => set({ q: '' })}><Icon name="close" size={14} /></button>}
        </form>
      </div>

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
        <Row key={kind + l} path={`/api/catalog${qs({ kind, list: l })}`} title={tr(r, e)} more={`/catalog${qs({ kind: kind === 'tv' ? 'tv' : '', list: l })}`} />
      )))}

      <p style={{ margin: 0, fontSize: 12, color: 'var(--mut)' }}>{tr('Данные и постеры — TMDB. Раздачи ищутся в источниках из настроек: JacRed или ваш Jackett / Prowlarr.', 'Data and posters from TMDB. Releases are searched on the sources in settings: JacRed or your own Jackett / Prowlarr.')}</p>
    </main>
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
    <Link to={`/catalog/${x.kind}/${x.id}`} className="poster">
      <Art className="pimg" src={x.poster} title={x.title} style={{ aspectRatio: '2/3', borderRadius: 12 }}
        label={
          <>
            {!x.poster && <span className="disp" style={{ position: 'absolute', left: 12, right: 12, bottom: 14, color: '#FFFFFF', fontSize: 20, fontWeight: 600, lineHeight: 1 }}>{x.title}</span>}
            {x.rating > 0 && <span className="tag" style={{ position: 'absolute', top: 8, left: 8, background: x.rating >= 7 ? '#2F9E62' : x.rating >= 5.5 ? '#000000A0' : '#8E3B4F', color: '#FFFFFF' }}>{x.rating.toFixed(1)}</span>}
          </>
        } />
      <div>
        <div style={{ fontSize: 14, fontWeight: 700, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }} title={x.title}>{x.title}</div>
        <div className="sub">{[x.year || '', showKind ? (x.kind === 'tv' ? tr('сериал', 'show') : tr('фильм', 'movie')) : ''].filter(Boolean).join(' · ')}</div>
      </div>
    </Link>
  );
}
