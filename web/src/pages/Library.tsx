import { Link, useParams, useSearchParams } from 'react-router-dom';
import { api, qs, type Title } from '../api';
import { useAsync } from '../state';
import { t, tr } from '../i18n';
import { TopBar } from '../components/TopBar';
import { PosterCard } from '../components/PosterCard';
import { Featured, type Slide } from '../components/Featured';
import { ContinueSection, ScanBar, localSlides, useHomeData } from '../components/LibraryParts';
import { Icon, PlayGlyph } from '../components/Icon';
import { useApp } from '../state';

const titles: Record<string, string> = { movies: 'movies', shows: 'shows', favorites: 'favorites', later: 'later' };

// Library grid: movies, shows or a personal list, filtered through URL params.
export default function Library() {
  const { kind = 'movies' } = useParams();
  const [params, setParams] = useSearchParams();
  const genre = params.get('genre') || '';
  const watched = params.get('watched') || '';
  const sort = params.get('sort') || 'added';
  const year = params.get('year') || '';
  const list = kind === 'favorites' || kind === 'later' ? kind : '';
  const k = kind === 'movies' ? 'movie' : kind === 'shows' ? 'show' : '';
  const { data, error, loading } = useAsync(
    () => api.get<{ items: Title[]; total: number }>('/api/titles' + qs({ kind: k, list, genre, watched, sort, year })),
    [kind, genre, watched, sort, year],
  );
  const genres = useAsync(() => api.get<{ name: string; count: number }[] | string[]>('/api/genres'), []);
  const set = (key: string, v: string) => {
    const n = new URLSearchParams(params);
    if (v) n.set(key, v); else n.delete(key);
    setParams(n, { replace: true });
  };
  const gnames = (genres.data || []).map((g: any) => (typeof g === 'string' ? g : g.name));
  // Movies and Shows open with the local featured titles and what is in progress.
  const { openCast, scan } = useApp();
  const local = !list;
  const home = useHomeData();
  const filtering = !!(genre || watched || year);
  const slides: Slide[] = !local || filtering ? [] : k === 'movie'
    ? localSlides((home.data?.featured || []).filter((x) => x.kind === 'movie'), openCast)
    : (data?.items || []).filter((x) => x.backdrop).slice(0, 5).map((x) => ({
      key: String(x.id), name: x.title, backdrop: x.backdrop, overview: x.overview,
      meta: [x.year || '', ...x.genres.slice(0, 2), x.episodes ? `${x.episodes} ${t('episodes')}` : ''].filter(Boolean).join(' · '),
      actions: <Link className="btn btn-w" to={`/show/${x.id}`}><PlayGlyph />{t('watch')}</Link>,
    }));
  const cont = local && !filtering ? (home.data?.continue || []).filter((x) => (k === 'movie' ? x.kind === 'movie' : x.kind === 'episode')) : [];
  const st = scan || home.data?.scan;
  const libEmpty = local && st && st.movies + st.shows === 0 && !st.scanning;

  return (
    <main className="main">
      <TopBar types={false} />
      {libEmpty && (
        <div className="empty glass">
          <Icon name="folder" size={32} />
          <div style={{ fontWeight: 700, color: 'var(--fg)', fontSize: 'calc(16px * var(--fs))' }}>{t('libEmpty')}</div>
          <div>{t('libEmptyHint')}</div>
          <Link className="btn btn-r" to="/settings#lib"><Icon name="plus" size={16} />{t('addFolder')}</Link>
        </div>
      )}
      {slides.length > 0 && <Featured slides={slides} label={tr('Рекомендуемое', 'Featured')} />}
      <ContinueSection items={cont} />
      <div style={{ display: 'flex', alignItems: 'flex-end', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <h1 className="h1">{t(titles[kind] || 'movies')}</h1>
          <div className="sub">{data ? `${data.total} ${tr('в библиотеке', 'in library')}` : ' '}</div>
        </div>
        <div className="row" style={{ marginLeft: 'auto' }}>
          <label className="sr" htmlFor="f-genre">{t('genre')}</label>
          <select id="f-genre" className="sel" value={genre} onChange={(e) => set('genre', e.target.value)}>
            <option value="">{t('anyGenre')}</option>
            {gnames.map((g) => <option key={g} value={g}>{g}</option>)}
          </select>
          <label className="sr" htmlFor="f-year">{tr('Год', 'Year')}</label>
          <input id="f-year" className="inp" style={{ width: 90 }} inputMode="numeric" placeholder={tr('Год', 'Year')} value={year} onChange={(e) => set('year', e.target.value.replace(/\D/g, '').slice(0, 4))} />
          <div role="group" aria-label={tr('Просмотр', 'Watched')} style={{ display: 'flex', gap: 4 }}>
            {[['', t('all2')], ['no', t('unwatched')], ['yes', t('watchedF')]].map(([v, l]) => (
              <button key={v} className={'pill' + (watched === v ? ' on' : '')} aria-pressed={watched === v} onClick={() => set('watched', v)}>{l}</button>
            ))}
          </div>
          <label className="sr" htmlFor="f-sort">{t('sort')}</label>
          <select id="f-sort" className="sel" value={sort} onChange={(e) => set('sort', e.target.value === 'added' ? '' : e.target.value)}>
            <option value="added">{t('sortAdded')}</option>
            <option value="title">{t('sortTitle')}</option>
            <option value="year">{t('sortYear')}</option>
            <option value="rating">{t('sortRating')}</option>
          </select>
        </div>
      </div>
      {error && <div className="err">{error}</div>}
      {loading && !data && <span className="spin" />}
      {data && data.items.length === 0 && !libEmpty && (
        <div className="empty">
          {list ? tr('Список пока пуст. Добавляйте фильмы кнопкой «В список» или ♥ на странице фильма.', 'This list is empty. Add titles from their pages.') : t('nothing')}
          {!list && !genre && !watched && !year && <Link className="btn btn-d sm" to="/settings#lib">{t('addFolder')}</Link>}
        </div>
      )}
      <div className="grid-posters">
        {data?.items.map((x) => <PosterCard key={x.kind + x.id} x={x} />)}
      </div>
      {local && st && <ScanBar st={st} />}
    </main>
  );
}
