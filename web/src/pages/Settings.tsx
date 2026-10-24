import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { api, type Check, type Indexer, type Library, type ScanStatus, type SearchPrefs, type Settings as S, type TorrentSettings } from '../api';
import { useApp, useAsync } from '../state';
import { setTextScale, textScale, textSteps } from '../zoom';
import { setLang, tr } from '../i18n';
import { Icon } from '../components/Icon';
import { FolderPicker } from '../components/FolderPicker';
import { Modal } from '../components/CastDialog';
import { ProfilesEditor } from './Profiles';

const sections = [
  ['look', 'Оформление', 'Appearance'], ['lib', 'Библиотека и TMDB', 'Library & TMDB'], ['play', 'Воспроизведение', 'Playback'],
  ['torrents', 'Торренты и VPN', 'Torrents & VPN'], ['access', 'Доступ и язык', 'Access & language'], ['prof', 'Профили и копии', 'Profiles & backup'], ['diag', 'Диагностика', 'Diagnostics'],
] as const;

// TmdbAccess picks how the server reaches TMDB: through the public mirrors
// Lampa users rely on (and TMDB itself), directly, or via a custom mirror.
function TmdbAccess({ st, save }: { st: S; save: (p: Partial<Record<keyof S, string>>) => Promise<S> }) {
  const [sel, setSel] = useState(st.tmdbVia || 'cub');
  const [apiUrl, setApiUrl] = useState(st.tmdbApi || '');
  const [imgUrl, setImgUrl] = useState(st.tmdbImg || '');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const apply = async (v: Partial<Record<keyof S, string>>) => {
    setBusy(true); setMsg(null);
    try {
      await save(v);
      setMsg({ ok: true, text: st.tmdbKeySet ? tr('TMDB отвечает', 'TMDB responds') : tr('Сохранено', 'Saved') });
    } catch (e: any) { setMsg({ ok: false, text: e.message }); setSel(st.tmdbVia || 'cub'); } finally { setBusy(false); }
  };
  const pick = (v: string) => {
    setSel(v as S['tmdbVia']);
    if (v !== 'custom') apply({ tmdbVia: v });
  };
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div className="row">
        <label className="fld" style={{ flex: '0 1 340px' }}>{tr('Подключение к TMDB', 'Connecting to TMDB')}
          <select className="sel" value={sel} onChange={(e) => pick(e.target.value)} disabled={busy}>
            <option value="cub">{tr('Через зеркала Lampa (работает без VPN)', 'Via Lampa mirrors (no VPN needed)')}</option>
            <option value="direct">{tr('Напрямую', 'Direct')}</option>
            <option value="custom">{tr('Своё зеркало…', 'Custom mirror…')}</option>
          </select>
        </label>
        {busy && <span className="spin" />}
      </div>
      {sel === 'custom' && (
        <form className="row" onSubmit={(e) => { e.preventDefault(); apply({ tmdbVia: 'custom', tmdbApi: apiUrl.trim(), tmdbImg: imgUrl.trim() }); }}>
          <label className="fld" style={{ flex: '1 1 240px' }}>{tr('Адрес API', 'API address')}
            <input className="inp mono" style={{ fontSize: 'calc(12px * var(--fs))' }} value={apiUrl} onChange={(e) => setApiUrl(e.target.value)} placeholder="https://…/3" />
          </label>
          <label className="fld" style={{ flex: '1 1 240px' }}>{tr('Адрес картинок', 'Images address')}
            <input className="inp mono" style={{ fontSize: 'calc(12px * var(--fs))' }} value={imgUrl} onChange={(e) => setImgUrl(e.target.value)} placeholder="https://…/t/p" />
          </label>
          <button className="btn btn-d sm" style={{ alignSelf: 'flex-end' }} disabled={busy}>{tr('Сохранить', 'Save')}</button>
        </form>
      )}
      {msg && <div className={msg.ok ? 'ok' : 'err'}>{msg.text}</div>}
    </div>
  );
}

