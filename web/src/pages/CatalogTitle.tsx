import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api, ApiError, fmtRuntime, fmtSize, type CatalogTitle, type SearchResult } from '../api';
import { useApp, useAsync } from '../state';
import { t, tr } from '../i18n';
import { Art, ambientFor, paletteFor } from '../components/Art';
import { Icon, PlayGlyph } from '../components/Icon';
import { Modal } from '../components/CastDialog';
import { Section, useAmbient } from '../components/Shell';
import { CatalogCard } from './Catalog';

type TitleResp = { provider: string; link: string; title: CatalogTitle; library: { kind: 'movie' | 'show'; id: number } | null; indexers: boolean };
type TorrentsResp = { results: SearchResult[]; errors: string[]; loose: boolean };
type Mode = 'stream' | 'download';

const qualities = ['', '2160p', '1080p', '720p'];

// A catalog title: provider details plus releases from the user's own indexers,
// each one either streamed right away (sequential download) or downloaded.
export default function CatalogTitlePage() {
  const { kind = 'movie', id } = useParams();
  const { data, error } = useAsync(() => api.get<TitleResp>(`/api/catalog/title/${kind}/${id}`), [kind, id]);
  if (error) {
    return (
      <main className="main pad">
        <div className="empty">{error}{/TMDB|Кинопоиск|ключ/.test(error) && <Link className="btn btn-l sm" to="/settings">{tr('Открыть настройки', 'Open settings')}</Link>}</div>
      </main>
    );
  }
  if (!data) return <main className="main pad"><span className="spin" /></main>;
  return <Detail key={kind + id} d={data} />;
}

