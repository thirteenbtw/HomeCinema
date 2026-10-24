import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, type Profile } from '../api';
import { useApp } from '../state';
import { tr } from '../i18n';
import { Modal } from '../components/CastDialog';
import { Icon } from '../components/Icon';

const colors = ['#5B6EE1', '#23806B', '#C8232D', '#B35F00', '#2D5B8C', '#8E3B4F'];
const MAX = 5;

function Avatar({ p, size = 56 }: { p: { name: string; color: string }; size?: number }) {
  return <span aria-hidden="true" style={{ width: size, height: size, borderRadius: '50%', background: p.color || 'var(--acc)', color: '#FFFFFF', display: 'grid', placeItems: 'center', fontWeight: 800, fontSize: size * 0.36 }}>{p.name.slice(0, 1).toUpperCase()}</span>;
}

// "Who's watching?" (FR-19): pick a profile, PIN when set.
export default function ProfilesPage() {
  const { state, reload } = useApp();
  const nav = useNavigate();
  const [pinFor, setPinFor] = useState<Profile | null>(null);
  const [pin, setPin] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const pick = async (p: Profile, code = '') => {
    setErr(null);
    try { await api.post(`/api/profiles/${p.id}/select`, { pin: code }); await reload(); nav('/'); } catch (e: any) { setErr(e.message); setPin(''); }
  };
  const list = state?.profiles || [];
  return (
    <main className="main pad" style={{ alignItems: 'center', justifyContent: 'center', minHeight: '80vh', textAlign: 'center' }}>
      <h1 className="h1" style={{ fontSize: 'calc(34px * var(--fs))' }}>{tr('Кто смотрит?', "Who's watching?")}</h1>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 28, justifyContent: 'center' }}>
        {list.map((p) => (
          <button key={p.id} onClick={() => (p.hasPin ? setPinFor(p) : pick(p))} aria-current={p.id === state?.profileId}
            style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10, border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--fg)', fontFamily: 'inherit' }}>
            <span style={{ borderRadius: '50%', boxShadow: p.id === state?.profileId ? '0 0 0 3px var(--bg), 0 0 0 5px var(--acc)' : 'none' }}><Avatar p={p} size={96} /></span>
            <span style={{ fontWeight: 700, fontSize: 'calc(15px * var(--fs))' }}>{p.name}</span>
            <span className="sub">{[p.hasPin ? 'PIN' : '', p.isKid ? tr('Детский', 'Kids') : ''].filter(Boolean).join(' · ') || ' '}</span>
          </button>
        ))}
      </div>
      <button className="btn btn-l sm" onClick={() => setEditing(true)}><Icon name="edit" size={16} />{tr('Управление профилями', 'Manage profiles')}</button>
      {err && !pinFor && <div className="err">{err}</div>}
      {pinFor && (
        <Modal title={`${tr('PIN для', 'PIN for')} ${pinFor.name}`} onClose={() => { setPinFor(null); setErr(null); setPin(''); }}>
          <form className="row" onSubmit={(e) => { e.preventDefault(); pick(pinFor, pin); }}>
            <input className="inp mono" type="password" inputMode="numeric" autoFocus maxLength={8} value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, ''))} aria-label="PIN" style={{ flex: 1, fontSize: 'calc(20px * var(--fs))', letterSpacing: '.4em', textAlign: 'center' }} />
            <button className="btn btn-d sm" disabled={pin.length < 4}>{tr('Войти', 'Enter')}</button>
          </form>
          {err && <div className="err">{err}</div>}
        </Modal>
      )}
      {editing && <Modal title={tr('Профили', 'Profiles')} onClose={() => setEditing(false)} wide><ProfilesEditor /></Modal>}
    </main>
  );
}

