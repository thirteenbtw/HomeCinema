import { useState, type ReactNode } from 'react';
import { api, type Library } from '../api';
import { useApp, useAsync } from '../state';
import { setLang, tr } from '../i18n';
import { FolderPicker } from '../components/FolderPicker';
import { Icon } from '../components/Icon';

function Frame({ children }: { children: ReactNode }) {
  const { prefs } = useApp();
  return (
    <div className={`app th-${prefs.theme} ac-${prefs.accent}`} style={{ alignItems: 'center', justifyContent: 'center', padding: 16 }}>
      <div aria-hidden="true" style={{ position: 'fixed', inset: 0, pointerEvents: 'none', opacity: 'var(--amb)' }}>
        <div className="blob" style={{ width: 620, height: 620, left: '10%', top: -240, background: 'var(--acc)' }} />
        <div className="blob" style={{ width: 520, height: 520, right: -120, bottom: -160, background: '#2D6FB5' }} />
      </div>
      <div className="glass" style={{ position: 'relative', width: 'min(620px,100%)', borderRadius: 24, border: '1px solid var(--line)', boxShadow: '0 30px 80px var(--sh)', padding: 28, display: 'flex', flexDirection: 'column', gap: 18 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <span style={{ width: 32, height: 32, borderRadius: 9, background: 'var(--acc)', display: 'grid', placeItems: 'center' }}><svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path d="M8 5l11 7-11 7z" fill="#FFFFFF" /></svg></span>
          <span className="disp" style={{ fontSize: 'calc(20px * var(--fs))', fontWeight: 600, letterSpacing: '.04em' }}>HomeCinema</span>
        </div>
        {children}
      </div>
    </div>
  );
}

export function Login() {
  const { reload } = useApp();
  const [pw, setPw] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const go = async () => {
    setErr(null);
    try { await api.post('/api/auth/login', { password: pw }); await reload(); } catch (e: any) { setErr(e.message); setPw(''); }
  };
  return (
    <Frame>
      <h1 className="h1">{tr('Вход', 'Sign in')}</h1>
      <div className="sub">{tr('Медиатека защищена паролем. Его задают в настройках на ПК, где установлен HomeCinema.', 'This library is password-protected.')}</div>
      <form className="row" onSubmit={(e) => { e.preventDefault(); go(); }}>
        <input className="inp" type="password" autoFocus value={pw} onChange={(e) => setPw(e.target.value)} aria-label={tr('Пароль', 'Password')} style={{ flex: 1 }} autoComplete="current-password" />
        <button className="btn btn-r sm" disabled={!pw}>{tr('Войти', 'Sign in')}</button>
      </form>
      {err && <div className="err">{err}</div>}
    </Frame>
  );
}

// First run (FR-21): language, library folders, TMDB key.
export function Setup() {
  const { reload, state } = useApp();
  const [step, setStep] = useState(0);
  const [lang, setL] = useState(state?.uiLang || 'ru');
  const libs = useAsync(() => api.get<{ libraries: Library[] }>('/api/libraries'), []);
  const [picker, setPicker] = useState<null | string>(null);
  const [key, setKey] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const pickLang = async (l: string) => { setL(l); setLang(l); await api.put('/api/settings', { uiLang: l }); };
  const addLib = async (path: string, kind: string) => {
    setPicker(null); setErr(null);
    try { await api.post('/api/libraries', { path, kind }); libs.reload(); } catch (e: any) { setErr(e.message); }
  };
  const finish = async (withKey: boolean) => {
    setErr(null); setBusy(true);
    try {
      if (withKey && key.trim()) await api.put('/api/settings', { tmdbKey: key.trim() });
      await api.put('/api/settings', { setupDone: '1' });
      await reload();
    } catch (e: any) { setErr(e.message); } finally { setBusy(false); }
  };
  const dots = <div style={{ display: 'flex', gap: 6 }}>{[0, 1, 2].map((i) => <span key={i} style={{ height: 6, width: i === step ? 22 : 6, borderRadius: 3, background: i === step ? 'var(--acc)' : 'var(--line2)', transition: 'width .3s' }} />)}</div>;

  return (
    <Frame>
      {dots}
      {step === 0 && (
        <>
          <h1 className="h1">{tr('Добро пожаловать', 'Welcome')}</h1>
          <div style={{ fontSize: 'calc(14px * var(--fs))', color: 'var(--fg2)', lineHeight: 1.6 }}>{tr('HomeCinema собирает фильмы и сериалы с дисков этого ПК в красивую библиотеку, играет их здесь и отправляет на ТВ. Всё работает внутри вашей сети, телеметрии нет.', 'HomeCinema turns the movies and shows on this PC into a library, plays them here and casts to your TV. Everything stays on your network.')}</div>
          <div role="radiogroup" aria-label={tr('Язык', 'Language')} style={{ display: 'flex', gap: 8 }}>
            {[['ru', 'Русский'], ['en', 'English']].map(([l, n]) => <button key={l} role="radio" aria-checked={lang === l} className={'pill' + (lang === l ? ' on' : '')} onClick={() => pickLang(l)}>{n}</button>)}
          </div>
          <div className="row"><button className="btn btn-r" onClick={() => setStep(1)}>{tr('Начать', 'Start')}</button></div>
        </>
      )}
      {step === 1 && (
        <>
          <h1 className="h1">{tr('Где ваши фильмы?', 'Where are your movies?')}</h1>
          <div className="sub">{tr('Добавьте одну или несколько папок. Подпапки и новые файлы подхватываются автоматически.', 'Add one or more folders. Subfolders and new files are picked up automatically.')}</div>
          {libs.data?.libraries.map((l) => (
            <div key={l.id} className="row" style={{ padding: 10, borderRadius: 12, background: 'var(--bg2)' }}><Icon name="folder" size={18} /><span className="mono" style={{ fontSize: 'calc(12px * var(--fs))', flex: 1, wordBreak: 'break-all' }}>{l.path}</span><span className="sub">{l.kind === 'movies' ? tr('Фильмы', 'Movies') : l.kind === 'shows' ? tr('Сериалы', 'Shows') : tr('Авто', 'Auto')}</span></div>
          ))}
          <div className="row">
            <button className="btn btn-d sm" onClick={() => setPicker('movies')}><Icon name="film" size={16} />{tr('Папка с фильмами', 'Movies folder')}</button>
            <button className="btn btn-d sm" onClick={() => setPicker('shows')}><Icon name="tv" size={16} />{tr('Папка с сериалами', 'Shows folder')}</button>
            <button className="btn btn-l sm" onClick={() => setPicker('auto')}><Icon name="folder" size={16} />{tr('Смешанная', 'Mixed')}</button>
          </div>
          {err && <div className="err">{err}</div>}
          <div className="row"><button className="btn btn-r" onClick={() => setStep(2)}>{libs.data?.libraries.length ? tr('Дальше', 'Next') : tr('Пропустить', 'Skip')}</button><button className="btn btn-l" onClick={() => setStep(0)}>{tr('Назад', 'Back')}</button></div>
        </>
      )}
      {step === 2 && (
        <>
          <h1 className="h1">{tr('Постеры и описания', 'Posters and descriptions')}</h1>
          <div style={{ fontSize: 'calc(14px * var(--fs))', color: 'var(--fg2)', lineHeight: 1.6 }}>
            {tr('Обложки, описания и актёров даёт TMDB. Нужен бесплатный ключ API: зарегистрируйтесь на ', 'Artwork and descriptions come from TMDB. Get a free API key at ')}
            <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">themoviedb.org</a>{tr(' и вставьте ключ ниже. Можно сделать это позже в настройках.', ' and paste it below. You can do this later in Settings.')}
          </div>
          <input className="inp mono" value={key} onChange={(e) => setKey(e.target.value)} placeholder="API key v3 / токен v4" aria-label={tr('Ключ TMDB', 'TMDB key')} autoComplete="off" />
          {err && <div className="err">{err}</div>}
          <div className="row">
            <button className="btn btn-r" disabled={busy || !key.trim()} onClick={() => finish(true)}>{busy ? <span className="spin" /> : tr('Проверить и готово', 'Verify and finish')}</button>
            <button className="btn btn-l" disabled={busy} onClick={() => finish(false)}>{tr('Позже', 'Later')}</button>
          </div>
        </>
      )}
      {picker && <FolderPicker title={tr('Выберите папку', 'Choose folder')} onPick={(p) => addLib(p, picker)} onClose={() => setPicker(null)} />}
    </Frame>
  );
}