export default function SettingsPage() {
  const loc = useLocation();
  const [cur, setCur] = useState(loc.hash.slice(1) || 'look');
  const { data: st, setData: setSt, reload } = useAsync(() => api.get<S>('/api/settings'), []);
  useEffect(() => {
    const id = loc.hash.slice(1);
    if (id) { setCur(id); setTimeout(() => document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' }), 50); }
  }, [loc.hash]);
  const save = async (patch: Partial<Record<keyof S | 'tmdbKey' | 'password', string>>) => {
    const next = await api.put<S>('/api/settings', patch);
    setSt(next);
    return next;
  };
  return (
    <main className="main pad">
      <div><h1 className="h1">{tr('Настройки', 'Settings')}</h1><div className="sub">{tr('Всё хранится локально. Телеметрии нет.', 'Everything stays on this PC. No telemetry.')}</div></div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 32, alignItems: 'flex-start' }}>
        <nav aria-label={tr('Разделы настроек', 'Settings sections')} style={{ flex: '1 1 200px', display: 'flex', flexDirection: 'column', gap: 2, position: 'sticky', top: 24 }}>
          {sections.map(([id, ru, en]) => <a key={id} className={'sn' + (cur === id ? ' on' : '')} href={`#${id}`} onClick={() => setCur(id)}>{tr(ru, en)}</a>)}
        </nav>
        <div style={{ flex: '999 1 560px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 16 }}>
          <Look />
          <LibrarySection st={st} save={save} />
          {st && <Playback st={st} save={save} />}
          <TorrentSection />
          {st && <Access st={st} save={save} reload={reload} />}
          <Card id="prof" title={tr('Профили', 'Profiles')} note={tr('до 5 · у каждого своя тема и история', 'up to 5 · each with its own theme and history')}>
            <ProfilesEditor />
            <Backup />
          </Card>
          <Diagnostics />
        </div>
      </div>
    </main>
  );
}

function Card({ id, title, note, action, children }: { id: string; title: string; note?: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section id={id} className="card glass" style={{ scrollMarginTop: 24 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
        <div><h2 className="h2">{title}</h2>{note && <div className="sub">{note}</div>}</div>
        {action && <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, flexWrap: 'wrap' }}>{action}</div>}
      </div>
      {children}
    </section>
  );
}

const themes = [
  { id: 'light', ru: 'Светлая', en: 'Light', noteRu: 'Как в макете', noteEn: 'Default', bg: '#FFFFFF', line: '#E2E2DE', mut: '#C9CBCF', card: '#F2F2F0' },
  { id: 'dark', ru: 'Тёмная', en: 'Dark', noteRu: 'Для вечера', noteEn: 'For the evening', bg: '#0E0F12', line: '#2A2D33', mut: '#4A4E56', card: '#212329' },
  { id: 'oled', ru: 'OLED', en: 'OLED', noteRu: 'Чистый чёрный', noteEn: 'True black', bg: '#000000', line: '#1F2024', mut: '#3A3C42', card: '#17181B' },
] as const;
const accents = [['red', '#C8232D', 'Красный', 'Red'], ['amber', '#B35F00', 'Янтарный', 'Amber'], ['blue', '#2563EB', 'Синий', 'Blue'], ['green', '#16803C', 'Зелёный', 'Green']] as const;

function Look() {
  const { prefs, setPrefs } = useApp();
  const bgHint = { live: tr('Точки подсвечиваются вокруг курсора', 'Dots light up around the cursor'), static: tr('Сетка точек без движения', 'Dot grid, no motion'), off: tr('Чистый фон темы', 'Plain theme background') }[prefs.bg];
  return (
    <Card id="look" title={tr('Оформление', 'Appearance')} note={tr('Применяется сразу ко всему приложению и сохраняется в профиле', 'Applies instantly and is saved to your profile')}>
      <div role="radiogroup" aria-label={tr('Тема', 'Theme')} style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(180px,1fr))', gap: 10 }}>
        {themes.map((t) => {
          const on = prefs.theme === t.id;
          return (
            <button key={t.id} className="tcard" role="radio" aria-checked={on} onClick={() => setPrefs({ theme: t.id })} style={{ border: on ? '2px solid var(--acc)' : '1px solid var(--line2)' }}>
              <div aria-hidden="true" style={{ height: 96, borderRadius: 10, background: t.bg, border: `1px solid ${t.line}`, display: 'flex', overflow: 'hidden' }}>
                <div style={{ width: 18, borderRight: `1px solid ${t.line}`, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 5, paddingTop: 8 }}>
                  <span style={{ width: 8, height: 8, borderRadius: 2, background: 'var(--acc)' }} /><span style={{ width: 6, height: 6, borderRadius: 2, background: t.mut }} /><span style={{ width: 6, height: 6, borderRadius: 2, background: t.mut }} />
                </div>
                <div style={{ flex: 1, padding: 8, display: 'flex', flexDirection: 'column', gap: 6 }}>
                  <div style={{ height: 40, borderRadius: 6, background: '#1A1F26' }} />
                  <div style={{ display: 'flex', gap: 4 }}>{[0, 1, 2].map((i) => <span key={i} style={{ flex: 1, height: 18, borderRadius: 4, background: t.card }} />)}</div>
                </div>
              </div>
              <span style={{ fontSize: 'calc(13px * var(--fs))', fontWeight: 700 }}>{tr(t.ru, t.en)}</span>
              <span className="sub" style={{ marginTop: -6 }}>{tr(t.noteRu, t.noteEn)}</span>
            </button>
          );
        })}
      </div>
      <div className="row" style={{ gap: 16 }}>
        <span style={{ fontWeight: 600, minWidth: 120 }}>{tr('Акцентный цвет', 'Accent color')}</span>
        <div role="radiogroup" aria-label={tr('Акцентный цвет', 'Accent color')} style={{ display: 'flex', gap: 10 }}>
          {accents.map(([id, c, ru, en]) => (
            <button key={id} role="radio" aria-checked={prefs.accent === id} aria-label={tr(ru, en)} onClick={() => setPrefs({ accent: id })}
              style={{ width: 44, height: 44, borderRadius: '50%', border: 0, padding: 0, background: 'transparent', cursor: 'pointer', display: 'grid', placeItems: 'center', boxShadow: prefs.accent === id ? `0 0 0 2px var(--bg), 0 0 0 4px ${c}` : 'none' }}>
              <span style={{ width: 30, height: 30, borderRadius: '50%', background: c }} />
            </button>
          ))}
        </div>
      </div>
      <div className="row" style={{ gap: 16 }}>
        <span style={{ fontWeight: 600, minWidth: 120 }}>{tr('Задний фон', 'Background')}</span>
        <div role="radiogroup" aria-label={tr('Задний фон', 'Background')} style={{ display: 'flex', gap: 4, padding: 4, borderRadius: 999, background: 'var(--chip)' }}>
          {([['live', 'Живой', 'Live'], ['static', 'Статичный', 'Static'], ['off', 'Выкл.', 'Off']] as const).map(([id, ru, en]) => (
            <button key={id} role="radio" aria-checked={prefs.bg === id} className="pill" onClick={() => setPrefs({ bg: id })} style={{ background: prefs.bg === id ? 'var(--fg)' : 'transparent', color: prefs.bg === id ? 'var(--bg)' : 'var(--fg2)' }}>{tr(ru, en)}</button>
          ))}
        </div>
        <span className="sub">{bgHint}</span>
      </div>
      <div className="row" style={{ gap: 16 }}>
        <label htmlFor="int" style={{ fontWeight: 600, minWidth: 120 }}>{tr('Яркость фона', 'Background intensity')}</label>
        <input id="int" className="rng" type="range" min={0} max={200} step={10} value={prefs.intensity} disabled={prefs.bg === 'off'} onChange={(e) => setPrefs({ intensity: Number(e.target.value) })} style={{ flex: '0 1 280px' }} />
        <span className="mono" style={{ fontSize: 'calc(12px * var(--fs))', color: 'var(--mut)' }}>{prefs.intensity}%</span>
      </div>
      <TextSize />
      <label className="chk"><input type="checkbox" checked={prefs.posterTint} onChange={(e) => setPrefs({ posterTint: e.target.checked })} />{tr('Подстраивать цвет фона под постер текущего фильма', 'Tint the background with the current poster')}</label>
      <label className="chk"><input type="checkbox" checked={prefs.trailers} onChange={(e) => setPrefs({ trailers: e.target.checked })} />{tr('Автозапуск трейлеров в шапке и при наведении на постер (без звука, с YouTube)', 'Autoplay trailers in the header and on poster hover (muted, from YouTube)')}</label>
      <div className="sub">{tr('Если в системе включено «Уменьшить движение», живой фон и параллакс замирают автоматически.', 'With the system “reduce motion” setting, the live background and parallax stay still.')}</div>
    </Card>
  );
}

// Text size of this device (like the Ctrl ± scale, it is not tied to a profile).
function TextSize() {
  const [v, setV] = useState(textScale());
  return (
    <div className="row">
      <span style={{ fontWeight: 600, minWidth: 120 }}>{tr('Размер текста', 'Text size')}</span>
      <div role="radiogroup" aria-label={tr('Размер текста', 'Text size')} style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {textSteps.map((x) => (
          <button key={x} className="pill" role="radio" aria-checked={v === x} onClick={() => { setTextScale(x); setV(x); }}
            style={v === x ? { background: 'var(--acc)', color: '#FFFFFF' } : undefined}>
            <span style={{ fontSize: `${Math.round(13 * x)}px` }}>Аа</span>{Math.round(x * 100)}%
          </button>
        ))}
      </div>
      <span className="sub">{tr('Масштаб всего интерфейса — Ctrl + «+» / «−», сброс — Ctrl + 0', 'Whole interface scale: Ctrl + plus / minus, reset: Ctrl + 0')}</span>
    </div>
  );
}

const kindLabel: Record<string, [string, string]> = { auto: ['Авто', 'Auto'], movies: ['Фильмы', 'Movies'], shows: ['Сериалы', 'Shows'], torrents: ['Загрузки', 'Downloads'], '': ['Авто', 'Auto'] };

function LibrarySection({ st, save }: { st: S | null; save: (p: any) => Promise<S> }) {
  const { scan } = useApp();
  const { data, reload } = useAsync(() => api.get<{ libraries: Library[]; scan: ScanStatus }>('/api/libraries'), []);
  const [picker, setPicker] = useState(false);
  const [kind, setKind] = useState('auto');
  const [key, setKey] = useState('');
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const s = scan || data?.scan;
  const add = async (path: string) => {
    setPicker(false);
    try { await api.post('/api/libraries', { path, kind }); reload(); } catch (e: any) { setMsg({ ok: false, text: e.message }); }
  };
  const del = async (l: Library) => { await api.del(`/api/libraries/${l.id}`); reload(); };
  const saveKey = async () => {
    setBusy(true); setMsg(null);
    try { await save({ tmdbKey: key.trim() }); setKey(''); setMsg({ ok: true, text: tr('Ключ подошёл — загружаем постеры и описания', 'Key works — fetching posters and descriptions') }); } catch (e: any) { setMsg({ ok: false, text: e.message }); } finally { setBusy(false); }
  };
  return (
    <Card id="lib" title={tr('Папки библиотеки', 'Library folders')}
      action={<>
        <button className="btn btn-l sm" onClick={() => api.post('/api/libraries/scan').then(reload)}><Icon name="refresh" size={16} />{tr('Пересканировать', 'Rescan')}</button>
        <button className="btn btn-d sm" onClick={() => setPicker(true)}><Icon name="plus" size={16} />{tr('Добавить папку', 'Add folder')}</button>
      </>}>
      {data?.libraries.length === 0 && <div className="empty">{tr('Папок пока нет. Добавьте папку с фильмами или сериалами — новые файлы будут подхватываться автоматически.', 'No folders yet. New files are picked up automatically.')}</div>}
      {data?.libraries.map((l) => (
        <div key={l.id} className="row" style={{ padding: 12, borderRadius: 12, background: 'var(--bg2)' }}>
          <Icon name="folder" size={18} />
          <span className="mono" style={{ fontSize: 'calc(12px * var(--fs))', flex: '1 1 200px', wordBreak: 'break-all' }}>{l.path}</span>
          <span className="pill" style={{ cursor: 'default', height: 26, fontSize: 'calc(11px * var(--fs))' }}>{tr(...(kindLabel[l.kind] || kindLabel.auto))}</span>
          <span style={{ fontSize: 'calc(12px * var(--fs))', color: l.online ? 'var(--mut2)' : 'var(--err-fg)' }}>
            {!l.online ? tr('Диск отключён — записи сохранены', 'Drive offline — entries kept') : s?.scanning && s.library === l.path ? `${tr('Сканирование', 'Scanning')} ${s.done}/${s.total}` : tr('Отслеживается', 'Watching')}
          </span>
          <button className="ri" style={{ width: 36, height: 36 }} aria-label={`${tr('Убрать папку', 'Remove folder')} ${l.path}`} onClick={() => del(l)}><Icon name="trash" size={16} /></button>
        </div>
      ))}
      {s && (s.matching > 0 || s.scanning) && <div className="sub">{s.scanning ? `${tr('Сканирование', 'Scanning')}: ${s.done}/${s.total}` : ''}{s.matching > 0 ? ` ${tr('Ищем в TMDB', 'Matching on TMDB')}: ${s.matching}` : ''}</div>}
      <div className="row">
        <label className="fld" style={{ flex: '0 1 180px' }}>{tr('Тип новой папки', 'New folder type')}
          <select className="sel" value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="auto">{tr('Определять автоматически', 'Detect automatically')}</option>
            <option value="movies">{tr('Только фильмы', 'Movies only')}</option>
            <option value="shows">{tr('Только сериалы', 'Shows only')}</option>
          </select>
        </label>
        <label className="fld" style={{ flex: '0 1 220px' }}>{tr('Язык метаданных', 'Metadata language')}
          <select className="sel" value={st?.metaLang || 'ru-RU'} onChange={(e) => save({ metaLang: e.target.value })}>
            <option value="ru-RU">{tr('Русский, затем English', 'Russian, then English')}</option>
            <option value="en-US">English</option>
            <option value="uk-UA">Українська, затем English</option>
          </select>
        </label>
        <form className="fld" style={{ flex: '1 1 260px' }} onSubmit={(e) => { e.preventDefault(); if (key.trim()) saveKey(); }}>
          <label htmlFor="key">{tr('Ключ TMDB', 'TMDB key')} {st?.tmdbKeySet ? <span style={{ color: 'var(--ok-fg)' }}>· {tr('задан', 'set')} ••••{st.tmdbKeyTail}</span> : <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">· {tr('получить бесплатно', 'get one free')}</a>}</label>
          <div style={{ display: 'flex', gap: 8 }}>
            <input id="key" className="inp mono" style={{ flex: 1, fontSize: 'calc(12px * var(--fs))' }} value={key} onChange={(e) => setKey(e.target.value)} placeholder={st?.tmdbKeySet ? '••••••••••••' + (st.tmdbKeyTail || '') : 'API key v3 / токен v4'} autoComplete="off" />
            <button className="btn btn-d sm" disabled={busy || !key.trim()}>{busy ? <span className="spin" /> : tr('Проверить', 'Verify')}</button>
          </div>
        </form>
      </div>
      {msg && <div className={msg.ok ? 'ok' : 'err'}>{msg.text}</div>}
      {st && <TmdbAccess st={st} save={save} />}
      <div className="sub">{tr('В TMDB уходят только названия и годы для поиска. Файлы и история просмотра остаются на ПК.', 'Only titles and years are sent to TMDB. Files and history stay on this PC.')}</div>
      {picker && <FolderPicker title={tr('Папка библиотеки', 'Library folder')} onPick={add} onClose={() => setPicker(false)} />}
    </Card>
  );
}

function Playback({ st, save }: { st: S; save: (p: any) => Promise<S> }) {
  const { toast } = useApp();
  const set = (k: string, v: string) => save({ [k]: v }).catch((e) => toast(e.message));
  const enc = st.hw?.encoders || [];
  const encName: Record<string, string> = { nvenc: 'NVENC (NVIDIA)', qsv: 'Quick Sync (Intel)', amf: 'AMF (AMD)', vaapi: 'VAAPI', cpu: tr('Только CPU', 'CPU only') };
  return (
    <Card id="play" title={tr('Воспроизведение и транскодирование', 'Playback & transcoding')}>
      {!st.hw?.ffmpegFound && <div className="err">{tr('FFmpeg не найден. Без него не работает транскодирование и чтение дорожек. В установщике он идёт в комплекте — переустановите или укажите путь.', 'FFmpeg not found. Transcoding needs it.')}</div>}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(240px,1fr))', gap: 12 }}>
        <label className="fld">{tr('Аппаратное ускорение', 'Hardware acceleration')}
          <select className="sel" value={st.hwaccel} onChange={(e) => set('hwaccel', e.target.value)}>
            <option value="auto">{tr('Авто', 'Auto')}{st.encoder ? ` (${encName[st.encoder] || st.encoder})` : ''}</option>
            {enc.map((e) => <option key={e} value={e}>{encName[e] || e} — {tr('найдено', 'found')}</option>)}
            <option value="cpu">{encName.cpu}</option>
          </select>
        </label>
        <label className="fld">{tr('Качество по умолчанию', 'Default quality')}
          <select className="sel" value={st.quality} onChange={(e) => set('quality', e.target.value)}>
            <option value="original">{tr('Оригинал, если возможно', 'Original when possible')}</option>
            <option value="1080p">1080p</option><option value="720p">720p</option><option value="480p">480p</option>
          </select>
        </label>
        <label className="fld">{tr('Параллельных транскодов', 'Concurrent transcodes')}
          <select className="sel" value={st.maxStreams} onChange={(e) => set('maxStreams', e.target.value)}>{['1', '2', '3', '4'].map((n) => <option key={n}>{n}</option>)}</select>
        </label>
        <label className="fld">{tr('Язык субтитров по умолчанию', 'Default subtitle language')}
          <select className="sel" value={st.subLang} onChange={(e) => set('subLang', e.target.value)}>
            <option value="rus">Русский</option><option value="eng">English</option><option value="">{tr('Не включать', 'None')}</option>
          </select>
        </label>
      </div>
      <div className="row" style={{ gap: 20 }}>
        <label className="chk"><input type="checkbox" checked={st.tonemap !== '0'} onChange={(e) => set('tonemap', e.target.checked ? '1' : '0')} />{tr('Тонмаппинг HDR → SDR', 'HDR → SDR tonemapping')}{st.hw && !st.hw.zscale && <span className="sub">({tr('без zscale — упрощённый', 'no zscale — basic')})</span>}</label>
        <label className="chk"><input type="checkbox" checked={st.autoNext !== '0'} onChange={(e) => set('autoNext', e.target.checked ? '1' : '0')} />{tr('Автоматически включать следующую серию', 'Autoplay next episode')}</label>
        <label className="chk"><input type="checkbox" checked={st.useExternal === '1'} onChange={(e) => set('useExternal', e.target.checked ? '1' : '')} />{tr('Предлагать внешний плеер (VLC / mpv)', 'Offer external player (VLC / mpv)')}</label>
      </div>
      <label className="fld">{tr('Путь к внешнему плееру (если не найден сам)', 'External player path (if not detected)')}
        <input className="inp mono" style={{ fontSize: 'calc(12px * var(--fs))' }} defaultValue={st.externalPlayer} placeholder="C:\Program Files\VideoLAN\VLC\vlc.exe" onBlur={(e) => e.target.value !== st.externalPlayer && set('externalPlayer', e.target.value)} />
      </label>
      <div className="sub">FFmpeg {st.hw?.version || '—'}</div>
    </Card>
  );
}

function TorrentSection() {
  const { toast } = useApp();
  const { data: ts, setData } = useAsync(() => api.get<TorrentSettings>('/api/torrent-settings'), []);
  const { data: ifs } = useAsync(() => api.get<{ name: string; ip: string; up: boolean; vpn: boolean }[]>('/api/interfaces'), []);
  const idx = useAsync(() => api.get<{ indexers: Indexer[] | null; prefs: SearchPrefs }>('/api/indexers'), []);
  const [picker, setPicker] = useState<null | 'downloadDir' | 'moviesDir' | 'showsDir'>(null);
  const [draft, setDraft] = useState<TorrentSettings | null>(null);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => { if (ts && !draft) setDraft(ts); }, [ts, draft]);
  if (!draft) return <Card id="torrents" title={tr('Торренты и VPN', 'Torrents & VPN')}><span className="spin" /></Card>;
  const set = <K extends keyof TorrentSettings>(k: K, v: TorrentSettings[K]) => setDraft({ ...draft, [k]: v });
  const dirty = JSON.stringify(draft) !== JSON.stringify(ts);
  const apply = async () => {
    setSaving(true); setErr(null);
    try { const n = await api.put<TorrentSettings>('/api/torrent-settings', draft); setData(n); setDraft(n); toast(tr('Настройки торрентов сохранены', 'Torrent settings saved')); } catch (e: any) { setErr(e.message); } finally { setSaving(false); }
  };
  const num = (k: keyof TorrentSettings, label: string, unit = '', min = 0) => (
    <label className="fld">{label}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <input className="inp mono" type="number" min={min} value={String(draft[k])} onChange={(e) => set(k, Number(e.target.value) as never)} style={{ width: '100%' }} />
        {unit && <span className="sub" style={{ whiteSpace: 'nowrap' }}>{unit}</span>}
      </div>
    </label>
  );
  const dir = (k: 'downloadDir' | 'moviesDir' | 'showsDir', label: string) => (
    <label className="fld">{label}
      <div style={{ display: 'flex', gap: 6 }}>
        <input className="inp mono" style={{ flex: 1, fontSize: 'calc(12px * var(--fs))' }} value={draft[k]} onChange={(e) => set(k, e.target.value)} />
        <button type="button" className="btn btn-l sm" style={{ padding: '0 12px' }} aria-label={tr('Выбрать папку', 'Choose folder')} onClick={() => setPicker(k)}><Icon name="folder" size={16} /></button>
      </div>
    </label>
  );
  const vpnIfs = (ifs || []).filter((i) => i.vpn);
  return (
    <Card id="torrents" title={tr('Торренты и VPN', 'Torrents & VPN')} note={tr('Встроенный движок. Изменения сети перезапускают клиент.', 'Built-in engine. Network changes restart the client.')}
      action={<button className="btn btn-r sm" disabled={!dirty || saving} onClick={apply}>{saving ? <span className="spin" /> : tr('Сохранить', 'Save')}</button>}>
      {err && <div className="err">{err}</div>}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(240px,1fr))', gap: 12 }}>
        {dir('downloadDir', tr('Папка загрузок', 'Download folder'))}
        {dir('moviesDir', tr('Куда переносить фильмы', 'Movies destination'))}
        {dir('showsDir', tr('Куда переносить сериалы', 'Shows destination'))}
      </div>
      <label className="chk"><input type="checkbox" checked={draft.autoImport} onChange={(e) => set('autoImport', e.target.checked)} />{tr('Переносить готовые загрузки в библиотеку (Название (Год) / Сезон N)', 'Move finished downloads into the library')}</label>

      <h3 className="h2" style={{ fontSize: 'calc(14px * var(--fs))', marginTop: 4 }}>{tr('Сеть и VPN', 'Network & VPN')}</h3>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(240px,1fr))', gap: 12 }}>
        <label className="fld">{tr('Интерфейс для торрентов', 'Torrent interface')}
          <select className="sel" value={draft.bindIface} onChange={(e) => set('bindIface', e.target.value)}>
            <option value="">{tr('Любой', 'Any')}</option>
            {(ifs || []).map((i) => <option key={i.name} value={i.name}>{i.name}{i.ip ? ` · ${i.ip}` : ''}{i.vpn ? ' (VPN)' : ''}{i.up ? '' : ` · ${tr('отключён', 'down')}`}</option>)}
          </select>
        </label>
        {num('port', tr('Входящий порт', 'Listening port'), '', 1024)}
        <label className="fld">{tr('Шифрование', 'Encryption')}
          <select className="sel" value={draft.encryption} onChange={(e) => set('encryption', e.target.value)}>
            <option value="prefer">{tr('Предпочитать', 'Prefer')}</option><option value="require">{tr('Требовать', 'Require')}</option><option value="off">{tr('Выключено', 'Off')}</option>
          </select>
        </label>
        <label className="fld">SOCKS5-{tr('прокси', 'proxy')}
          <input className="inp mono" style={{ fontSize: 'calc(12px * var(--fs))' }} value={draft.proxy} onChange={(e) => set('proxy', e.target.value)} placeholder="socks5://user:pass@host:1080" />
        </label>
      </div>
      <label className="chk"><input type="checkbox" checked={draft.killSwitch} disabled={!draft.bindIface} onChange={(e) => set('killSwitch', e.target.checked)} />
        Kill switch — {tr('останавливать загрузки, если интерфейс VPN пропал', 'stop transfers when the VPN interface disappears')}</label>
      {!draft.bindIface && vpnIfs.length > 0 && <div className="sub">{tr('Найден VPN-интерфейс', 'VPN interface found')}: {vpnIfs.map((i) => i.name).join(', ')}. {tr('Выберите его, чтобы включить kill switch.', 'Pick it to enable the kill switch.')}</div>}
      <div className="row" style={{ gap: 20 }}>
        <label className="chk"><input type="checkbox" checked={draft.dht} onChange={(e) => set('dht', e.target.checked)} />DHT</label>
        <label className="chk"><input type="checkbox" checked={draft.pex} onChange={(e) => set('pex', e.target.checked)} />PEX</label>
        <label className="chk"><input type="checkbox" checked={draft.upnp} onChange={(e) => set('upnp', e.target.checked)} />UPnP</label>
      </div>
      <label className="fld">{tr('Дополнительные трекеры (добавляются ко всем раздачам, по одному в строке)', 'Extra trackers (added to every torrent, one per line)')}
        <textarea className="inp mono" style={{ fontSize: 'calc(12px * var(--fs))', minHeight: 96, height: 'auto', padding: 10, resize: 'vertical' }} value={draft.trackers} onChange={(e) => set('trackers', e.target.value)} />
      </label>

      <h3 className="h2" style={{ fontSize: 'calc(14px * var(--fs))', marginTop: 4 }}>{tr('Скорость и раздача', 'Speed & seeding')}</h3>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(180px,1fr))', gap: 12 }}>
        {num('downKBs', tr('Лимит загрузки', 'Download limit'), tr('КБ/с, 0 — нет', 'KB/s, 0 = none'))}
        {num('upKBs', tr('Лимит отдачи', 'Upload limit'), tr('КБ/с, 0 — нет', 'KB/s, 0 = none'))}
        {num('maxActive', tr('Активных загрузок', 'Active downloads'), '', 1)}
        {num('seedRatio', tr('Раздавать до рейтинга', 'Seed until ratio'))}
        {num('seedMinutes', tr('Или до', 'Or for'), tr('мин', 'min'))}
        {num('minFreeGB', tr('Оставлять свободным', 'Keep free'), tr('ГБ', 'GB'))}
        {num('bufferMB', tr('Буфер перед стартом', 'Start buffer'), tr('МБ', 'MB'), 8)}
        <label className="fld">{tr('Расписание загрузок', 'Download schedule')}
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <select className="sel" value={draft.scheduleFrom} onChange={(e) => set('scheduleFrom', Number(e.target.value))} aria-label={tr('С', 'From')}>
              <option value={-1}>{tr('Всегда', 'Always')}</option>{Array.from({ length: 24 }, (_, h) => <option key={h} value={h}>{String(h).padStart(2, '0')}:00</option>)}
            </select>
            {draft.scheduleFrom >= 0 && <>—<select className="sel" value={draft.scheduleTo} onChange={(e) => set('scheduleTo', Number(e.target.value))} aria-label={tr('До', 'To')}>{Array.from({ length: 24 }, (_, h) => <option key={h} value={h}>{String(h).padStart(2, '0')}:00</option>)}</select></>}
          </div>
        </label>
      </div>

      <Indexers data={idx.data} reload={idx.reload} />
      {picker && <FolderPicker title={tr('Выберите папку', 'Choose folder')} initial={draft[picker]} onPick={(p) => { set(picker, p); setPicker(null); }} onClose={() => setPicker(null)} />}
    </Card>
  );
}

