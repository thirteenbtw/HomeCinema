import { useEffect, useState } from 'react';
import { api } from '../api';
import { tr } from '../i18n';
import { Modal } from './CastDialog';
import { Icon } from './Icon';

type Listing = { path: string; parent: string; dirs: { name: string; path: string }[] };

// Server-side folder browser: the server runs on this PC, so it lists the
// PC's own drives (works the same from a phone on the LAN).
export function FolderPicker({ title, initial, onPick, onClose }: { title: string; initial?: string; onPick: (path: string) => void; onClose: () => void }) {
  const [path, setPath] = useState(initial || '');
  const [list, setList] = useState<Listing | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [typed, setTyped] = useState(initial || '');
  useEffect(() => {
    setErr(null);
    api.get<Listing>(`/api/fs?path=${encodeURIComponent(path)}`).then((l) => { setList(l); setTyped(l.path); }).catch((e) => setErr(e.message));
  }, [path]);
  return (
    <Modal title={title} onClose={onClose}>
      <form className="row" onSubmit={(e) => { e.preventDefault(); setPath(typed); }}>
        <input className="inp mono" style={{ flex: 1, fontSize: 'calc(12px * var(--fs))' }} value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="D:\Фильмы" aria-label={tr('Путь', 'Path')} />
        <button className="btn btn-l sm">{tr('Открыть', 'Open')}</button>
      </form>
      {err && <div className="err">{err}</div>}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2, maxHeight: 340, overflow: 'auto', border: '1px solid var(--line)', borderRadius: 12, padding: 6 }}>
        {list?.path && <button className="sn" style={{ border: 0, background: 'transparent', cursor: 'pointer', gap: 8 }} onClick={() => setPath(list.parent)}><Icon name="left" size={16} />{list.parent ? '..' : tr('Диски', 'Drives')}</button>}
        {list?.dirs.map((d) => (
          <button key={d.path} className="sn" style={{ border: 0, background: 'transparent', cursor: 'pointer', gap: 8, textAlign: 'left' }} onClick={() => setPath(d.path)}><Icon name="folder" size={16} />{d.name}</button>
        ))}
        {list && list.dirs.length === 0 && <div className="sub" style={{ padding: 10 }}>{tr('Вложенных папок нет', 'No subfolders')}</div>}
      </div>
      <div className="row">
        <button className="btn btn-d sm" disabled={!list?.path} onClick={() => list?.path && onPick(list.path)}>{tr('Выбрать эту папку', 'Use this folder')}</button>
        <span className="mono sub" style={{ wordBreak: 'break-all' }}>{list?.path}</span>
      </div>
    </Modal>
  );
}
