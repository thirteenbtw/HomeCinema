import { Fragment, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { api, ApiError, fmtSize, fmtSpeed, type SearchResult, type StreamStatus, type TorrentInfo } from '../api';
import { useApp } from '../state';
import { tr } from '../i18n';
import { Icon, PlayGlyph } from '../components/Icon';
import { Modal } from '../components/CastDialog';

type Tab = 'active' | 'stream' | 'search' | 'done' | 'seeding';

const statusLabel: Record<string, [string, string]> = {
  meta: ['Метаданные', 'Metadata'], paused: ['Пауза', 'Paused'], streaming: ['Стриминг', 'Streaming'], done: ['Готово', 'Done'],
  seeding: ['Раздаётся', 'Seeding'], nospace: ['Нет места', 'No space'], scheduled: ['По расписанию', 'Scheduled'],
  queued: ['В очереди', 'Queued'], downloading: ['Загрузка', 'Downloading'], error: ['Ошибка', 'Error'],
};
function statusColors(s: string): [string, string] {
  if (s === 'streaming') return ['var(--acc)', '#FFFFFF'];
  if (s === 'done' || s === 'seeding') return ['var(--ok-bg)', 'var(--ok-fg)'];
  if (s === 'nospace' || s === 'error') return ['var(--err-bg)', 'var(--err-fg)'];
  if (s === 'downloading') return ['var(--fg)', 'var(--bg)'];
  return ['var(--chip)', 'var(--fg2)'];
}
function eta(sec: number) {
  if (!sec || sec < 0) return '—';
  if (sec < 60) return tr('< 1 мин', '< 1 min');
  const m = Math.round(sec / 60);
  return m < 60 ? `${m} ${tr('мин', 'min')}` : `${Math.floor(m / 60)} ${tr('ч', 'h')} ${m % 60} ${tr('мин', 'min')}`;
}

// Torrents (FR-T1..T12): add by magnet/.torrent, watch while downloading,
// search the user's own Torznab indexers.
export default function Torrents() {
  const { torrents: ov, openCast, toast } = useApp();
  const nav = useNavigate();
  const [params, setParams] = useSearchParams();
  const tab = (params.get('tab') as Tab) || 'active';
  // A magnet link opened from the browser arrives here via the desktop shell.
  const [magnet, setMagnet] = useState(params.get('magnet') || '');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [risk, setRisk] = useState<null | (() => void)>(null);
  const [sel, setSel] = useState<string | null>(null);
  const [drag, setDrag] = useState(false);
  const [confirmDel, setConfirmDel] = useState<TorrentInfo | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const items = ov?.items || [];

  const setTab = (t: Tab) => { const n = new URLSearchParams(params); n.set('tab', t); setParams(n, { replace: true }); };
  const streamT = items.find((x) => x.hash === sel) || items.find((x) => x.mode === 'stream' && x.status !== 'done') || null;

  const add = async (body: { magnet?: string; link?: string; file?: File }, mode: 'stream' | 'download') => {
    setErr(null); setBusy(true);
    try {
      let res: { hash: string };
      if (body.file) {
        const fd = new FormData();
        fd.append('file', body.file); fd.append('mode', mode); fd.append('keep', '1');
        res = await api.post('/api/torrents', fd);
      } else {
        res = await api.post('/api/torrents', { magnet: body.magnet, link: body.link, mode, keep: true });
      }
      setMagnet('');
      setSel(res.hash);
      toast(mode === 'stream' ? tr('Готовим поток — буферизация…', 'Buffering…') : tr('Загрузка добавлена', 'Download added'));
      if (mode === 'stream') setTab('stream');
      else if (tab === 'search') setTab('active');
    } catch (e) {
      if (e instanceof ApiError && e.status === 428) setRisk(() => () => add(body, mode));
      else setErr((e as Error).message);
    } finally { setBusy(false); }
  };

  const acceptRisk = async () => {
    await api.put('/api/torrent-settings', { acceptedRisk: true });
    const retry = risk;
    setRisk(null);
    retry?.();
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault(); setDrag(false);
    const f = e.dataTransfer.files?.[0];
    if (f) { add({ file: f }, 'download'); return; }
    const text = e.dataTransfer.getData('text');
    if (text) setMagnet(text.trim());
  };

  const filtered = items.filter((x) => {
    switch (tab) {
      case 'stream': return x.mode === 'stream';
      case 'done': return x.status === 'done';
      case 'seeding': return x.status === 'seeding';
      default: return !['done', 'seeding'].includes(x.status);
    }
  });
  const count = (t: Tab) => items.filter((x) => (t === 'stream' ? x.mode === 'stream' : t === 'done' ? x.status === 'done' : t === 'seeding' ? x.status === 'seeding' : !['done', 'seeding'].includes(x.status))).length;

  const patch = async (h: string, b: object) => { try { await api.patch(`/api/torrents/${h}`, b); } catch (e: any) { toast(e.message); } };

  return (
    <main className="main pad" onDragOver={(e) => { e.preventDefault(); setDrag(true); }} onDragLeave={(e) => { if (e.currentTarget === e.target) setDrag(false); }} onDrop={onDrop}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', gap: 16 }}>
        <div>
          <h1 className="h1">{tr('Торренты', 'Torrents')}</h1>
          <div className="sub">{tr('Встроенный движок · загрузки попадают в библиотеку автоматически', 'Built-in engine · downloads go to the library automatically')}</div>
        </div>
        <div style={{ marginLeft: 'auto', display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {ov && (ov.vpnDown
            ? <span className="pill" style={{ background: 'var(--err-bg)', color: 'var(--err-fg)', cursor: 'default' }}><Dot />{tr('VPN отключён — загрузки остановлены', 'VPN down — transfers stopped')}</span>
            : ov.iface
              ? <span className="pill" style={{ background: 'var(--ok-bg)', color: 'var(--ok-fg)', cursor: 'default' }}><Dot />VPN {ov.iface}{ov.killSwitch ? tr(' · kill switch вкл.', ' · kill switch on') : ''}</span>
              : <Link className="pill" to="/settings#torrents" style={{ cursor: 'pointer' }}><Dot />{tr('Без привязки к VPN', 'Not bound to VPN')}</Link>)}
          <span className="pill mono" style={{ cursor: 'default' }}>↓ {fmtSpeed(ov?.down || 0)}</span>
          <span className="pill mono" style={{ cursor: 'default' }}>↑ {fmtSpeed(ov?.up || 0)}</span>
          {ov && <span className="pill" style={{ cursor: 'default', ...(ov.diskLow ? { background: 'var(--err-bg)', color: 'var(--err-fg)' } : {}) }}>{tr('Свободно', 'Free')} {Math.round(ov.freeGB)} {tr('ГБ', 'GB')}</span>}
        </div>
      </div>

      {ov?.error && <div className="err">{ov.error}</div>}

      <form aria-label={tr('Добавить раздачу', 'Add torrent')} className="glass" onSubmit={(e) => { e.preventDefault(); if (magnet.trim()) add({ magnet: magnet.trim() }, 'stream'); }}
        style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12, padding: 16, borderRadius: 16, border: `1.5px dashed ${drag ? 'var(--acc)' : 'var(--line2)'}`, background: drag ? 'var(--soft)' : undefined }}>
        <button type="button" onClick={() => fileInput.current?.click()} aria-label={tr('Выбрать .torrent-файл', 'Choose a .torrent file')} style={{ width: 44, height: 44, borderRadius: 12, background: 'var(--soft)', color: 'var(--acct)', display: 'grid', placeItems: 'center', border: 0, cursor: 'pointer' }}><Icon name="magnet" /></button>
        <input ref={fileInput} type="file" accept=".torrent,application/x-bittorrent" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) add({ file: f }, 'download'); e.target.value = ''; }} />
        <label style={{ flex: '1 1 360px', display: 'flex', flexDirection: 'column', gap: 4, fontSize: 12, color: 'var(--mut2)' }}>{tr('Magnet-ссылка или .torrent — перетащите файл сюда', 'Magnet link or .torrent — drop a file here')}
          <input className="inp mono" value={magnet} onChange={(e) => setMagnet(e.target.value)} placeholder="magnet:?xt=urn:btih:…" />
        </label>
        <button className="btn btn-r" disabled={busy || !magnet.trim()}><PlayGlyph />{tr('Смотреть сразу', 'Watch now')}</button>
        <button type="button" className="btn btn-l" disabled={busy || !magnet.trim()} onClick={() => add({ magnet: magnet.trim() }, 'download')}><Icon name="download" size={18} />{tr('Скачать', 'Download')}</button>
      </form>
      {err && <div className="err">{err}</div>}

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 24 }}>
        <section aria-label={tr('Список раздач', 'Torrents')} style={{ flex: '999 1 560px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div role="tablist" style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            {([['active', tr('Активные', 'Active')], ['stream', tr('Стриминг', 'Streaming')], ['search', tr('Поиск', 'Search')], ['done', tr('Завершённые', 'Completed')], ['seeding', tr('Раздаются', 'Seeding')]] as [Tab, string][]).map(([k, l]) => (
              <button key={k} role="tab" aria-selected={tab === k} className={'pill' + (tab === k ? ' on' : '')} onClick={() => setTab(k)}>{l}{k !== 'search' && count(k) > 0 ? ` · ${count(k)}` : ''}</button>
            ))}
          </div>
          {tab === 'search' ? <SearchTab onAdd={(link, mode) => add({ link }, mode)} busy={busy} initial={params.get('q') || ''} /> : (
            <>
              {filtered.length === 0 ? (
                <div className="empty">{tab === 'active' ? tr('Нет активных загрузок. Вставьте magnet-ссылку выше или найдите раздачу во вкладке «Поиск».', 'No active downloads. Paste a magnet link above.') : tr('Здесь пока пусто', 'Nothing here yet')}</div>
              ) : (
                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', minWidth: 760, borderCollapse: 'collapse' }}>
                    <thead><tr>
                      <th className="th">{tr('Название', 'Name')}</th><th className="th">{tr('Прогресс', 'Progress')}</th><th className="th">{tr('Статус', 'Status')}</th>
                      <th className="th">{tr('Скорость', 'Speed')}</th><th className="th">{tr('Сиды / пиры', 'Seeds / peers')}</th><th className="th">{tr('Осталось', 'ETA')}</th><th className="th"><span className="sr">{tr('Действия', 'Actions')}</span></th>
                    </tr></thead>
                    <tbody>
                      {filtered.map((r) => {
                        const [bg, fg] = statusColors(r.status);
                        const isSel = streamT?.hash === r.hash;
                        const sl = statusLabel[r.status] || [r.status, r.status];
                        return (
                          <Fragment key={r.hash}>
                            <tr style={{ background: isSel ? 'var(--soft)' : undefined, cursor: 'pointer' }} onClick={() => setSel(r.hash)}>
                              <td className="td" style={{ maxWidth: 340 }}>
                                <div style={{ fontWeight: 700, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={r.name}>{r.name || r.hash}</div>
                                <div className="sub">{r.size ? fmtSize(r.size) : '—'}{r.fileCount > 1 ? ` · ${r.fileCount} ${tr('файлов', 'files')}` : ''}{r.mode === 'stream' ? ` · ${tr('последовательно', 'sequential')}` : ''}{r.error ? ` · ${r.error}` : ''}</div>
                              </td>
                              <td className="td" style={{ width: 180 }}>
                                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                  <div style={{ flex: 1, height: 4, borderRadius: 2, background: 'var(--line2)' }}><div style={{ height: 4, borderRadius: 2, width: `${r.progress * 100}%`, background: r.status === 'done' || r.status === 'seeding' ? '#2F9E62' : r.status === 'paused' ? 'var(--mut)' : 'var(--acc)' }} /></div>
                                  <span className="mono" style={{ fontSize: 11 }}>{Math.floor(r.progress * 100)}%</span>
                                </div>
                              </td>
                              <td className="td"><span className="tag" style={{ background: bg, color: fg }}>{tr(sl[0], sl[1])}</span></td>
                              <td className="td mono" style={{ fontSize: 12 }}>{r.status === 'seeding' ? '↑ ' + fmtSpeed(r.up) : fmtSpeed(r.down)}</td>
                              <td className="td mono" style={{ fontSize: 12 }}>{r.seeds} / {r.peers}</td>
                              <td className="td" style={{ fontSize: 12, color: 'var(--mut2)' }}>{r.status === 'seeding' ? `${tr('рейтинг', 'ratio')} ${r.ratio.toFixed(2)}` : r.status === 'done' ? '—' : eta(r.eta)}</td>
                              <td className="td" style={{ whiteSpace: 'nowrap' }} onClick={(e) => e.stopPropagation()}>
                                <button className="ri" style={{ width: 36, height: 36 }} aria-label={r.status === 'paused' ? tr('Продолжить', 'Resume') : tr('Пауза', 'Pause')} onClick={() => patch(r.hash, { paused: r.status !== 'paused' })}>
                                  {r.status === 'paused' ? <PlayGlyph size={14} /> : <Icon name="pause2" size={18} />}
                                </button>
                                {r.status !== 'done' && r.status !== 'seeding' && <button className="ri" style={{ width: 36, height: 36 }} aria-label={tr('Поднять приоритет', 'Raise priority')} onClick={() => patch(r.hash, { priority: r.priority + 1 })}><Icon name="up" size={18} /></button>}
                                <button className="ri" style={{ width: 36, height: 36 }} aria-label={tr('Файлы', 'Files')} aria-expanded={open === r.hash} onClick={() => setOpen(open === r.hash ? null : r.hash)}><Icon name="folder" size={18} /></button>
                                <button className="ri" style={{ width: 36, height: 36 }} aria-label={tr('Удалить', 'Remove')} onClick={() => setConfirmDel(r)}><Icon name="trash" size={18} /></button>
                              </td>
                            </tr>
                            {open === r.hash && <tr><td className="td" colSpan={7}><Files hash={r.hash} onPlay={(i) => nav(`/play/torrent/${r.hash}?file=${i}`)} /></td></tr>}
                          </Fragment>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
              <p style={{ margin: 0, fontSize: 12, color: 'var(--mut)' }}>{tr('После загрузки файлы автоматически переносятся в библиотеку: Фильмы / Сериалы / Сезон N. Исполняемые файлы из раздач не запускаются.', 'Finished files move to the library automatically. Executables from torrents are never opened.')}</p>
            </>
          )}
        </section>

        {streamT && <StreamPanel t={streamT} onCast={(file) => openCast({ torrent: streamT.hash, file, title: streamT.name })} onKeep={(keep) => patch(streamT.hash, { keep })} />}
      </div>

      <p className="glass" style={{ margin: 0, padding: '12px 16px', borderRadius: 12, border: '1px solid var(--line)', fontSize: 12, color: 'var(--mut2)' }}>
        {tr('Ответственность за законность загружаемого и раздаваемого контента лежит на пользователе. Источники поиска (Torznab / Jackett / Prowlarr) добавляете вы сами — в комплекте их нет.', 'You are responsible for the legality of content you download and share. Search sources (Torznab / Jackett / Prowlarr) are added by you; none are bundled.')}
      </p>

      {risk && (
        <Modal title={tr('Перед первой загрузкой', 'Before your first download')} onClose={() => setRisk(null)}>
          <p style={{ margin: 0, fontSize: 14, lineHeight: 1.6 }}>{tr('Торрент-клиент скачивает и одновременно раздаёт данные другим участникам. Ответственность за законность загружаемого и раздаваемого контента лежит на вас. Рекомендуем привязать клиент к VPN в настройках — тогда при обрыве VPN загрузки остановятся.', 'The torrent client downloads and shares data with others at the same time. You are responsible for the legality of the content. We recommend binding the client to a VPN in Settings.')}</p>
          <div className="row">
            <button className="btn btn-r sm" onClick={acceptRisk}>{tr('Понимаю, продолжить', 'I understand, continue')}</button>
            <Link className="btn btn-l sm" to="/settings#torrents">{tr('Настроить VPN', 'Set up VPN')}</Link>
          </div>
        </Modal>
      )}
      {confirmDel && <DeleteDialog t={confirmDel} onClose={() => setConfirmDel(null)} onDone={() => { if (sel === confirmDel.hash) setSel(null); setConfirmDel(null); }} />}
    </main>
  );
}

function Dot() { return <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'currentColor' }} />; }

function DeleteDialog({ t, onClose, onDone }: { t: TorrentInfo; onClose: () => void; onDone: () => void }) {
  const [data, setData] = useState(t.status !== 'done' && t.status !== 'seeding');
  const [err, setErr] = useState<string | null>(null);
  const go = async () => {
    try { await api.del(`/api/torrents/${t.hash}${data ? '?data=1' : ''}`); onDone(); } catch (e: any) { setErr(e.message); }
  };
  return (
    <Modal title={tr('Удалить раздачу?', 'Remove torrent?')} onClose={onClose}>
      <div style={{ fontWeight: 600, wordBreak: 'break-word' }}>{t.name}</div>
      <label className="chk"><input type="checkbox" checked={data} onChange={(e) => setData(e.target.checked)} />{tr('Удалить и недокачанные файлы с диска', 'Also delete downloaded data')}</label>
      {t.imported && t.imported.length > 0 && <div className="sub">{tr('Файлы, уже перенесённые в библиотеку, останутся.', 'Files already moved to the library stay.')}</div>}
      {err && <div className="err">{err}</div>}
      <div className="row"><button className="btn btn-r sm" onClick={go}>{tr('Удалить', 'Remove')}</button><button className="btn btn-l sm" onClick={onClose}>{tr('Отмена', 'Cancel')}</button></div>
    </Modal>
  );
}

function Files({ hash, onPlay }: { hash: string; onPlay: (i: number) => void }) {
  const [info, setInfo] = useState<TorrentInfo | null>(null);
  const load = () => api.get<TorrentInfo>(`/api/torrents/${hash}`).then(setInfo).catch(() => {});
  useEffect(() => { load(); const id = window.setInterval(load, 3000); return () => clearInterval(id); }, [hash]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!info) return <span className="spin" />;
  if (!info.files?.length) return <div className="sub">{tr('Ждём метаданные раздачи…', 'Waiting for metadata…')}</div>;
  const toggle = async (i: number, on: boolean) => {
    const off = info.files!.filter((f) => (f.index === i ? !on : !f.wanted)).map((f) => f.index);
    await api.patch(`/api/torrents/${hash}`, { filesOff: off });
    load();
  };
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      {info.files.map((f) => (
        <div key={f.index} style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 12 }}>
          <input type="checkbox" checked={f.wanted} disabled={f.blocked} onChange={(e) => toggle(f.index, e.target.checked)} aria-label={f.path} style={{ width: 16, height: 16, accentColor: 'var(--acc)' }} />
          <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={f.path}>{f.path}{f.blocked && <span className="tag" style={{ marginLeft: 6, background: 'var(--err-bg)', color: 'var(--err-fg)' }}>{tr('исполняемый — заблокирован', 'executable — blocked')}</span>}</span>
          <span className="mono" style={{ color: 'var(--mut)' }}>{Math.floor(f.progress * 100)}% · {fmtSize(f.size)}</span>
          {f.video && !f.blocked && <button className="btn btn-l sm" style={{ height: 30, padding: '0 10px' }} onClick={() => onPlay(f.index)}><PlayGlyph size={12} />{tr('Смотреть', 'Play')}</button>}
        </div>
      ))}
    </div>
  );
}

function StreamPanel({ t, onCast, onKeep }: { t: TorrentInfo; onCast: (file: number) => void; onKeep: (keep: boolean) => void }) {
  const [st, setSt] = useState<StreamStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    let stop = false;
    setSt(null); setErr(null);
    const tick = () => api.get<StreamStatus>(`/api/torrents/${t.hash}/stream?cells=200`).then((s) => { if (!stop) { setSt(s); setErr(null); } }).catch((e) => !stop && setErr(e.message));
    tick();
    const id = window.setInterval(tick, 1500);
    return () => { stop = true; clearInterval(id); };
  }, [t.hash]);
  const ready = st?.ready;
  const bufPct = st && st.target ? Math.min(1, st.buffered / st.target) : 0;
  const colors = ['#3A3C41', '#FFFFFF', 'var(--acc)', '#8E9096'];
  return (
    <aside aria-label={tr('Стриминг', 'Streaming')} style={{ flex: '1 1 380px', minWidth: 0, borderRadius: 20, background: 'var(--panel)', border: '1px solid var(--line)', color: '#FFFFFF', padding: 20, display: 'flex', flexDirection: 'column', gap: 16, alignSelf: 'flex-start' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{ fontSize: 11, fontWeight: 700, padding: '3px 8px', borderRadius: 999, background: t.mode === 'stream' ? 'var(--acc)' : '#FFFFFF2E' }}>{t.mode === 'stream' ? tr('Смотреть сразу', 'Watch now') : tr('Загрузка', 'Download')}</span>
        <span style={{ fontSize: 12, color: '#A9ADB4' }}>{t.mode === 'stream' ? tr('последовательная загрузка', 'sequential download') : tr('обычная загрузка', 'regular download')}</span>
      </div>
      <div className="disp" style={{ fontSize: 26, fontWeight: 600, lineHeight: 1.05, wordBreak: 'break-word' }}>{st?.fileName?.split(/[\\/]/).pop()?.replace(/\.[^.]+$/, '') || t.name}</div>
      <div style={{ fontSize: 12, color: '#A9ADB4' }}>{[st ? fmtSize(st.size) : t.size ? fmtSize(t.size) : '', `${Math.floor(t.progress * 100)}% ${tr('загружено', 'downloaded')}`].filter(Boolean).join(' · ')}</div>
      {err && !st && <div style={{ fontSize: 12, color: '#FFB4B8' }}>{t.status === 'meta' ? tr('Получаем метаданные от пиров…', 'Fetching metadata from peers…') : err}</div>}
      {st && (
        <>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
              <span style={{ fontWeight: 700 }}>{ready ? tr('Буфер готов — можно смотреть', 'Buffer ready — you can watch') : tr('Буферизация…', 'Buffering…')}</span>
              <span className="mono" style={{ color: '#A9ADB4' }}>{Math.round(st.buffered / 1048576)} / {Math.round(st.target / 1048576)} {tr('МБ', 'MB')}</span>
            </div>
            <div style={{ height: 6, borderRadius: 3, background: '#FFFFFF26' }}><div style={{ height: 6, borderRadius: 3, width: `${bufPct * 100}%`, background: ready ? '#3CCB7F' : 'var(--acc)', transition: 'width .5s' }} /></div>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
              <span style={{ fontWeight: 700 }}>{tr('Карта кусков', 'Piece map')}</span>
              <span style={{ color: '#A9ADB4' }}>{st.numPieces.toLocaleString('ru-RU')} {tr('кусков', 'pieces')} · {fmtSize(st.pieceSize)}</span>
            </div>
            <div role="img" aria-label={tr(`Карта кусков: загружено ${Math.floor(st.progress * 100)}%`, `Piece map: ${Math.floor(st.progress * 100)}% downloaded`)} style={{ display: 'grid', gridTemplateColumns: 'repeat(40,minmax(0,1fr))', gap: 2 }}>
              {st.pieces.map((p, i) => <div key={i} style={{ aspectRatio: '1', borderRadius: 1, background: colors[p] || colors[0] }} />)}
            </div>
            <div style={{ display: 'flex', gap: 14, flexWrap: 'wrap', fontSize: 11, color: '#C9CCD1' }}>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 8, height: 8, background: '#FFFFFF' }} />{tr('Загружено', 'Downloaded')}</span>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 8, height: 8, background: 'var(--acc)' }} />{tr('Приоритет (позиция плеера)', 'Priority (player position)')}</span>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 8, height: 8, background: '#3A3C41' }} />{tr('Нет', 'Missing')}</span>
            </div>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,minmax(0,1fr))', gap: 8 }}>
            <Stat label={tr('Скорость', 'Speed')} value={fmtSpeed(st.down)} />
            <Stat label={tr('Пиры', 'Peers')} value={String(st.peers)} />
            <Stat label={tr('Сиды', 'Seeds')} value={String(st.seeds)} />
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            <Link className="btn btn-w" to={`/play/torrent/${t.hash}?file=${st.fileIndex}`} aria-disabled={!ready}><PlayGlyph />{tr('Смотреть на ПК', 'Watch on PC')}</Link>
            <button className="btn btn-g" onClick={() => onCast(st.fileIndex)}><Icon name="cast" size={18} />{tr('На ТВ', 'To TV')}</button>
          </div>
        </>
      )}
      <label style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 13, color: '#D4D7DC', minHeight: 44 }}>
        <input type="checkbox" checked={t.keep} onChange={(e) => onKeep(e.target.checked)} style={{ width: 18, height: 18, accentColor: 'var(--acc)' }} />
        {tr('Сохранить в библиотеку после просмотра', 'Keep in library after watching')}
      </label>
    </aside>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return <div style={{ padding: 10, borderRadius: 10, background: '#FFFFFF14' }}><div style={{ fontSize: 11, color: '#A9ADB4' }}>{label}</div><div className="mono" style={{ fontSize: 15 }}>{value}</div></div>;
}

