import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, fmtTime, type CastStatus, type Device, type Item } from '../api';
import { useApp } from '../state';
import { tr } from '../i18n';
import { Art } from '../components/Art';
import { Icon, PauseGlyph, PlayGlyph, StopGlyph } from '../components/Icon';

function codecLine(d: Device) {
  const v = (d.caps?.video || []).map((c) => ({ h264: 'H.264', hevc: 'HEVC', vp9: 'VP9', av1: 'AV1' } as Record<string, string>)[c] || c).join(', ');
  const parts = [v];
  if (d.caps?.hdr) parts.push('HDR10');
  if (d.caps && !(d.caps.audio || []).includes('dts')) parts.push(tr('без DTS', 'no DTS'));
  if (d.caps?.maxHeight) parts.push(d.caps.maxHeight >= 2160 ? '4K' : `${d.caps.maxHeight}p`);
  return parts.filter(Boolean).join(' · ');
}

// Devices (FR-11..FR-14): discovered TVs, manual add, and the remote.
export default function Devices() {
  const { devices, cast, toast } = useApp();
  const [scanning, setScanning] = useState(false);
  const [ip, setIp] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [sel, setSel] = useState<string | null>(null);
  const [polled, setPolled] = useState<CastStatus[] | null>(null);

  useEffect(() => { api.get<CastStatus[]>('/api/cast').then((c) => setPolled(c || [])).catch(() => {}); }, []);
  const sessions = cast.length ? cast : polled || [];
  const active = sessions.find((s) => s.deviceId === sel) || sessions[0];

  const rescan = async () => {
    setScanning(true);
    try { await api.post('/api/devices/scan'); } catch (e: any) { toast(e.message); } finally { setScanning(false); }
  };
  const addIp = async () => {
    setErr(null);
    try { await api.post('/api/devices', { ip: ip.trim() }); setIp(''); toast(tr('Устройство добавлено', 'Device added')); } catch (e: any) { setErr(e.message); }
  };
  const remove = async (d: Device) => { try { await api.del(`/api/devices/${encodeURIComponent(d.id)}`); } catch (e: any) { toast(e.message); } };

  return (
    <main className="main pad">
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', gap: 16 }}>
        <div>
          <h1 className="h1">{tr('Устройства', 'Devices')}</h1>
          <div className="sub">{tr('Найдено в локальной сети', 'Found on the local network')}: {devices.filter((d) => d.online).length} · Google Cast</div>
        </div>
        <button className="btn btn-l" style={{ marginLeft: 'auto' }} onClick={rescan} disabled={scanning}>{scanning ? <span className="spin" /> : <Icon name="refresh" size={18} />}{tr('Искать снова', 'Rescan')}</button>
      </div>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 24 }}>
        <section aria-label={tr('Список устройств', 'Device list')} style={{ flex: '999 1 480px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 10 }}>
          {devices.length === 0 && (
            <div className="empty">{tr('ТВ пока не найдены. Проверьте, что ТВ включён и подключён к той же сети, что и этот ПК. Если сеть гостевая или с изоляцией клиентов, добавьте ТВ по IP.', 'No TVs yet. Make sure the TV is on and on the same network, or add it by IP.')}</div>
          )}
          {devices.map((d) => {
            const s = sessions.find((x) => x.deviceId === d.id && x.state !== 'idle');
            const [label, bg, fg] = s ? [tr('Транслирует', 'Casting'), 'var(--soft)', 'var(--acct)'] : d.online ? (d.busy ? [tr('Занят', 'Busy'), 'var(--warn-bg)', 'var(--warn-fg)'] : [tr('Свободен', 'Idle'), 'var(--ok-bg)', 'var(--ok-fg)']) : [tr('Не в сети', 'Offline'), 'var(--chip)', 'var(--mut2)'];
            return (
              <div key={d.id} className="glass" onClick={() => s && setSel(d.id)}
                style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 16, padding: 16, borderRadius: 16, border: s ? '2px solid var(--acc)' : '1px solid var(--line)', opacity: d.online ? 1 : 0.6, cursor: s ? 'pointer' : 'default' }}>
                <div style={{ width: 56, height: 40, borderRadius: 6, background: '#111214', display: 'grid', placeItems: 'center', color: '#FFFFFF' }}><Icon name="tv" /></div>
                <div style={{ flex: '1 1 200px', display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><span style={{ fontWeight: 700, fontSize: 'calc(15px * var(--fs))' }}>{d.name}</span><span className="tag" style={{ background: bg, color: fg, height: 20 }}>{label}</span></div>
                  <span className="sub">{d.model || 'Google Cast'}{d.manual ? ` · ${tr('добавлен вручную', 'added manually')}` : ''}</span>
                  <span style={{ fontSize: 'calc(12px * var(--fs))', color: 'var(--mut2)' }}>{codecLine(d)}</span>
                </div>
                <span className="pill" style={{ cursor: 'default' }}>{d.proto === 'cast' || !d.proto ? 'Google Cast' : d.proto}</span>
                <span className="mono" style={{ fontSize: 'calc(12px * var(--fs))', color: 'var(--mut)' }}>{d.addr}</span>
                {d.manual && <button className="ri" aria-label={`${tr('Удалить', 'Remove')} ${d.name}`} onClick={(e) => { e.stopPropagation(); remove(d); }}><Icon name="trash" size={18} /></button>}
              </div>
            );
          })}
          <form onSubmit={(e) => { e.preventDefault(); if (ip.trim()) addIp(); }} style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', gap: 10, padding: 16, borderRadius: 16, border: '1.5px dashed var(--line2)' }}>
            <label className="fld" style={{ flex: '1 1 200px' }}>{tr('Не нашли ТВ? Добавьте по IP', 'TV missing? Add by IP')}
              <input className="inp mono" value={ip} onChange={(e) => setIp(e.target.value)} placeholder="192.168.1.___" inputMode="decimal" />
            </label>
            <label className="fld" style={{ flex: '0 1 160px' }}>{tr('Протокол', 'Protocol')}
              <select className="sel"><option>Google Cast</option></select>
            </label>
            <button className="btn btn-d" style={{ height: 40 }} disabled={!ip.trim()}>{tr('Подключить', 'Connect')}</button>
          </form>
          {err && <div className="err">{err}</div>}
          <p style={{ margin: 0, fontSize: 'calc(12px * var(--fs))', color: 'var(--mut)' }}>{tr('DLNA и AirPlay появятся в следующих версиях. Трансляция идёт напрямую с этого ПК по локальной сети, в интернет ничего не отправляется.', 'DLNA and AirPlay come in later versions. Streams go straight from this PC over your LAN.')}</p>
        </section>

        {active ? <Remote s={active} /> : (
          <aside style={{ flex: '1 1 380px', minWidth: 0, borderRadius: 24, background: 'var(--bg2)', border: '1px solid var(--line)', color: 'var(--fg)', padding: 20, display: 'flex', flexDirection: 'column', gap: 12, alignSelf: 'flex-start' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 'calc(12px * var(--fs))', color: 'var(--mut2)' }}><Icon name="cast" size={16} />{tr('Пульт', 'Remote')}</div>
            <div style={{ fontSize: 'calc(14px * var(--fs))', color: 'var(--mut2)', lineHeight: 1.5 }}>{tr('Сейчас ничего не транслируется. Откройте фильм и нажмите «На ТВ» — здесь появится пульт.', 'Nothing is casting. Open a title and press “To TV” — the remote appears here.')}</div>
          </aside>
        )}
      </div>
    </main>
  );
}