function Indexers({ data, reload }: { data: { indexers: Indexer[] | null; prefs: SearchPrefs } | null; reload: () => void }) {
  const { toast } = useApp();
  const [list, setList] = useState<Indexer[]>([]);
  const [prefs, setPrefs] = useState<SearchPrefs | null>(null);
  useEffect(() => { if (data) { setList(data.indexers || []); setPrefs(data.prefs); } }, [data]);
  const save = async () => {
    try { await api.put('/api/indexers', { indexers: list, prefs }); toast(tr('Источники сохранены', 'Indexers saved')); reload(); } catch (e: any) { toast(e.message); }
  };
  const upd = (i: number, p: Partial<Indexer>) => setList(list.map((x, j) => (j === i ? { ...x, ...p } : x)));
  return (
    <>
      <h3 className="h2" style={{ fontSize: 'calc(14px * var(--fs))', marginTop: 4 }}>{tr('Источники поиска', 'Search sources')}</h3>
      <div className="sub">{tr('JacRed — публичный парсер русских трекеров, как в Lampa: работает сразу, без Jackett и аккаунтов. Свои Jackett или Prowlarr добавляются как Torznab.', 'JacRed is a public parser of Russian trackers, as in Lampa: works right away, no Jackett or accounts. Your own Jackett or Prowlarr are added as Torznab.')}</div>
      {list.map((x, i) => (
        <div key={i} className="row" style={{ padding: 10, borderRadius: 12, background: 'var(--bg2)' }}>
          <input type="checkbox" checked={x.on} onChange={(e) => upd(i, { on: e.target.checked })} aria-label={tr('Включён', 'Enabled')} style={{ width: 18, height: 18, accentColor: 'var(--acc)' }} />
          <input className="inp" style={{ flex: '0 1 140px' }} value={x.name} onChange={(e) => upd(i, { name: e.target.value })} placeholder={tr('Название', 'Name')} aria-label={tr('Название', 'Name')} />
          <input className="inp mono" style={{ flex: '1 1 260px', fontSize: 'calc(12px * var(--fs))' }} value={x.url} onChange={(e) => upd(i, { url: e.target.value })} placeholder="http://127.0.0.1:9117/api/v2.0/indexers/all/results/torznab/" aria-label="URL" />
          <span className="tag" style={{ flex: 'none' }}>{x.kind === 'jacred' ? 'JacRed' : 'Torznab'}</span>
          <input className="inp mono" style={{ flex: '0 1 160px', fontSize: 'calc(12px * var(--fs))' }} value={x.apiKey} onChange={(e) => upd(i, { apiKey: e.target.value })} placeholder={x.kind === 'jacred' ? tr('ключ, если нужен', 'key, if needed') : 'API key'} aria-label="API key" />
          <button className="ri" style={{ width: 36, height: 36 }} aria-label={tr('Удалить', 'Remove')} onClick={() => setList(list.filter((_, j) => j !== i))}><Icon name="trash" size={16} /></button>
        </div>
      ))}
      {prefs && (
        <div className="row">
          <label className="fld">{tr('Качество', 'Quality')}
            <select className="sel" value={prefs.quality} onChange={(e) => setPrefs({ ...prefs, quality: e.target.value })}><option value="2160p">4K</option><option value="1080p">1080p</option><option value="720p">720p</option><option value="">{tr('Любое', 'Any')}</option></select>
          </label>
          <label className="fld">{tr('Язык озвучки', 'Audio language')}
            <select className="sel" value={prefs.language} onChange={(e) => setPrefs({ ...prefs, language: e.target.value })}><option value="rus">Русский</option><option value="eng">English</option><option value="">{tr('Любой', 'Any')}</option></select>
          </label>
          <label className="fld">{tr('Макс. размер, ГБ', 'Max size, GB')}<input className="inp mono" type="number" min={0} value={prefs.maxSizeGB} onChange={(e) => setPrefs({ ...prefs, maxSizeGB: Number(e.target.value) })} style={{ width: 100 }} /></label>
          <label className="fld">{tr('Мин. сидов', 'Min seeds')}<input className="inp mono" type="number" min={0} value={prefs.minSeeds} onChange={(e) => setPrefs({ ...prefs, minSeeds: Number(e.target.value) })} style={{ width: 100 }} /></label>
        </div>
      )}
      <div className="row">
        <button className="btn btn-l sm" onClick={() => setList([...list, { name: 'Jackett', url: '', apiKey: '', on: true }])}><Icon name="plus" size={16} />{tr('Добавить Torznab', 'Add Torznab')}</button>
        {!list.some((x) => x.kind === 'jacred') && <button className="btn btn-l sm" onClick={() => setList([...list, { name: 'JacRed', url: 'https://jac.red', apiKey: '', on: true, kind: 'jacred' }])}><Icon name="plus" size={16} />{tr('Добавить JacRed', 'Add JacRed')}</button>}
        <button className="btn btn-d sm" onClick={save}>{tr('Сохранить источники', 'Save sources')}</button>
      </div>
    </>
  );
}

