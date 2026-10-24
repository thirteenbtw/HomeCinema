import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams, useParams } from 'react-router-dom';
import { api, type CastMember, type CatalogItem, type Person, type PersonCard, type PersonCredit } from '../api';
import { useAsync } from '../state';
import { getLang, t, tr } from '../i18n';
import { Backdrop } from '../components/Art';
import { Icon } from '../components/Icon';
import { Section } from '../components/Shell';
import { TopBar } from '../components/TopBar';
import { previewProps } from '../components/Preview';
import { CatalogCard } from './Catalog';

// People from TMDB: a person's page in the design's style (photo and
// biography, what is coming, what they are known for, the whole
// filmography), the popular people list, and a lookup by name for cast
// saved before people had TMDB ids.

export function personLink(a: CastMember) {
  const id = a.id || a.tmdbId;
  return id ? `/person/${id}` : `/person/find?name=${encodeURIComponent(a.name)}`;
}

const fmtDate = (d: string, opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'long', year: 'numeric' }) => {
  const x = new Date(d + 'T00:00:00');
  return isNaN(+x) ? d : x.toLocaleDateString(getLang() === 'en' ? 'en-US' : 'ru-RU', opts);
};

function age(from: string, to?: string) {
  const a = new Date(from + 'T00:00:00'), b = to ? new Date(to + 'T00:00:00') : new Date();
  if (isNaN(+a) || isNaN(+b)) return 0;
  let y = b.getFullYear() - a.getFullYear();
  if (b.getMonth() < a.getMonth() || (b.getMonth() === a.getMonth() && b.getDate() < a.getDate())) y--;
  return y;
}

function yearsWord(n: number) {
  if (getLang() === 'en') return n === 1 ? 'year' : 'years';
  const m10 = n % 10, m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return 'год';
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return 'года';
  return 'лет';
}

function deptLabel(dept: string, gender: number) {
  const f = gender === 1;
  switch (dept) {
    case 'Acting': return tr(f ? 'Актриса' : 'Актёр', f ? 'Actress' : 'Actor');
    case 'Directing': return tr('Режиссёр', 'Director');
    case 'Writing': return tr('Сценарист', 'Writer');
    case 'Production': return tr('Продюсер', 'Producer');
    case 'Sound': return tr('Композитор', 'Composer');
    case 'Camera': return tr('Оператор', 'Cinematographer');
    default: return dept;
  }
}

export default function PersonPage() {
  const { id } = useParams();
  const { data, error } = useAsync(() => api.get<Person>(`/api/catalog/person/${id}`), [id]);
  return (
    <main className="main">
      <TopBar catalog />
      {error && <div className="empty">{error}</div>}
      {!data && !error && <span className="spin" />}
      {data && <Detail key={data.id} p={data} />}
    </main>
  );
}

type Filter = 'all' | 'acting' | 'directing' | 'other';
type Sort = 'year' | 'rating' | 'popular';

