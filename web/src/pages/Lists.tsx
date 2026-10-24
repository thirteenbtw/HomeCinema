import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { api, ApiError, type CatalogItem, type Title } from '../api';
import { useApp, useAsync } from '../state';
import { tr } from '../i18n';
import { TopBar } from '../components/TopBar';
import { PosterCard } from '../components/PosterCard';
import { Section } from '../components/Shell';
import { Icon } from '../components/Icon';
import { stars } from '../components/Marks';
import { CatalogCard } from './Catalog';
import { loadMarks, markLabels, type Mark, type MarkList } from '../marks';

const lists: MarkList[] = ['liked', 'later', 'watched'];
const libList: Partial<Record<MarkList, string>> = { liked: 'favorites', later: 'later' };

// Lists: what the profile liked, wants to watch later or has seen, from the
// catalog (any TMDB title) and from the library, plus Letterboxd.
export default function Lists() {
  const [params, setParams] = useSearchParams();
  const list = (lists.includes(params.get('list') as MarkList) ? params.get('list') : 'liked') as MarkList;
  const kind = params.get('kind') || '';
  const sort = params.get('sort') || '';
  const { data, error, reload } = useAsync(() => api.get<Mark[]>(`/api/marks?list=${list}`), [list]);
  const lib = useAsync(() => (libList[list] ? api.get<{ items: Title[] }>(`/api/titles?list=${libList[list]}`) : Promise.resolve({ items: [] })), [list]);
  useEffect(() => {
    const f = (e: Event) => { if (!(e as CustomEvent).detail?.running) { reload(); loadMarks(); } };
    window.addEventListener('hc:letterboxd', f);
    return () => window.removeEventListener('hc:letterboxd', f);
  }, [reload]);
  const set = (k: string, v: string) => { const n = new URLSearchParams(params); if (v) n.set(k, v); else n.delete(k); setParams(n, { replace: true }); };

  // Titles toggled off on this page stay until it reloads, so an accidental
  // click can be undone in place.
  const items = useMemo(() => {
    let xs = (data || []).filter((m) => !kind || m.kind === kind);
    if (sort === 'title') xs = [...xs].sort((a, b) => a.title.localeCompare(b.title));
    if (sort === 'year') xs = [...xs].sort((a, b) => b.year - a.year);
    if (sort === 'rating') xs = [...xs].sort((a, b) => b.rating - a.rating);
    return xs;
  }, [data, kind, sort]);

  return (
    <main className="main">
      <TopBar types={false} />
      <div style={{ display: 'flex', alignItems: 'flex-end', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <h1 className="h1">{tr('Мои списки', 'My lists')}</h1>
          <div className="sub">{data ? `${items.length} ${tr('в списке', 'in the list')}` : ' '}</div>
        </div>
        <div className="row" style={{ marginLeft: 'auto' }}>
          <div role="tablist" aria-label={tr('Список', 'List')} style={{ display: 'flex', gap: 4 }}>
            {lists.map((l) => <button key={l} role="tab" aria-selected={list === l} className={'pill' + (list === l ? ' on' : '')} onClick={() => set('list', l === 'liked' ? '' : l)}>{tr(...markLabels[l])}</button>)}
          </div>
          <div role="group" aria-label={tr('Тип', 'Type')} style={{ display: 'flex', gap: 4 }}>
            {[['', tr('Все', 'All')], ['movie', tr('Фильмы', 'Movies')], ['tv', tr('Сериалы', 'Shows')]].map(([v, l]) => (
              <button key={v} className={'pill' + (kind === v ? ' on' : ' ghost')} aria-pressed={kind === v} onClick={() => set('kind', v)}>{l}</button>
            ))}
          </div>
          <label className="sr" htmlFor="l-sort">{tr('Сортировка', 'Sort')}</label>
          <select id="l-sort" className="sel" value={sort} onChange={(e) => set('sort', e.target.value)}>
            <option value="">{tr('Недавно добавленные', 'Recently added')}</option>
            <option value="title">{tr('По названию', 'Title')}</option>
            <option value="year">{tr('По году', 'Year')}</option>
            <option value="rating">{tr('По моей оценке', 'My rating')}</option>
          </select>
        </div>
      </div>
      {error && <div className="err">{error}</div>}
      {!data && !error && <span className="spin" />}
      {data && items.length === 0 && (
        <div className="empty">
          {tr('Пока пусто. Отмечайте фильмы кнопками ♥ и «Смотреть позже» на постерах и страницах фильмов — или импортируйте их из Letterboxd ниже.', 'Nothing here yet. Mark titles with ♥ and “Watch later” on posters and title pages, or import them from Letterboxd below.')}
          <Link className="btn btn-l sm" to="/">{tr('В каталог', 'Browse the catalog')}</Link>
        </div>
      )}
      <div className="grid-posters">
        {items.map((m) => (
          <div key={m.kind + m.id} style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <CatalogCard x={toItem(m)} showKind={!kind} />
            {m.rating > 0 && <div className="sub" style={{ color: '#00A945', fontWeight: 700 }} title={tr('Ваша оценка', 'Your rating')}>{stars(m.rating)}</div>}
          </div>
        ))}
      </div>
      {(lib.data?.items.length || 0) > 0 && (
        <Section title={tr('Из библиотеки', 'From the library')} note={tr('Отмечено на страницах фильмов в библиотеке', 'Marked on library title pages')}>
          <div className="grid-posters">{lib.data!.items.map((x) => <PosterCard key={x.kind + x.id} x={x} />)}</div>
        </Section>
      )}
      <LetterboxdPanel onDone={() => { reload(); loadMarks(); }} />
    </main>
  );
}

function toItem(m: Mark): CatalogItem {
  return { kind: m.kind, id: m.id, title: m.title || m.origTitle, origTitle: m.origTitle, year: m.year, poster: m.poster, backdrop: '', overview: '', rating: 0, votes: 0, genreIds: [] };
}

type Job = { running: boolean; kind: 'import' | 'sync'; done: number; total: number; found: number; missed: string[]; error?: string; at: number; profileId: number };
type LB = { user: string; syncedAt: number; job: Job | null };

function LetterboxdPanel({ onDone }: { onDone: () => void }) {
  const { toast, state } = useApp();
  const { data, setData } = useAsync(() => api.get<LB>('/api/letterboxd'), []);
  const [user, setUser] = useState('');
  const [fallback, setFallback] = useState<MarkList | ''>('');
  const [pending, setPending] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const file = useRef<HTMLInputElement>(null);
  useEffect(() => { if (data) setUser(data.user); }, [data?.user]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const f = (e: Event) => {
      const job = (e as CustomEvent<Job>).detail;
      if (job.profileId !== state?.profileId) return;
      setData((d) => (d ? { ...d, job, syncedAt: !job.running && job.kind === 'sync' && !job.error ? Math.floor(Date.now() / 1000) : d.syncedAt } : d));
      if (!job.running) onDone();
    };
    window.addEventListener('hc:letterboxd', f);
    return () => window.removeEventListener('hc:letterboxd', f);
  }, [state?.profileId]); // eslint-disable-line react-hooks/exhaustive-deps

  const job = data?.job;
  const running = !!job?.running;
  const upload = async (f: File, list: MarkList | '') => {
    setBusy(true);
    try {
      const fd = new FormData();
      fd.append('file', f);
      if (list) fd.append('list', list);
      await api.post('/api/letterboxd/import', fd);
      setPending(null);
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) { setPending(f); setFallback('later'); } else toast((e as Error).message);
    } finally { setBusy(false); }
  };
  const saveUser = async () => {
    setBusy(true);
    try {
      const r = await api.put<{ user: string }>('/api/letterboxd', { user });
      setData((d) => (d ? { ...d, user: r.user } : d));
      setUser(r.user);
      if (r.user) toast(tr('Загружаем дневник из Letterboxd…', 'Loading the diary from Letterboxd…'));
    } catch (e) { toast((e as Error).message); } finally { setBusy(false); }
  };
  const sync = () => api.post('/api/letterboxd/sync').catch((e) => toast(e.message));
  const clear = async () => {
    if (!window.confirm(tr('Убрать из списков всё, что пришло из Letterboxd?', 'Remove everything imported from Letterboxd from your lists?'))) return;
    const r = await api.del<{ removed: number }>('/api/letterboxd/marks');
    toast(tr(`Убрано отметок: ${r.removed}`, `Removed: ${r.removed}`));
    onDone();
  };

  return (
    <Section title="Letterboxd" id="letterboxd" note={tr('Импорт и экспорт списков. У Letterboxd нет открытого API, поэтому это не двусторонняя синхронизация.', 'Import and export your lists. Letterboxd has no open API, so this is not a two-way sync.')}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 16 }}>
        <div className="card">
          <div style={{ fontWeight: 700 }}>{tr('Импорт всего профиля', 'Import your whole profile')}</div>
          <div className="sub" style={{ lineHeight: 1.5 }}>
            {tr('На letterboxd.com: Settings → Data → Export your data. Загрузите сюда полученный ZIP — просмотренные, оценки, лайки и watchlist лягут в «Просмотрено», «Нравится» и «Смотреть позже». Фильмы сопоставляются с TMDB по названию и году.', 'On letterboxd.com: Settings → Data → Export your data. Upload the ZIP here: watched films, ratings, likes and the watchlist go to Watched, Liked and Watch later. Films are matched to TMDB by title and year.')}
          </div>
          <input ref={file} type="file" accept=".zip,.csv,application/zip,text/csv" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) upload(f, ''); e.target.value = ''; }} />
          <div className="row">
            <button className="btn btn-r sm" disabled={busy || running} onClick={() => file.current?.click()}><Icon name="up" size={16} />{tr('Загрузить ZIP или CSV', 'Upload ZIP or CSV')}</button>
          </div>
          {pending && (
            <div className="row" role="group" aria-label={tr('Куда положить список', 'Which list')}>
              <span className="sub">{tr(`«${pending.name}» — это не файл из экспорта. Положить фильмы в:`, `“${pending.name}” is not an export file. Put its films in:`)}</span>
              <select className="sel" value={fallback} onChange={(e) => setFallback(e.target.value as MarkList)}>
                {lists.map((l) => <option key={l} value={l}>{tr(...markLabels[l])}</option>)}
              </select>
              <button className="btn btn-l sm" disabled={busy} onClick={() => upload(pending, fallback)}>{tr('Импортировать', 'Import')}</button>
            </div>
          )}
        </div>

        <div className="card">
          <div style={{ fontWeight: 700 }}>{tr('Подтягивать новые просмотры', 'Pull new diary entries')}</div>
          <div className="sub" style={{ lineHeight: 1.5 }}>
            {tr('По публичной RSS-ленте профиля: последние ~50 записей дневника с оценками и лайками. Проверяется каждые 6 часов. Watchlist в ленте нет — его переносит только импорт ZIP.', 'From the profile’s public RSS feed: the last ~50 diary entries with ratings and likes, checked every 6 hours. The watchlist is not in the feed; only the ZIP import brings it.')}
          </div>
          <div className="row">
            <label className="sr" htmlFor="lb-user">{tr('Имя пользователя Letterboxd', 'Letterboxd username')}</label>
            <input id="lb-user" className="inp" style={{ flex: '1 1 160px' }} placeholder={tr('имя или ссылка на профиль', 'username or profile link')} value={user} onChange={(e) => setUser(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') saveUser(); }} />
            <button className="btn btn-l sm" disabled={busy || user === (data?.user || '')} onClick={saveUser}>{tr('Сохранить', 'Save')}</button>
            {data?.user && <button className="btn btn-d sm" disabled={running} onClick={sync}><Icon name="refresh" size={16} />{tr('Обновить', 'Sync')}</button>}
          </div>
          {data?.user && <div className="sub">{data.syncedAt ? `${tr('Обновлено', 'Synced')} ${new Date(data.syncedAt * 1000).toLocaleString()}` : tr('Ещё не обновлялось', 'Not synced yet')}</div>}
        </div>

        <div className="card">
          <div style={{ fontWeight: 700 }}>{tr('Перенести в Letterboxd', 'Move to Letterboxd')}</div>
          <div className="sub" style={{ lineHeight: 1.5 }}>
            {tr('CSV в формате импорта Letterboxd (с ID TMDB, поэтому фильмы находятся точно). «Просмотренные» загрузите на letterboxd.com/import, «Смотреть позже» — в импорт watchlist. Лайков в формате импорта нет: такие фильмы получат тег liked. Сериалы Letterboxd не хранит.', 'CSV in Letterboxd’s import format (with TMDB IDs, so films match exactly). Upload “watched” at letterboxd.com/import and “watch later” in the watchlist import. The format has no likes, so liked films get the tag “liked”. Letterboxd does not keep shows.')}
          </div>
          <div className="row">
            <a className="btn btn-l sm" href="/api/letterboxd/export/films.csv" download><Icon name="download" size={16} />{tr('Просмотренные и лайки', 'Watched and liked')}</a>
            <a className="btn btn-l sm" href="/api/letterboxd/export/watchlist.csv" download><Icon name="download" size={16} />{tr('Смотреть позже', 'Watch later')}</a>
          </div>
          <button className="btn btn-d sm" style={{ alignSelf: 'flex-start' }} onClick={clear}><Icon name="trash" size={16} />{tr('Убрать импортированное', 'Remove imported')}</button>
        </div>
      </div>

      {job && (
        <div className="card" aria-live="polite">
          {running ? (
            <div className="row"><span className="spin" />{job.kind === 'sync' ? tr('Читаем ленту Letterboxd…', 'Reading the Letterboxd feed…') : tr('Сопоставляем с TMDB…', 'Matching with TMDB…')} {job.total > 0 && `${job.done} / ${job.total}`}</div>
          ) : job.error ? (
            <div className="err">{job.error}</div>
          ) : (
            <div>{tr(`Готово: добавлено ${job.found} из ${job.total}.`, `Done: ${job.found} of ${job.total} added.`)}</div>
          )}
          {job.total > 0 && running && <div style={{ height: 4, background: 'var(--line)', borderRadius: 2 }}><div style={{ height: 4, borderRadius: 2, width: `${(job.done / job.total) * 100}%`, background: 'var(--acc)' }} /></div>}
          {!running && job.missed.length > 0 && (
            <details>
              <summary className="sub" style={{ cursor: 'pointer' }}>{tr(`Не нашлись в TMDB: ${job.total - job.found}`, `Not found on TMDB: ${job.total - job.found}`)}</summary>
              <div className="sub" style={{ lineHeight: 1.6, marginTop: 6 }}>{job.missed.join(' · ')}</div>
            </details>
          )}
        </div>
      )}
    </Section>
  );
}
