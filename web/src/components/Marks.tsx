import { useEffect, useState, type MouseEvent } from 'react';
import { api, type CatalogItem } from '../api';
import { useApp } from '../state';
import { tr } from '../i18n';
import { Icon } from './Icon';
import { markLabels, setMark, useMarks, type MarkList } from '../marks';

type Markable = Pick<CatalogItem, 'kind' | 'id' | 'title' | 'origTitle' | 'year' | 'poster'>;

const icons: Record<MarkList, string> = { liked: 'heart', later: 'clock', watched: 'check' };

function useToggle(x: Markable) {
  const m = useMarks();
  const { toast } = useApp();
  return {
    on: (l: MarkList) => m.has(l, x),
    toggle: (l: MarkList) => {
      const on = !m.has(l, x);
      setMark(l, x, on).catch((e) => toast((e as Error).message));
      if (on && l === 'watched' && m.has('later', x)) setMark('later', x, false).catch(() => {});
    },
  };
}

// MarkButtons: like / watch later / watched on a title page hero.
export function MarkButtons({ x }: { x: Markable }) {
  const { on, toggle } = useToggle(x);
  const rating = useMarks().rating(x);
  return (
    <div role="group" aria-label={tr('Мои списки', 'My lists')} style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
      {(['liked', 'later', 'watched'] as MarkList[]).map((l) => (
        <button key={l} className="btn btn-g" aria-pressed={on(l)} onClick={() => toggle(l)}
          style={on(l) ? { background: l === 'liked' ? 'var(--acc)' : '#FFFFFF', color: l === 'liked' ? '#FFFFFF' : '#111214', borderColor: 'transparent' } : undefined}>
          <Icon name={icons[l]} size={18} style={l === 'liked' && on(l) ? { fill: 'currentColor' } : undefined} />
          {tr(...markLabels[l])}
        </button>
      ))}
      {rating > 0 && <span title={tr('Ваша оценка на Letterboxd', 'Your Letterboxd rating')} style={{ fontSize: 'calc(13px * var(--fs))', fontWeight: 700, color: '#00E054' }}>{stars(rating)}</span>}
    </div>
  );
}

// stars renders a 1–10 rating as Letterboxd's five stars with halves.
export function stars(r: number) {
  return '★'.repeat(Math.floor(r / 2)) + (r % 2 ? '½' : '');
}

// CardMarks: what a poster shows of the lists, plus quick toggles on hover.
export function CardMarks({ x }: { x: Markable }) {
  const { on, toggle } = useToggle(x);
  const click = (l: MarkList) => (e: MouseEvent) => { e.preventDefault(); e.stopPropagation(); toggle(l); };
  return (
    <div className="card-marks">
      {(['liked', 'later'] as MarkList[]).map((l) => (
        <button key={l} className={'cm' + (on(l) ? ' on' : '')} aria-pressed={on(l)} aria-label={tr(...markLabels[l])} title={tr(...markLabels[l])} onClick={click(l)}>
          <Icon name={icons[l]} size={14} style={l === 'liked' && on(l) ? { fill: 'currentColor' } : undefined} />
        </button>
      ))}
      {on('watched') && <span className="cm on seen" aria-label={tr('Просмотрено', 'Watched')} title={tr('Просмотрено', 'Watched')}><Icon name="check" size={14} /></span>}
    </div>
  );
}

type Score = { value: number; votes?: number; url: string };
type Scores = { imdb: Score | null; letterboxd: Score | null };

// ExternalRatings: IMDb and Letterboxd scores, loaded after the page.
export function ExternalRatings({ kind, id }: { kind: string; id: number }) {
  const [r, setR] = useState<Scores | null>(null);
  useEffect(() => {
    let gone = false;
    api.get<Scores>(`/api/catalog/title/${kind}/${id}/ratings`).then((x) => { if (!gone) setR(x); }).catch(() => {});
    return () => { gone = true; };
  }, [kind, id]);
  if (!r) return null;
  const chip = { fontSize: 'calc(11px * var(--fs))', padding: '2px 7px', borderRadius: 4, fontWeight: 700, display: 'inline-flex', gap: 4, alignItems: 'center' } as const;
  return (
    <>
      {r.imdb && (
        <a href={r.imdb.url} target="_blank" rel="noreferrer" style={{ ...chip, background: '#F5C518', color: '#000000' }} title={r.imdb.votes ? `${r.imdb.votes.toLocaleString()} ${tr('оценок', 'votes')}` : 'IMDb'}>
          IMDb {r.imdb.value.toFixed(1)}
        </a>
      )}
      {r.letterboxd && (
        <a href={r.letterboxd.url} target="_blank" rel="noreferrer" style={{ ...chip, background: '#202830', color: '#FFFFFF' }} title={r.letterboxd.votes ? `${r.letterboxd.votes.toLocaleString()} ${tr('оценок', 'votes')}` : 'Letterboxd'}>
          <span aria-hidden="true" style={{ display: 'inline-flex', gap: 1 }}>
            <i style={{ width: 6, height: 6, borderRadius: '50%', background: '#FF8000' }} /><i style={{ width: 6, height: 6, borderRadius: '50%', background: '#00E054' }} /><i style={{ width: 6, height: 6, borderRadius: '50%', background: '#40BCF4' }} />
          </span>
          Letterboxd {r.letterboxd.value.toFixed(1)}
        </a>
      )}
    </>
  );
}