function Detail({ p }: { p: Person }) {
  const nav = useNavigate();
  const [more, setMore] = useState(false);
  const [filter, setFilter] = useState<Filter>('all');
  const [sort, setSort] = useState<Sort>('year');
  const label = deptLabel(p.dept, p.gender);
  const movies = p.credits.filter((c) => c.kind === 'movie').length;
  const shows = p.credits.length - movies;
  const counts: Record<Filter, number> = { all: p.credits.length, acting: 0, directing: 0, other: 0 };
  p.credits.forEach((c) => { (c.depts || [c.dept]).forEach((d) => { counts[d]++; }); });
  const longBio = p.bio.length > 420;

  useEffect(() => { document.title = p.name; }, [p.name]);

  return (
    <>
      <section className="hero" aria-label={p.name} style={{ position: 'relative', overflow: 'hidden', borderRadius: 20, background: '#16171A', color: '#FFFFFF', boxShadow: '0 20px 50px var(--sh)', minHeight: 'clamp(340px, 30vw, 480px)', display: 'flex', alignItems: 'flex-end' }}>
        {p.backdrop && <Backdrop src={p.backdrop} />}
        <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(90deg,#000000E0 0%,#000000A6 50%,#00000040 100%), linear-gradient(0deg,#000000A0,transparent 60%)' }} />
        <div style={{ position: 'relative', display: 'flex', gap: 'clamp(20px, 3vw, 40px)', alignItems: 'flex-end', padding: 'clamp(20px, 3vw, 40px)', flexWrap: 'wrap', width: '100%' }}>
          <div className="ph-photo">
            {p.photo ? <img src={p.photo} alt={p.name} /> : <div style={{ height: '100%', display: 'grid', placeItems: 'center', color: '#FFFFFF59' }}><Icon name="user" size={56} /></div>}
          </div>
          <div style={{ flex: '1 1 340px', display: 'flex', flexDirection: 'column', gap: 12, maxWidth: 760 }}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
              <button onClick={() => nav(-1)} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, height: 28, padding: '0 10px', borderRadius: 999, background: '#FFFFFF2E', color: '#FFFFFF', border: 0, cursor: 'pointer', font: "600 calc(12px * var(--fs)) 'Inter',sans-serif" }}><Icon name="left" size={14} />{t('back')}</button>
              {label && <span style={{ fontSize: 'calc(11px * var(--fs))', fontWeight: 700, padding: '4px 10px', borderRadius: 999, background: 'var(--acc)' }}>{label}</span>}
            </div>
            <h1 className="disp" style={{ margin: 0, fontSize: 'calc(clamp(40px,6vw,76px) * var(--fs))', lineHeight: 0.92, fontWeight: 600, letterSpacing: '.04em', overflowWrap: 'anywhere' }}>{p.name}</h1>
            <div style={{ display: 'flex', gap: 14, flexWrap: 'wrap', fontSize: 'calc(13px * var(--fs))', fontWeight: 600, color: '#D4D7DC' }}>
              {p.birthday && <span>{tr('Родился', 'Born')} {fmtDate(p.birthday)}{!p.deathday && ` · ${age(p.birthday)} ${yearsWord(age(p.birthday))}`}</span>}
              {p.deathday && <span>{tr('Умер', 'Died')} {fmtDate(p.deathday)}{p.birthday && ` · ${age(p.birthday, p.deathday)} ${yearsWord(age(p.birthday, p.deathday))}`}</span>}
              {p.place && <span>{p.place}</span>}
            </div>
            {p.bio && (
              <div>
                <p style={{ margin: 0, fontSize: 'calc(14px * var(--fs))', lineHeight: 1.6, color: '#D4D7DC', whiteSpace: 'pre-line', ...(more ? {} : { display: '-webkit-box', WebkitLineClamp: 4, WebkitBoxOrient: 'vertical', overflow: 'hidden' }) }}>{p.bio}</p>
                {longBio && <button onClick={() => setMore(!more)} style={{ marginTop: 6, padding: 0, border: 0, background: 'none', color: '#FFFFFF', font: "700 calc(13px * var(--fs)) 'Inter',sans-serif", cursor: 'pointer', textDecoration: 'underline', textUnderlineOffset: 3 }}>{more ? tr('Свернуть', 'Less') : tr('Читать полностью', 'Read more')}</button>}
              </div>
            )}
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center', fontSize: 'calc(12px * var(--fs))', fontWeight: 700 }}>
              {movies > 0 && <span style={{ padding: '5px 10px', borderRadius: 999, background: '#FFFFFF24' }}>{movies} {tr('фильмов', 'movies')}</span>}
              {shows > 0 && <span style={{ padding: '5px 10px', borderRadius: 999, background: '#FFFFFF24' }}>{shows} {tr('сериалов', 'shows')}</span>}
              {p.imdbId && <a href={`https://www.imdb.com/name/${p.imdbId}/`} target="_blank" rel="noreferrer" style={{ color: '#FFFFFF', marginLeft: 6 }}>IMDb ↗</a>}
              <a href={`https://www.themoviedb.org/person/${p.id}`} target="_blank" rel="noreferrer" style={{ color: '#FFFFFF' }}>TMDB ↗</a>
            </div>
          </div>
        </div>
      </section>

      {p.upcoming.length > 0 && (
        <Section title={tr('Скоро', 'Coming soon')} note={tr('Анонсированные проекты', 'Announced projects')}>
          <div className="scroll-x" style={{ gap: 12 }}>
            {p.upcoming.slice(0, 12).map((c) => <Upcoming key={c.kind + c.id} c={c} />)}
          </div>
        </Section>
      )}

      {p.knownFor.length > 0 && (
        <Section title={tr('Известен по', 'Known for')}>
          <div className="scroll-x">
            {p.knownFor.map((x) => <div key={x.kind + x.id} style={{ width: 150, flex: 'none' }}><CatalogCard x={x} showKind /></div>)}
          </div>
        </Section>
      )}

      {p.credits.length > 0 && (
        <Section title={tr('Фильмография', 'Filmography')} note={`${p.credits.length} ${tr('работ', 'credits')}`}>
          <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', justifyContent: 'space-between' }}>
            <div role="group" aria-label={tr('Роль', 'Role')} style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
              {([['all', tr('Все', 'All')], ['acting', tr('Актёр', 'Actor')], ['directing', tr('Режиссёр', 'Director')], ['other', tr('Другое', 'Other')]] as [Filter, string][])
                .filter(([k]) => k === 'all' || counts[k] > 0)
                .map(([k, l]) => <button key={k} className={'pill' + (filter === k ? ' on' : ' ghost')} aria-pressed={filter === k} onClick={() => setFilter(k)}>{l} <span style={{ opacity: 0.6 }}>{counts[k]}</span></button>)}
            </div>
            <div role="group" aria-label={t('sort')} style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
              {([['year', tr('По году', 'By year')], ['rating', tr('По рейтингу', 'By rating')], ['popular', tr('По популярности', 'By popularity')]] as [Sort, string][])
                .map(([k, l]) => <button key={k} className={'pill' + (sort === k ? ' on' : ' ghost')} aria-pressed={sort === k} onClick={() => setSort(k)}>{l}</button>)}
            </div>
          </div>
          <Filmography credits={p.credits} filter={filter} sort={sort} />
        </Section>
      )}
    </>
  );
}