// Profile list with create/edit, used on the settings page.
export function ProfilesEditor() {
  const { state, reload, toast } = useApp();
  const [edit, setEdit] = useState<Partial<Profile> & { pin?: string; clearPin?: boolean } | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const list = state?.profiles || [];
  const save = async () => {
    if (!edit) return;
    setErr(null);
    const body: Record<string, unknown> = { name: edit.name, color: edit.color, isKid: !!edit.isKid };
    if (edit.pin) body.pin = edit.pin; else if (edit.clearPin) body.pin = '';
    try {
      if (edit.id) await api.patch(`/api/profiles/${edit.id}`, body); else await api.post('/api/profiles', body);
      setEdit(null); await reload();
    } catch (e: any) { setErr(e.message); }
  };
  const remove = async () => {
    if (!edit?.id) return;
    try { await api.del(`/api/profiles/${edit.id}`); setEdit(null); await reload(); toast(tr('Профиль удалён', 'Profile removed')); } catch (e: any) { setErr(e.message); }
  };
  return (
    <>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 14 }}>
        {list.map((p, i) => (
          <button key={p.id} onClick={() => setEdit({ ...p })} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6, border: 0, background: 'transparent', cursor: 'pointer', fontFamily: 'inherit', color: 'var(--fg)', minWidth: 72 }}>
            <Avatar p={p} />
            <span style={{ fontSize: 'calc(12px * var(--fs))', fontWeight: 600 }}>{p.name}</span>
            <span style={{ fontSize: 'calc(11px * var(--fs))', color: 'var(--mut)' }}>{i === 0 ? tr('Владелец', 'Owner') : [p.hasPin ? 'PIN' : '', p.isKid ? tr('Детский', 'Kids') : ''].filter(Boolean).join(' · ') || ' '}</span>
          </button>
        ))}
        {list.length < MAX && (
          <button onClick={() => setEdit({ name: '', color: colors[list.length % colors.length] })} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6, border: 0, background: 'transparent', cursor: 'pointer', fontFamily: 'inherit', color: 'var(--fg)', minWidth: 72 }}>
            <span style={{ width: 56, height: 56, borderRadius: '50%', background: '#8E9196', color: '#FFFFFF', display: 'grid', placeItems: 'center', fontWeight: 800, fontSize: 'calc(20px * var(--fs))' }}>+</span>
            <span style={{ fontSize: 'calc(12px * var(--fs))', fontWeight: 600 }}>{tr('Добавить', 'Add')}</span>
            <span style={{ fontSize: 'calc(11px * var(--fs))', color: 'var(--mut)' }}>{list.length}/{MAX}</span>
          </button>
        )}
      </div>
      {edit && (
        <Modal title={edit.id ? tr('Профиль', 'Profile') : tr('Новый профиль', 'New profile')} onClose={() => { setEdit(null); setErr(null); }}>
          <form style={{ display: 'flex', flexDirection: 'column', gap: 12 }} onSubmit={(e) => { e.preventDefault(); save(); }}>
            <label className="fld">{tr('Имя', 'Name')}<input className="inp" autoFocus value={edit.name || ''} onChange={(e) => setEdit({ ...edit, name: e.target.value })} maxLength={24} /></label>
            <div role="radiogroup" aria-label={tr('Цвет', 'Color')} style={{ display: 'flex', gap: 8 }}>
              {colors.map((c) => <button type="button" key={c} role="radio" aria-checked={edit.color === c} aria-label={c} onClick={() => setEdit({ ...edit, color: c })} style={{ width: 36, height: 36, borderRadius: '50%', border: 0, background: c, cursor: 'pointer', boxShadow: edit.color === c ? '0 0 0 2px var(--bg), 0 0 0 4px ' + c : 'none' }} />)}
            </div>
            <label className="fld">PIN {edit.hasPin && <span>· {tr('задан; оставьте пустым, чтобы не менять', 'set; leave empty to keep')}</span>}
              <input className="inp mono" type="password" inputMode="numeric" maxLength={8} value={edit.pin || ''} onChange={(e) => setEdit({ ...edit, pin: e.target.value.replace(/\D/g, '') })} placeholder={tr('4–8 цифр, необязательно', '4–8 digits, optional')} />
            </label>
            {edit.hasPin && <label className="chk"><input type="checkbox" checked={!!edit.clearPin} onChange={(e) => setEdit({ ...edit, clearPin: e.target.checked })} />{tr('Убрать PIN', 'Remove PIN')}</label>}
            <label className="chk"><input type="checkbox" checked={!!edit.isKid} onChange={(e) => setEdit({ ...edit, isKid: e.target.checked })} />{tr('Детский профиль', 'Kids profile')}</label>
            {err && <div className="err">{err}</div>}
            <div className="row">
              <button className="btn btn-d sm" disabled={!edit.name?.trim() || (!!edit.pin && edit.pin.length < 4)}>{tr('Сохранить', 'Save')}</button>
              {edit.id && list.length > 1 && <button type="button" className="btn btn-o sm" onClick={remove}>{tr('Удалить профиль', 'Delete profile')}</button>}
            </div>
          </form>
        </Modal>
      )}
    </>
  );
}