function Remote({ s }: { s: CastStatus }) {
  const nav = useNavigate();
  const { toast } = useApp();
  const [item, setItem] = useState<Item | null>(null);
  const [seekPos, setSeekPos] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setItem(null);
    if (s.itemId) api.get<{ item: Item }>(`/api/items/${s.itemId}`).then((r) => setItem(r.item)).catch(() => {});
  }, [s.itemId]);
  const act = async (action: string, body?: object) => {
    try { await api.post(`/api/cast/${encodeURIComponent(s.deviceId)}/${action}`, body); } catch (e: any) { toast(e.message); }
  };
  const reload = async (audio: number | undefined, sub: number | undefined) => {
    setBusy(true);
    try {
      await api.post(`/api/cast/${encodeURIComponent(s.deviceId)}/load`, s.torrent ? { torrent: s.torrent, file: s.file, start: s.position } : { fileId: s.fileId, start: s.position, audio, sub });
    } catch (e: any) { toast(e.message); } finally { setBusy(false); }
  };
  const playing = s.state === 'playing' || s.state === 'buffering';
  const pos = seekPos ?? s.position;
  const pct = s.duration ? Math.min(1, pos / s.duration) : 0;
  const file = item?.files?.[0];
  // A torrent has only a file name: show it as words, without the extension.
  const title = item ? (item.kind === 'episode' ? item.showTitle || item.title : item.title) : s.torrent ? (s.title || '').replace(/\.[a-z0-9]{2,4}$/i, '').replace(/[._]+/g, ' ') : s.title;
  const continueOnPC = async () => {
    await act('stop');
    if (s.torrent) nav(`/play/torrent/${s.torrent}?file=${s.file}`);
    else nav(`/play/${s.fileId}?item=${s.itemId}&start=${Math.floor(s.position)}`);
  };
  return (
    <aside aria-label={tr('Пульт', 'Remote')} style={{ flex: '1 1 380px', minWidth: 0, borderRadius: 24, background: 'var(--bg2)', border: '1px solid var(--line)', color: 'var(--fg)', padding: 20, display: 'flex', flexDirection: 'column', gap: 18, alignSelf: 'flex-start' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 'calc(12px * var(--fs))', color: 'var(--mut2)' }}>
        <Icon name="cast" size={16} />{tr('Идёт на', 'Playing on')} «{s.deviceName}»
        {s.state === 'buffering' && <span className="spin" style={{ width: 14, height: 14 }} />}
      </div>
      {s.error && <div style={{ fontSize: 'calc(12px * var(--fs))', color: 'var(--err-fg)' }}>{s.error}</div>}
      <div style={{ display: 'flex', gap: 14, alignItems: 'center' }}>
        <Art src={item?.poster} title={title || '?'} style={{ width: 72, height: 108, borderRadius: 8, flex: 'none' }} />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4, minWidth: 0 }}>
          <span className="disp" title={title} style={{ fontSize: `calc(${(title || '').length > 30 ? 20 : 26}px * var(--fs))`, fontWeight: 600, lineHeight: 1.05, overflowWrap: 'anywhere', display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{title}</span>
          {item?.kind === 'episode' && <span style={{ fontSize: 'calc(12px * var(--fs))', color: 'var(--mut2)' }}>С{item.season}·Э{item.episode} {item.epTitle}</span>}
          <span style={{ fontSize: 'calc(12px * var(--fs))', color: 'var(--mut)' }}>{s.audioLabel || '—'} · {tr('субтитры', 'subtitles')} {s.subLabel || tr('выкл.', 'off')}</span>
        </div>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <input type="range" className="rng" min={0} max={Math.max(1, Math.round(s.duration))} value={Math.round(pos)} aria-label={tr('Позиция', 'Position')} aria-valuetext={fmtTime(pos)}
          onChange={(e) => setSeekPos(Number(e.target.value))}
          onMouseUp={() => { if (seekPos !== null) { act('seek', { position: seekPos }); setTimeout(() => setSeekPos(null), 1500); } }}
          onKeyUp={() => { if (seekPos !== null) { act('seek', { position: seekPos }); setTimeout(() => setSeekPos(null), 1500); } }}
          onTouchEnd={() => { if (seekPos !== null) { act('seek', { position: seekPos }); setTimeout(() => setSeekPos(null), 1500); } }}
          style={{ width: '100%', background: `linear-gradient(90deg,var(--acc) ${pct * 100}%,var(--line2) ${pct * 100}%)` }} />
        <div className="mono" style={{ display: 'flex', justifyContent: 'space-between', fontSize: 'calc(11px * var(--fs))', color: 'var(--mut)' }}><span>{fmtTime(pos)}</span><span>{fmtTime(s.duration)}</span></div>
      </div>
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', gap: 14 }}>
        <button className="rb" aria-label={tr('Назад 30 секунд', 'Back 30 seconds')} onClick={() => act('seek', { position: Math.max(0, s.position - 30) })}><Icon name="back10" /></button>
        <button className="rb" aria-label={playing ? tr('Пауза', 'Pause') : tr('Продолжить', 'Play')} onClick={() => act(playing ? 'pause' : 'play')} style={{ width: 72, height: 72, background: 'var(--fg)', color: 'var(--bg)' }}>{playing ? <PauseGlyph size={26} /> : <PlayGlyph size={24} />}</button>
        <button className="rb" aria-label={tr('Вперёд 30 секунд', 'Forward 30 seconds')} onClick={() => act('seek', { position: s.position + 30 })}><Icon name="fwd10" /></button>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <button className="rb" aria-label={tr('Тише', 'Volume down')} style={{ width: 44, height: 44 }} onClick={() => act('volume', { level: Math.max(0, s.volume - 0.1) })}>−</button>
        <div aria-hidden="true" style={{ flex: 1, height: 4, borderRadius: 2, background: 'var(--chip)' }}><div style={{ width: `${Math.round(s.volume * 100)}%`, height: 4, borderRadius: 2, background: 'var(--fg)' }} /></div>
        <button className="rb" aria-label={tr('Громче', 'Volume up')} style={{ width: 44, height: 44 }} onClick={() => act('volume', { level: Math.min(1, s.volume + 0.1) })}>+</button>
        <button className="rb" aria-label={tr('Остановить трансляцию', 'Stop casting')} style={{ width: 44, height: 44 }} onClick={() => act('stop')}><StopGlyph /></button>
      </div>
      {file && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2,minmax(0,1fr))', gap: 8 }}>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 4, fontSize: 'calc(11px * var(--fs))', color: 'var(--mut)' }}>{tr('Аудио', 'Audio')}
            <select className="sel" disabled={busy} value={s.audio} onChange={(e) => reload(Number(e.target.value), s.sub >= 0 ? s.sub : undefined)}>
              {(file.audio || []).map((a, i) => <option key={a.index} value={a.index}>{(a.lang || '').toUpperCase() || `${tr('Дорожка', 'Track')} ${i + 1}`}{a.title ? ` (${a.title})` : ''} · {a.codec.toUpperCase()}</option>)}
            </select>
          </label>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 4, fontSize: 'calc(11px * var(--fs))', color: 'var(--mut)' }}>{tr('Субтитры', 'Subtitles')}
            <select className="sel" disabled={busy} value={s.sub} onChange={(e) => reload(s.audio >= 0 ? s.audio : undefined, Number(e.target.value))}>
              <option value={-1}>{tr('Выкл.', 'Off')}</option>
              {(file.subs || []).map((t, i) => <option key={i} value={i}>{(t.lang || '').toUpperCase() || `#${i + 1}`}{t.title ? ` (${t.title})` : ''} ({tr('вжечь', 'burn in')})</option>)}
            </select>
          </label>
        </div>
      )}
      <div style={{ padding: 12, borderRadius: 12, background: 'var(--chip)', display: 'flex', flexDirection: 'column', gap: 6, fontSize: 'calc(12px * var(--fs))' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}><span style={{ color: 'var(--mut)' }}>{tr('Транскод', 'Transcode')}</span><span style={{ textAlign: 'right' }}>{s.transcode || tr('нет, напрямую', 'none, direct')}</span></div>
        {s.encoder && <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}><span style={{ color: 'var(--mut)' }}>{tr('Ускорение', 'Acceleration')}</span><span>{s.encoder.toUpperCase()}</span></div>}
        {/tonemap|HDR/i.test(s.transcode) && <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}><span style={{ color: 'var(--mut)' }}>HDR → SDR</span><span>{tr('тонмаппинг вкл.', 'tonemapping on')}</span></div>}
      </div>
      <button className="btn btn-d" style={{ justifyContent: 'center' }} onClick={continueOnPC}>{tr('Продолжить на ПК с', 'Continue on PC from')} {fmtTime(s.position)}</button>
    </aside>
  );
}
