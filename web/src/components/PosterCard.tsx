import { Link } from 'react-router-dom';
import type { Title } from '../api';
import { Art } from './Art';
import { Icon } from './Icon';
import { t } from '../i18n';

// Poster tile for library grids and search results.
export function PosterCard({ x }: { x: Title }) {
  const to = x.kind === 'show' ? `/show/${x.id}` : `/movie/${x.id}`;
  return (
    <Link to={to} className="poster">
      <Art className="pimg" src={x.poster} title={x.title} style={{ aspectRatio: '2/3', borderRadius: 12 }}
        label={
          <>
            {!x.poster && <span className="disp" style={{ position: 'absolute', left: 12, right: 12, bottom: 14, color: '#FFFFFF', fontSize: 'calc(20px * var(--fs))', fontWeight: 600, lineHeight: 1 }}>{x.title}</span>}
            {x.quality && <span className="tag" style={{ position: 'absolute', top: 8, left: 8, background: '#000000A0', color: '#FFFFFF' }}>{x.quality}</span>}
            {x.watched && <span aria-label={t('watched')} style={{ position: 'absolute', top: 8, right: 8, width: 26, height: 26, borderRadius: '50%', background: '#2F9E62', color: '#FFFFFF', display: 'grid', placeItems: 'center' }}><Icon name="check" size={14} /></span>}
            {x.progress > 0 && !x.watched && <div style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: 4, background: '#FFFFFF33' }}><div style={{ height: 4, width: `${x.progress * 100}%`, background: 'var(--acc)' }} /></div>}
          </>
        } />
      <div>
        <div style={{ fontSize: 'calc(14px * var(--fs))', fontWeight: 700, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{x.title}</div>
        <div className="sub">{[x.year || '', x.genres?.[0] || '', x.kind === 'show' && x.episodes ? `${x.episodes} ${t('episodes')}` : ''].filter(Boolean).join(' · ')}</div>
      </div>
    </Link>
  );
}
