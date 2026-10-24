// App-wide state: session, profile preferences (theme, accent, background),
// live server events and small UI helpers (toast, "send to TV" dialog).
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { api, type AppState, type CastStatus, type Device, type Prefs, type Profile, type ScanStatus, type TorrentOverview } from './api';
import { setLang } from './i18n';

export type CastTarget = { fileId?: number; torrent?: string; file?: number; start?: number; title?: string; audio?: number; sub?: number };

type Ctx = {
  state: AppState | null;
  reload: () => Promise<void>;
  profile: Profile | null;
  prefs: Required<Prefs>;
  setPrefs: (p: Prefs) => void;
  scan: ScanStatus | null;
  torrents: TorrentOverview | null;
  devices: Device[];
  cast: CastStatus[];
  lastProgress: { itemId: number; position: number; device: string } | null;
  toast: (msg: string) => void;
  castTarget: CastTarget | null;
  openCast: (t: CastTarget | null) => void;
};

const AppCtx = createContext<Ctx>(null as unknown as Ctx);
export const useApp = () => useContext(AppCtx);

const defaults: Required<Prefs> = { theme: 'light', accent: 'iris', bg: 'live', intensity: 100, posterTint: true, trailers: true };

function loadLocalPrefs(): Prefs {
  try { return JSON.parse(localStorage.getItem('hc.prefs') || '{}'); } catch { return {}; }
}

export function AppProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AppState | null>(null);
  const [localPrefs, setLocalPrefs] = useState<Prefs>(loadLocalPrefs);
  const [scan, setScan] = useState<ScanStatus | null>(null);
  const [torrents, setTorrents] = useState<TorrentOverview | null>(null);
  const [devices, setDevices] = useState<Device[]>([]);
  const [cast, setCast] = useState<CastStatus[]>([]);
  const [lastProgress, setLastProgress] = useState<Ctx['lastProgress']>(null);
  const [toastMsg, setToastMsg] = useState<string | null>(null);
  const [castTarget, setCastTarget] = useState<CastTarget | null>(null);
  const toastTimer = useRef<number>();
  const saveTimer = useRef<number>();

  const reload = useCallback(async () => {
    const s = await api.get<AppState>('/api/state');
    setLang(s.uiLang);
    setState(s);
  }, []);

  useEffect(() => {
    reload().catch(() => setState(null));
    const onUnauth = () => reload().catch(() => {});
    window.addEventListener('hc:unauthorized', onUnauth);
    return () => window.removeEventListener('hc:unauthorized', onUnauth);
  }, [reload]);

  // Live updates over SSE.
  useEffect(() => {
    if (!state || state.authRequired) return;
    const es = new EventSource('/api/events');
    es.onmessage = (ev) => {
      try {
        const m = JSON.parse(ev.data);
        switch (m.type) {
          case 'scan': setScan(m.data); break;
          case 'torrents': setTorrents(m.data); break;
          case 'devices': setDevices(m.data); break;
          case 'cast': setCast(m.data || []); break;
          case 'progress': setLastProgress(m.data); break;
          case 'letterboxd': window.dispatchEvent(new CustomEvent('hc:letterboxd', { detail: m.data })); break;
        }
      } catch { /* ignore malformed event */ }
    };
    api.get<Device[]>('/api/devices').then(setDevices).catch(() => {});
    return () => es.close();
  }, [state?.authRequired, state?.profileId]);

  const profile = useMemo(() => state?.profiles?.find((p) => p.id === state.profileId) ?? null, [state]);
  const prefs = useMemo<Required<Prefs>>(() => ({ ...defaults, ...(profile?.prefs || {}), ...localPrefs }), [profile, localPrefs]);

  const setPrefs = useCallback((p: Prefs) => {
    setLocalPrefs((cur) => {
      const next = { ...cur, ...p };
      try { localStorage.setItem('hc.prefs', JSON.stringify(next)); } catch { /* private mode */ }
      return next;
    });
    // Persist into the profile so the theme follows the person between devices.
    if (profile) {
      window.clearTimeout(saveTimer.current);
      saveTimer.current = window.setTimeout(() => {
        const merged = { ...(profile.prefs || {}), ...loadLocalPrefs(), ...p };
        api.patch(`/api/profiles/${profile.id}`, { prefs: merged }).catch(() => {});
      }, 600);
    }
  }, [profile]);

  const toast = useCallback((msg: string) => {
    setToastMsg(msg);
    window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => setToastMsg(null), 3500);
  }, []);

  const value: Ctx = { state, reload, profile, prefs, setPrefs, scan, torrents, devices, cast, lastProgress, toast, castTarget, openCast: setCastTarget };
  return (
    <AppCtx.Provider value={value}>
      {children}
      {toastMsg && <div className="toast" role="status">{toastMsg}</div>}
    </AppCtx.Provider>
  );
}

// useAsync loads data and reloads when deps change.
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const run = useCallback(() => {
    setLoading(true);
    return fn().then((d) => { setData(d); setError(null); }).catch((e) => setError(e.message || String(e))).finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  useEffect(() => { run(); }, [run]);
  return { data, error, loading, reload: run, setData };
}