function Detail({ d }: { d: TitleResp }) {
  const x = d.title;
  const { toast } = useApp();
  const nav = useNavigate();
  const setAmbient = useAmbient();
  const show = x.kind === 'tv';
  const seasons = x.seasons || [];
  const [season, setSeason] = useState(0);
  const [quality, setQuality] = useState('');
  const [res, setRes] = useState<TorrentsResp | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [risk, setRisk] = useState<null | (() => void)>(null);
  const [sky] = paletteFor(x.title);

  useEffect(() => { setAmbient(ambientFor(x.title)); return () => setAmbient(null); }, [x.title]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!d.indexers) return;
    let gone = false;
    setLoading(true); setErr(null);
    api.get<TorrentsResp>(`/api/catalog/title/${x.kind}/${x.id}/torrents${season ? '?season=' + season : ''}`)
      .then((r) => { if (!gone) setRes(r); })
      .catch((e) => { if (!gone) { setErr(e.message); setRes(null); } })
      .finally(() => { if (!gone) setLoading(false); });
    return () => { gone = true; };
  }, [x.kind, x.id, season, d.indexers]);

  const shown = useMemo(() => (res?.results || []).filter((r) => !quality || r.quality === quality), [res, quality]);
  const best = shown.find((r) => r.best) || (shown[0] && shown[0].seeds > 0 ? shown[0] : undefined);

  const add = async (r: SearchResult, mode: Mode) => {
    setBusy(r.link + mode);
    try {
      const { hash } = await api.post<{ hash: string }>('/api/torrents', { link: r.link, mode, keep: true });
      if (mode === 'download') toast(tr('Загрузка добавлена — после неё фильм появится в библиотеке', 'Download added — it lands in the library when done'));
      else if (show) nav(`/torrents?tab=stream&sel=${hash}`); // a season pack: pick the episode there
      else nav(`/play/torrent/${hash}`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 428) setRisk(() => () => add(r, mode));
      else toast((e as Error).message);
    } finally { setBusy(null); }
  };
  const acceptRisk = async () => {
    await api.put('/api/torrent-settings', { acceptedRisk: true });
    const retry = risk;
    setRisk(null);
    retry?.();
  };

  const libLink = d.library ? `/${d.library.kind}/${d.library.id}` : '';
  const meta = [x.year || '', x.runtime ? fmtRuntime(x.runtime) + (show ? tr(' / серия', ' / ep') : '') : '', ...x.genres.slice(0, 3)].filter(Boolean).join(' · ');

  return (
    <main className="main" style={{ gap: 28, paddingTop: 16 }}>
      <section style={{ position: 'relative', overflow: 'hidden', borderRadius: 20, background: sky, minHeight: 420, display: 'flex', flexDirection: 'column', padding: '20px 40px 36px', color: '#FFFFFF', boxShadow: '0 20px 50px var(--sh)' }}>
        {x.backdrop && <img aria-hidden="true" src={x.backdrop} alt="" style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover' }} />}
        <div aria-hidden="true" style={{ position: 'absolute', inset: 0, background: 'linear-gradient(90deg,#000000D0 0%,#00000080 55%,#00000020 85%)' }} />
        <nav aria-label={tr('Путь', 'Breadcrumb')} style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, fontWeight: 600 }}>
          <button onClick={() => nav(-1)} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, height: 28, padding: '0 10px', borderRadius: 999, background: '#FFFFFF2E', color: '#FFFFFF', border: 0, cursor: 'pointer', font: 'inherit' }}><Icon name="left" size={14} />{t('back')}</button>
          <Link to={`/catalog${show ? '?kind=tv' : ''}`} style={{ color: '#FFFFFF' }}>{t('catalog')}</Link>
          <span style={{ color: '#C9CCD1' }}>›</span><span>{x.title}</span>
        </nav>
        <div style={{ position: 'relative', display: 'flex', gap: 28, alignItems: 'flex-end', marginTop: 'auto', paddingTop: 28, flexWrap: 'wrap' }}>
          <Art src={x.poster} title={x.title} style={{ width: 180, aspectRatio: '2/3', borderRadius: 14, flex: 'none', boxShadow: '0 16px 40px #00000080' }} />
          <div style={{ flex: '1 1 360px', display: 'flex', flexDirection: 'column', gap: 12, maxWidth: 680 }}>
            <div className="disp" style={{ fontSize: 'clamp(38px,6vw,68px)', lineHeight: 0.95, fontWeight: 600, letterSpacing: '.04em' }}>{x.title}</div>
            {x.origTitle && x.origTitle !== x.title && <div style={{ fontSize: 14, color: '#D4D7DC' }}>{x.origTitle}</div>}
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', fontSize: 13, fontWeight: 600 }}>
              {meta}
              {x.cert && <span style={{ fontSize: 11, padding: '2px 7px', border: '1px solid #FFFFFF66', borderRadius: 4 }}>{x.cert}</span>}
              {x.rating > 0 && <span style={{ fontSize: 11, padding: '2px 7px', borderRadius: 4, ...(d.provider === 'TMDB' ? { background: '#01B4E4', color: '#06222C' } : { background: '#FF6600', color: '#FFFFFF' }) }}>{d.provider === 'TMDB' ? 'TMDB' : 'КП'} {x.rating.toFixed(1)}</span>}
              {show && seasons.length > 0 && <span style={{ fontSize: 11, padding: '2px 7px', borderRadius: 4, background: '#FFFFFF2E' }}>{seasons.length} {tr('сез.', 'seasons')}</span>}
            </div>
            {x.tagline && <div style={{ fontSize: 14, fontStyle: 'italic', color: '#D4D7DC' }}>{x.tagline}</div>}
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', paddingTop: 4 }}>
              {libLink && <Link className="btn btn-w" to={libLink}><PlayGlyph />{tr('Есть в библиотеке', 'In your library')}</Link>}
              {best && <button className={libLink ? 'btn btn-g' : 'btn btn-w'} disabled={!!busy} onClick={() => add(best, 'stream')}>{busy === best.link + 'stream' ? <span className="spin" /> : <PlayGlyph />}{tr('Смотреть сразу', 'Watch now')}{best.quality ? ` · ${best.quality}` : ''}</button>}
              {best && <button className="btn btn-g" disabled={!!busy} onClick={() => add(best, 'download')}><Icon name="download" size={18} />{tr('Скачать', 'Download')}</button>}
              {!best && !libLink && d.indexers && loading && <span className="btn btn-g" aria-live="polite"><span className="spin" />{tr('Ищем раздачи…', 'Looking for releases…')}</span>}
            </div>
          </div>
        </div>
      </section>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 32 }}>
        <div style={{ flex: '999 1 480px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 28 }}>
          <p style={{ margin: 0, fontSize: 14, lineHeight: 1.65, color: 'var(--fg2)', maxWidth: 760 }}>{x.overview || tr('Описания нет.', 'No description.')}</p>

          <Section title={tr('Раздачи', 'Releases')} note={tr('Из ваших источников Torznab · «Смотреть» начинает показ, пока файл качается', 'From your Torznab sources · “Watch” starts playback while the file downloads')}>
            {!d.indexers ? (
              <div className="empty">
                {tr('Чтобы смотреть и скачивать из каталога, добавьте свой источник Torznab — например Jackett.', 'To watch or download from the catalog, add your own Torznab source such as Jackett.')}
                <Link className="btn btn-l sm" to="/settings#torrents">{tr('Добавить источник', 'Add a source')}</Link>
              </div>
            ) : (
              <>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {show && seasons.length > 1 && (
                    <div role="group" aria-label={t('season')} className="scroll-x" style={{ gap: 4, maxWidth: '100%' }}>
                      <button className={'pill' + (!season ? ' on' : ' ghost')} style={{ flex: 'none' }} onClick={() => setSeason(0)}>{tr('Все сезоны', 'All seasons')}</button>
                      {seasons.map((s) => <button key={s.number} className={'pill' + (season === s.number ? ' on' : ' ghost')} style={{ flex: 'none' }} onClick={() => setSeason(s.number)}>{t('season')} {s.number}</button>)}
                    </div>
                  )}
                  <div role="group" aria-label={tr('Качество', 'Quality')} style={{ display: 'flex', gap: 4 }}>
                    {qualities.map((q) => <button key={q} className={'pill' + (quality === q ? ' on' : ' ghost')} aria-pressed={quality === q} onClick={() => setQuality(q)}>{q || tr('Любое', 'Any')}</button>)}
                  </div>
                </div>
                {loading && <div style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 13, color: 'var(--mut2)' }}><span className="spin" />{tr('Опрашиваем источники…', 'Querying sources…')}</div>}
                {err && <div className="err">{err}</div>}
                {res?.errors.map((e) => <div key={e} className="sub">⚠ {e}</div>)}
                {res?.loose && shown.length > 0 && <div className="sub">{tr('Точных совпадений нет — показаны все найденные раздачи, проверьте название.', 'No exact matches — showing everything found, check the names.')}</div>}
                {res && !loading && shown.length === 0 && <div className="empty">{tr('В ваших источниках раздач не нашлось', 'No releases on your sources')}</div>}
                {shown.length > 0 && !loading && (
                  <div style={{ overflowX: 'auto' }}>
                    <table style={{ width: '100%', minWidth: 680, borderCollapse: 'collapse' }}>
                      <thead><tr><th className="th">{tr('Раздача', 'Release')}</th><th className="th">{tr('Качество', 'Quality')}</th><th className="th">{tr('Размер', 'Size')}</th><th className="th">{tr('Сиды', 'Seeds')}</th><th className="th"><span className="sr">{tr('Действия', 'Actions')}</span></th></tr></thead>
                      <tbody>
                        {shown.slice(0, 60).map((r) => (
                          <tr key={r.link + r.title} style={{ background: r === best ? 'var(--soft)' : undefined }}>
                            <td className="td" style={{ maxWidth: 420 }}>
                              <div style={{ fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={r.title}>{r === best && <span className="tag" style={{ background: 'var(--acc)', color: '#FFFFFF', marginRight: 6 }}>{tr('Лучшее', 'Best')}</span>}{r.title}</div>
                              <div className="sub">{[r.indexer, r.lang === 'rus' ? tr('русская озвучка', 'Russian audio') : '', r.date?.slice(0, 16)].filter(Boolean).join(' · ')}</div>
                            </td>
                            <td className="td">{r.quality || '—'}</td>
                            <td className="td mono" style={{ fontSize: 12 }}>{fmtSize(r.size)}</td>
                            <td className="td mono" style={{ fontSize: 12, color: r.seeds < 3 ? 'var(--err-fg)' : undefined }}>{r.seeds}</td>
                            <td className="td" style={{ whiteSpace: 'nowrap' }}>
                              <button className="btn btn-r sm" style={{ height: 32, padding: '0 12px' }} disabled={!!busy} onClick={() => add(r, 'stream')}>{busy === r.link + 'stream' ? <span className="spin" /> : <PlayGlyph size={12} />}{tr('Смотреть', 'Watch')}</button>{' '}
                              <button className="btn btn-l sm" style={{ height: 32, padding: '0 12px' }} disabled={!!busy} onClick={() => add(r, 'download')} aria-label={tr('Скачать', 'Download')} title={tr('Скачать в библиотеку', 'Download to the library')}><Icon name="download" size={16} /></button>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            )}
          </Section>

          {x.cast.length > 0 && (
            <Section title={t('cast')}>
              <div className="scroll-x">
                {x.cast.map((a) => (
                  <div key={a.name + a.role} style={{ width: 104, flex: 'none', display: 'flex', flexDirection: 'column', gap: 6 }}>
                    <div style={{ width: 104, aspectRatio: '3/4', borderRadius: 10, overflow: 'hidden', background: '#3A3B3E' }}>
                      {a.photo && <img src={a.photo} alt="" loading="lazy" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />}
                    </div>
                    <div style={{ fontSize: 12, fontWeight: 700, lineHeight: 1.2 }}>{a.name}</div>
                    <div className="sub" style={{ fontSize: 11, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{a.role}</div>
                  </div>
                ))}
              </div>
            </Section>
          )}

          {x.related.length > 0 && (
            <Section title={tr('Похожие', 'More like this')} note={tr(`Рекомендации: ${d.provider}`, `Recommended by ${d.provider}`)}>
              <div className="scroll-x">
                {x.related.map((r) => <div key={r.kind + r.id} style={{ width: 140, flex: 'none' }}><CatalogCard x={r} /></div>)}
              </div>
            </Section>
          )}
        </div>

        <aside style={{ flex: '1 1 240px', display: 'flex', flexDirection: 'column', gap: 10, fontSize: 13, alignSelf: 'flex-start' }}>
          {([
            [tr('Оригинальное название', 'Original title'), x.origTitle],
            [tr('Год', 'Year'), x.year ? String(x.year) : ''],
            [tr('Страна', 'Country'), (x.countries || []).join(', ')],
            [tr('Жанры', 'Genres'), x.genres.join(', ')],
            [tr('Сезоны', 'Seasons'), show ? seasons.map((s) => `${s.number} (${s.episodes})`).join(', ') : ''],
          ] as [string, string][]).filter(([, v]) => v).map(([k, v]) => (
            <div key={k}><div className="sub">{k}</div><div style={{ fontWeight: 600 }}>{v}</div></div>
          ))}
          {x.imdbId && <a href={`https://www.imdb.com/title/${x.imdbId}/`} target="_blank" rel="noreferrer" style={{ fontWeight: 600 }}>IMDb ↗</a>}
          <a href={d.link} target="_blank" rel="noreferrer" style={{ fontWeight: 600 }}>{d.provider} ↗</a>
        </aside>
      </div>

      {risk && (
        <Modal title={tr('Перед первой загрузкой', 'Before your first download')} onClose={() => setRisk(null)}>
          <p style={{ margin: 0, fontSize: 14, lineHeight: 1.6 }}>{tr('Торрент-клиент скачивает и одновременно раздаёт данные другим участникам. Ответственность за законность загружаемого и раздаваемого контента лежит на вас. Рекомендуем привязать клиент к VPN в настройках — тогда при обрыве VPN загрузки остановятся.', 'The torrent client downloads and shares data with others at the same time. You are responsible for the legality of the content. We recommend binding the client to a VPN in Settings.')}</p>
          <div className="row">
            <button className="btn btn-r sm" onClick={acceptRisk}>{tr('Понимаю, продолжить', 'I understand, continue')}</button>
            <Link className="btn btn-l sm" to="/settings#torrents">{tr('Настроить VPN', 'Set up VPN')}</Link>
          </div>
        </Modal>
      )}
    </main>
  );
}