function Upcoming({ c }: { c: PersonCredit }) {
  return (
    <Link to={`/catalog/${c.kind}/${c.id}`} className="poster" style={{ flex: 'none', width: 280 }} {...previewProps(c)}>
      <div className="pimg" style={{ position: 'relative', aspectRatio: '16/9', borderRadius: 12, overflow: 'hidden', background: '#202126' }}>
        {(c.backdrop || c.poster) && <img src={c.backdrop || c.poster} alt="" loading="lazy" style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover' }} />}
        <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(transparent 35%, #000000C8)' }} />
        <span style={{ position: 'absolute', left: 10, top: 10, fontSize: 'calc(11px * var(--fs))', fontWeight: 700, padding: '3px 8px', borderRadius: 999, background: 'var(--acc)', color: '#FFFFFF' }}>
          {c.date ? fmtDate(c.date, { day: 'numeric', month: 'short', year: 'numeric' }) : tr('Дата не объявлена', 'Date TBA')}
        </span>
        <span className="disp" style={{ position: 'absolute', left: 12, right: 12, bottom: 12, fontSize: 'calc(20px * var(--fs))', fontWeight: 600, lineHeight: 1, color: '#FFFFFF', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.title}</span>
      </div>
      <div className="sub" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{c.role || (c.kind === 'tv' ? tr('Сериал', 'Show') : tr('Фильм', 'Movie'))}</div>
    </Link>
  );
}

function Filmography({ credits, filter, sort }: { credits: PersonCredit[]; filter: Filter; sort: Sort }) {
  const list = useMemo(() => {
    const l = credits.filter((c) => filter === 'all' || (c.depts || [c.dept]).includes(filter));
    const by = {
      year: (a: PersonCredit, b: PersonCredit) => (b.date || '9999').localeCompare(a.date || '9999'),
      rating: (a: PersonCredit, b: PersonCredit) => (b.votes >= 20 ? b.rating : 0) - (a.votes >= 20 ? a.rating : 0),
      popular: (a: PersonCredit, b: PersonCredit) => b.votes - a.votes,
    }[sort];
    return [...l].sort(by);
  }, [credits, filter, sort]);
  const [shown, setShown] = useState(60);
  useEffect(() => setShown(60), [filter, sort]);

  let decade = '';
  return (
    <div className="filmo">
      {list.slice(0, shown).map((c) => {
        const y = c.date ? c.date.slice(0, 4) : '';
        let head = null;
        if (sort === 'year') {
          const d = y ? `${y.slice(0, 3)}0-е` : tr('Скоро', 'Upcoming');
          const dl = getLang() === 'en' && y ? `${y.slice(0, 3)}0s` : d;
          if (dl !== decade) { decade = dl; head = <div className="filmo-dec">{dl}</div>; }
        }
        return (
          <div key={c.kind + c.id} style={{ display: 'contents' }}>
            {head}
            <Link to={`/catalog/${c.kind}/${c.id}`} className="filmo-row" {...previewProps(c as CatalogItem)}>
              <span className="mono" style={{ fontSize: 'calc(13px * var(--fs))', color: 'var(--mut)' }}>{y || '—'}</span>
              {c.poster ? <img className="pimg" src={c.poster.replace('/w342/', '/w185/')} alt="" loading="lazy" /> : <span className="ph0 pimg" />}
              <span style={{ minWidth: 0 }}>
                <span style={{ display: 'block', fontWeight: 700, fontSize: 'calc(14px * var(--fs))', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{c.title}</span>
                <span className="sub" style={{ display: 'block', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {[c.kind === 'tv' ? tr('Сериал', 'Show') : tr('Фильм', 'Movie'), c.dept === 'acting' ? (c.role ? tr(`роль: ${c.role}`, `as ${c.role}`) : '') : c.role].filter(Boolean).join(' · ')}
                </span>
              </span>
              <span className="fr" style={{ fontSize: 'calc(12px * var(--fs))', fontWeight: 700, color: c.rating >= 7 ? 'var(--ok-fg)' : 'var(--mut)' }}>{c.votes >= 20 && c.rating > 0 ? `★ ${c.rating.toFixed(1)}` : ''}</span>
            </Link>
          </div>
        );
      })}
      {list.length === 0 && <div className="filmo-row" style={{ gridTemplateColumns: '1fr', color: 'var(--mut)' }}>{t('nothing')}</div>}
      {shown < list.length && (
        <button className="filmo-row" onClick={() => setShown(shown + 60)} style={{ gridTemplateColumns: '1fr', border: 0, borderTop: '1px solid var(--line)', background: 'none', font: "700 calc(13px * var(--fs)) 'Inter',sans-serif", color: 'var(--acct)', cursor: 'pointer', textAlign: 'center' }}>
          {tr(`Показать ещё ${Math.min(60, list.length - shown)}`, `Show ${Math.min(60, list.length - shown)} more`)}
        </button>
      )}
    </div>
  );
}

// PersonFind opens a person by name (cast saved without TMDB ids).
export function PersonFind() {
  const [params] = useSearchParams();
  const name = params.get('name') || '';
  const nav = useNavigate();
  const [miss, setMiss] = useState(false);
  useEffect(() => {
    api.get<{ items: PersonCard[] }>(`/api/catalog/people?q=${encodeURIComponent(name)}`)
      .then((r) => { if (r.items[0]) nav(`/person/${r.items[0].id}`, { replace: true }); else setMiss(true); })
      .catch(() => setMiss(true));
  }, [name]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <main className="main">
      <TopBar catalog />
      {miss ? <div className="empty">{tr(`Не нашли «${name}» в TMDB`, `“${name}” was not found on TMDB`)}</div> : <span className="spin" />}
    </main>
  );
}

export function Face({ x }: { x: PersonCard }) {
  return (
    <Link to={`/person/${x.id}`} className="face">
      <div className="ph">
        {x.photo ? <img src={x.photo} alt="" loading="lazy" /> : <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', color: 'var(--mut)' }}><Icon name="user" size={40} /></div>}
      </div>
      <div>
        <div style={{ fontSize: 'calc(14px * var(--fs))', fontWeight: 700, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{x.name}</div>
        {x.knownFor && <div className="sub" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{x.knownFor}</div>}
      </div>
    </Link>
  );
}

// People: popular people, faces stay black and white until hovered.
export function PeoplePage() {
  const [items, setItems] = useState<PersonCard[]>([]);
  const [page, setPage] = useState(0);
  const [pages, setPages] = useState(1);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const load = (p: number) => {
    setLoading(true);
    api.get<{ items: PersonCard[]; pages: number }>(`/api/catalog/people?page=${p}`)
      .then((r) => { setItems((x) => (p === 1 ? r.items : [...x, ...r.items.filter((i) => !x.some((y) => y.id === i.id))])); setPage(p); setPages(r.pages); })
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false));
  };
  useEffect(() => load(1), []);
  return (
    <main className="main">
      <TopBar catalog />
      <Section title={tr('Популярные люди', 'Popular people')} note={tr('Наведите — фото станет цветным', 'Hover to colorize')}>
        {err && <div className="empty">{err}</div>}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(150px,1fr))', gap: 16 }}>
          {items.map((x) => <Face key={x.id} x={x} />)}
        </div>
        {loading && <span className="spin" />}
        {!loading && page < pages && items.length > 0 && <button className="btn btn-l sm" style={{ alignSelf: 'center' }} onClick={() => load(page + 1)}>{tr('Показать ещё', 'Show more')}</button>}
      </Section>
    </main>
  );
}

// PeopleRow: people matching a search, shown above the titles.
export function PeopleRow({ q }: { q: string }) {
  const { data } = useAsync(() => api.get<{ items: PersonCard[] }>(`/api/catalog/people?q=${encodeURIComponent(q)}`), [q]);
  const items = (data?.items || []).filter((x) => x.photo).slice(0, 12);
  if (!items.length) return null;
  return (
    <Section title={tr('Люди', 'People')}>
      <div className="scroll-x" style={{ gap: 14 }}>
        {items.map((x) => <div key={x.id} style={{ width: 130, flex: 'none' }}><Face x={x} /></div>)}
      </div>
    </Section>
  );
}
