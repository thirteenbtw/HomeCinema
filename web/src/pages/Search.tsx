import { Link, useSearchParams } from 'react-router-dom';
import { api, type Title } from '../api';
import { useAsync } from '../state';
import { t, tr } from '../i18n';
import { TopBar } from '../components/TopBar';
import { PosterCard } from '../components/PosterCard';
import { Section } from '../components/Shell';

type Person = { name: string; photo: string; titles: Title[] };

export default function Search() {
  const [params] = useSearchParams();
  const q = params.get('q') || '';
  const kind = params.get('kind') || '';
  const { data, error, loading } = useAsync(() => api.get<{ titles: Title[]; people: Person[] }>(`/api/search?q=${encodeURIComponent(q)}`), [q]);
  const titles = kind === 'people' ? [] : (data?.titles || []).filter((x) => !kind || x.kind === kind);
  const people = kind === '' || kind === 'people' ? data?.people || [] : [];
  return (
    <main className="main">
      <TopBar key={q + kind} />
      <h1 className="h1">{tr('Поиск', 'Search')}: «{q}»</h1>
      {error && <div className="err">{error}</div>}
      {loading && <span className="spin" />}
      {data && titles.length === 0 && people.length === 0 && (
        <div className="empty">{t('nothing')}<Link className="btn btn-l sm" to={`/torrents?tab=search&q=${encodeURIComponent(q)}`}>{tr('Искать в торрент-источниках', 'Search torrent indexers')}</Link></div>
      )}
      {people.length > 0 && (
        <Section title={t('people')}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {people.map((p) => (
              <div key={p.name} style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
                <div style={{ width: 72, height: 72, borderRadius: '50%', overflow: 'hidden', background: '#3A3B3E', flex: 'none' }}>
                  {p.photo && <img src={p.photo} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontWeight: 700 }}>{p.name}</div>
                  <div className="scroll-x" style={{ marginTop: 8 }}>
                    {p.titles.map((x) => <div key={x.kind + x.id} style={{ width: 120, flex: 'none' }}><PosterCard x={x} /></div>)}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </Section>
      )}
      {titles.length > 0 && (
        <Section title={tr('Фильмы и сериалы', 'Movies and shows')}>
          <div className="grid-posters">{titles.map((x) => <PosterCard key={x.kind + x.id} x={x} />)}</div>
        </Section>
      )}
    </main>
  );
}