function Access({ st, save, reload }: { st: S; save: (p: any) => Promise<S>; reload: () => void }) {
  const { state, reload: reloadApp, toast } = useApp();
  const [pw, setPw] = useState('');
  const setUi = async (l: string) => { await save({ uiLang: l }); setLang(l); await reloadApp(); reload(); };
  const setPassword = async (v: string) => {
    try { await save({ password: v }); setPw(''); toast(v ? tr('Пароль установлен', 'Password set') : tr('Пароль снят', 'Password removed')); } catch (e: any) { toast(e.message); }
  };
  return (
    <Card id="access" title={tr('Доступ и язык', 'Access & language')}>
      <div className="row">
        <label className="fld">{tr('Язык интерфейса', 'Interface language')}
          <select className="sel" value={st.uiLang} onChange={(e) => setUi(e.target.value)}><option value="ru">Русский</option><option value="en">English</option></select>
        </label>
      </div>
      <div className="sub">{tr('Сервер отвечает только устройствам локальной сети. Открыть с телефона или ТВ-браузера:', 'The server only answers devices on your LAN. Open from a phone:')} {state?.lanUrls?.map((u) => <span key={u} className="mono" style={{ marginRight: 8 }}>{u}</span>)}</div>
      <form className="row" onSubmit={(e) => { e.preventDefault(); if (pw) setPassword(pw); }} style={{ alignItems: 'flex-end' }}>
        <label className="fld" style={{ flex: '1 1 240px' }}>{tr('Пароль для входа с других устройств', 'Password for other devices')} {st.passwordSet && <span style={{ color: 'var(--ok-fg)' }}>· {tr('установлен', 'set')}</span>}
          <input className="inp" type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" placeholder={tr('Не задан — вход свободный внутри сети', 'Not set — open within the LAN')} />
        </label>
        <button className="btn btn-d sm" disabled={!pw}>{tr('Установить', 'Set')}</button>
        {st.passwordSet && <button type="button" className="btn btn-l sm" onClick={() => setPassword('')}>{tr('Снять пароль', 'Remove')}</button>}
      </form>
      <div className="sub">{tr('На этом ПК пароль не спрашивается.', 'This PC never asks for the password.')}</div>
    </Card>
  );
}