function SearchTab({ onAdd, busy, initial }: { onAdd: (link: string, mode: 'stream' | 'download') => void; busy: boolean; initial: string }) {
  const [q, setQ] = useState(initial);
  const [res, setRes] = useState<SearchResult[] | null>(null);
  const [errs, setErrs] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const run = async (text = q) => {
    if (!text.trim()) return;
    setLoading(true); setErr(null);
    try {
      const r = await api.get<{ results: SearchResult[] | null; errors: string[] | null }>(`/api/torrents/search?q=${encodeURIComponent(text.trim())}`);
      setRes(r.results || []); setErrs(r.errors || []);
    } catch (e: any) { setErr(e.message); setRes(null); } finally { setLoading(false); }
  };
  useEffect(() => { if (initial) run(initial); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <form className="row" onSubmit={(e) => { e.preventDefault(); run(); }}>
        <input className="inp" style={{ flex: '1 1 260px' }} value={q} onChange={(e) => setQ(e.target.value)} placeholder={tr('Название фильма или сериала', 'Movie or show title')} aria-label={tr('Поиск раздач', 'Search torrents')} />
        <button className="btn btn-d sm" disabled={loading}>{loading ? <span className="spin" /> : <Icon name="search" size={16} />}{tr('Искать', 'Search')}</button>
      </form>
      {err && <div className="err">{err} {err.includes('Torznab') && <Link to="/settings#torrents">{tr('Открыть настройки', 'Open settings')}</Link>}</div>}
      {errs.map((e) => <div key={e} className="sub">⚠ {e}</div>)}
      {res && res.length === 0 && <div className="empty">{tr('Ничего не найдено в ваших источниках', 'Nothing found in your indexers')}</div>}
      {res && res.length > 0 && (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', minWidth: 720, borderCollapse: 'collapse' }}>
            <thead><tr><th className="th">{tr('Раздача', 'Release')}</th><th className="th">{tr('Качество', 'Quality')}</th><th className="th">{tr('Размер', 'Size')}</th><th className="th">{tr('Сиды', 'Seeds')}</th><th className="th">{tr('Источник', 'Indexer')}</th><th className="th" /></tr></thead>
            <tbody>
              {res.slice(0, 50).map((r) => (
                <tr key={r.link + r.title} style={{ background: r.best ? 'var(--soft)' : undefined }}>
                  <td className="td" style={{ maxWidth: 360 }}>
                    <div style={{ fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={r.title}>{r.best && <span className="tag" style={{ background: 'var(--acc)', color: '#FFFFFF', marginRight: 6 }}>{tr('Лучшее', 'Best')}</span>}{r.title}</div>
                    <div className="sub">{[r.lang, r.date?.slice(0, 10)].filter(Boolean).join(' · ')}</div>
                  </td>
                  <td className="td">{r.quality || '—'}</td>
                  <td className="td mono" style={{ fontSize: 12 }}>{fmtSize(r.size)}</td>
                  <td className="td mono" style={{ fontSize: 12 }}>{r.seeds}</td>
                  <td className="td" style={{ fontSize: 12, color: 'var(--mut2)' }}>{r.indexer}</td>
                  <td className="td" style={{ whiteSpace: 'nowrap' }}>
                    <button className="btn btn-r sm" style={{ height: 32, padding: '0 12px' }} disabled={busy} onClick={() => onAdd(r.link, 'stream')}><PlayGlyph size={12} />{tr('Смотреть', 'Watch')}</button>{' '}
                    <button className="btn btn-l sm" style={{ height: 32, padding: '0 12px' }} disabled={busy} onClick={() => onAdd(r.link, 'download')} aria-label={tr('Скачать', 'Download')}><Icon name="download" size={16} /></button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
