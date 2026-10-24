import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, fmtTime } from '../api';
import { useApp } from '../state';
import { t, tr } from '../i18n';
import { Icon } from './Icon';

export function Modal({ title, onClose, children, wide }: { title: string; onClose: () => void; children: ReactNode; wide?: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLElement>('button, input, select, a')?.focus();
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    return () => { window.removeEventListener('keydown', onKey); prev?.focus(); };
  }, [onClose]);
  return (
    <div className="modal-back" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="modal" role="dialog" aria-modal="true" aria-label={title} ref={ref} style={wide ? { width: 'min(820px,100%)' } : undefined}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <h2 className="h2" style={{ fontSize: 'calc(18px * var(--fs))' }}>{title}</h2>
          <button className="ri" aria-label={t('close')} onClick={onClose} style={{ marginLeft: 'auto' }}><Icon name="close" /></button>
        </div>
        {children}
      </div>
    </div>
  );
}

// "Send to TV" device picker (FR-12): discovered devices plus manual IP.
export function CastDialog() {
  const { castTarget, openCast, devices, toast } = useApp();
  const nav = useNavigate();
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [ip, setIp] = useState('');
  const [scanning, setScanning] = useState(false);
  if (!castTarget) return null;
  const close = () => { openCast(null); setErr(null); };

  const send = async (id: string) => {
    setBusy(id); setErr(null);
    try {
      await api.post(`/api/cast/${encodeURIComponent(id)}/load`, {
        fileId: castTarget.fileId, torrent: castTarget.torrent, file: castTarget.file ?? -1, start: castTarget.start || 0,
        audio: castTarget.audio, sub: castTarget.sub,
      });
      toast(tr('Отправлено на ТВ', 'Sent to TV'));
      close();
      nav('/devices');
    } catch (e: any) {
      setErr(e.message);
    } finally { setBusy(null); }
  };

  const addIp = async () => {
    setErr(null); setBusy('ip');
    try {
      const d = await api.post<{ id: string }>('/api/devices', { ip });
      await send(d.id);
    } catch (e: any) { setErr(e.message); setBusy(null); }
  };

  const rescan = async () => {
    setScanning(true);
    try { await api.post('/api/devices/scan'); } finally { setScanning(false); }
  };

  const online = devices.filter((d) => d.online);
  return (
    <Modal title={t('pickDevice')} onClose={close}>
      {castTarget.title && <div className="sub">{castTarget.title}{castTarget.start ? ` · ${tr('с', 'from')} ${fmtTime(castTarget.start)}` : ''}</div>}
      {err && <div className="err">{err}</div>}
      {online.length === 0 && <div className="sub">{t('noDevices')}</div>}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {online.map((d) => (
          <button key={d.id} className="btn btn-l" style={{ justifyContent: 'flex-start', height: 'auto', padding: '12px 16px', borderRadius: 14 }} disabled={!!busy} onClick={() => send(d.id)}>
            <Icon name="tv" />
            <span style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 2 }}>
              <span>{d.name}</span>
              <span className="sub" style={{ fontWeight: 500 }}>{d.model || 'Google Cast'} · {d.addr}{d.busy ? ` · ${tr('уже транслирует', 'playing')}` : ''}</span>
            </span>
            {busy === d.id && <span className="spin" style={{ marginLeft: 'auto' }} />}
          </button>
        ))}
      </div>
      <form className="row" onSubmit={(e) => { e.preventDefault(); addIp(); }} style={{ alignItems: 'flex-end' }}>
        <label className="fld" style={{ flex: '1 1 200px' }}>{tr('Не нашли ТВ? Добавьте по IP', 'TV missing? Add by IP')}
          <input className="inp mono" value={ip} onChange={(e) => setIp(e.target.value)} placeholder="192.168.1.___" inputMode="decimal" />
        </label>
        <button className="btn btn-d sm" disabled={!ip || !!busy}>{tr('Подключить', 'Connect')}</button>
        <button type="button" className="btn btn-l sm" onClick={rescan} disabled={scanning}><Icon name="refresh" size={16} />{tr('Искать снова', 'Rescan')}</button>
      </form>
    </Modal>
  );
}