function Backup() {
  const { toast } = useApp();
  const input = useRef<HTMLInputElement>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const restore = async (f: File) => {
    const fd = new FormData(); fd.append('file', f);
    try { await api.post('/api/restore', fd); setMsg(tr('Копия принята. Перезапустите HomeCinema, чтобы применить её.', 'Backup accepted. Restart HomeCinema to apply it.')); } catch (e: any) { toast(e.message); }
  };
  return (
    <>
      <div className="row">
        <a className="btn btn-l" href="/api/backup" download>{tr('Создать резервную копию БД', 'Back up database')}</a>
        <button className="btn btn-l" onClick={() => input.current?.click()}>{tr('Восстановить', 'Restore')}</button>
        <a className="btn btn-l" href="/api/export/watched.csv" download>{tr('Экспорт просмотренного', 'Export watched')}</a>
        <input ref={input} type="file" accept=".db,application/octet-stream" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) restore(f); e.target.value = ''; }} />
      </div>
      {msg && <div className="ok">{msg}</div>}
    </>
  );
}

function Diagnostics() {
  const { data, reload, loading } = useAsync(() => api.get<{ checks: Check[] }>('/api/diagnostics').then((r) => r.checks), []);
  const [logs, setLogs] = useState<{ lines: string[]; path: string } | null>(null);
  return (
    <Card id="diag" title={tr('Диагностика', 'Diagnostics')}
      action={<>
        <button className="btn btn-l sm" onClick={reload} disabled={loading}>{loading ? <span className="spin" /> : tr('Проверить снова', 'Check again')}</button>
        <button className="btn btn-l sm" onClick={() => api.get<{ lines: string[]; path: string }>('/api/logs').then(setLogs)}>{tr('Открыть логи', 'Open logs')}</button>
      </>}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(260px,1fr))', gap: 8 }}>
        {(data || []).map((c) => {
          const [bg, fg, mark] = c.ok ? ['var(--ok-bg)', 'var(--ok-fg)', '✓'] : c.warn ? ['var(--warn-bg)', 'var(--warn-fg)', '!'] : ['var(--err-bg)', 'var(--err-fg)', '×'];
          return (
            <div key={c.id} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: 12, borderRadius: 12, background: 'var(--bg2)', fontSize: 'calc(13px * var(--fs))' }}>
              <span aria-hidden="true" style={{ width: 24, height: 24, borderRadius: '50%', background: bg, color: fg, display: 'grid', placeItems: 'center', fontWeight: 800, fontSize: 'calc(12px * var(--fs))', flex: 'none' }}>{mark}</span>
              <span style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}><span style={{ fontWeight: 600 }}>{c.name}</span><span style={{ fontSize: 'calc(11px * var(--fs))', color: 'var(--mut)', wordBreak: 'break-word' }}>{c.detail}</span></span>
            </div>
          );
        })}
      </div>
      <div className="sub">{tr('Не находится ТВ? Проверьте, что ПК и ТВ в одной подсети и что брандмауэр Windows разрешил HomeCinema в частной сети.', 'TV not found? Check both are on the same subnet and Windows Firewall allows HomeCinema on private networks.')} <Link to="/devices">{tr('Устройства', 'Devices')}</Link></div>
      {logs && (
        <Modal title={tr('Журнал', 'Log')} onClose={() => setLogs(null)} wide>
          <div className="sub mono">{logs.path}</div>
          <pre className="mono" style={{ margin: 0, fontSize: 'calc(11px * var(--fs))', maxHeight: 420, overflow: 'auto', background: 'var(--bg2)', padding: 12, borderRadius: 10, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{logs.lines.slice(-300).join('\n') || tr('Пусто', 'Empty')}</pre>
        </Modal>
      )}
    </Card>
  );
}
