// Typed client for the HomeCinema server REST API.

export type Track = { index: number; codec: string; lang: string; title: string; channels?: number; default?: boolean; external?: string };
export type FileInfo = {
  id: number; itemId: number; name: string; size: number; container: string; duration: number; videoCodec: string;
  width: number; height: number; bitDepth: number; hdr: string; audio: Track[] | null; subs: Track[] | null;
};
export type CastMember = { name: string; role: string; photo: string };
export type Progress = { position: number; duration: number; watched: boolean; device: string; updatedAt: number };
export type Item = {
  id: number; kind: 'movie' | 'episode'; title: string; origTitle?: string; year: number; showId?: number; showTitle?: string;
  season?: number; episode?: number; epTitle?: string; tmdbId: number; overview: string; poster: string; backdrop: string;
  genres: string[]; rating: number; runtime: number; cert: string; cast?: CastMember[]; matched: number; addedAt: number;
  source: string; quality: string; progress?: Progress; favorite: boolean; later: boolean; files?: FileInfo[];
};
export type Title = {
  kind: 'movie' | 'show'; id: number; title: string; year: number; poster: string; backdrop: string; genres: string[];
  rating: number; quality: string; addedAt: number; watched: boolean; progress: number; favorite: boolean; later: boolean;
  episodes?: number; overview: string;
};
export type Show = {
  id: number; title: string; year: number; tmdbId: number; overview: string; poster: string; backdrop: string; genres: string[];
  rating: number; cast?: CastMember[]; matched: number; favorite: boolean; later: boolean; episodes: number; unwatched: number;
  seasons?: { number: number; episodes: Item[] }[];
};
export type Profile = { id: number; name: string; hasPin: boolean; color: string; prefs: Prefs; isKid: boolean };
export type Prefs = { theme?: 'light' | 'dark' | 'oled'; accent?: 'red' | 'amber' | 'blue' | 'green'; bg?: 'live' | 'static' | 'off'; intensity?: number; posterTint?: boolean };
export type ScanStatus = { scanning: boolean; library: string; done: number; total: number; added: number; movies: number; shows: number; matching: number; error?: string };
export type AppState = {
  version: string; setupDone: boolean; authRequired: boolean; lanUrls: string[]; uiLang: string; local: boolean;
  profiles?: Profile[]; profileId?: number;
};
export type PlayTrack = { index: number; label: string; codec: string; lang: string; selected: boolean; url?: string; bitmap?: boolean; external?: boolean };
export type Playback = {
  mode: 'direct' | 'hls'; url: string; offset: number; start: number; duration: number; session?: string; reason?: string;
  summary: string; encoder?: string; audio: PlayTrack[]; subs: PlayTrack[]; fileId: number; itemId: number; title: string; info: string; external: boolean;
};
export type Compat = { name: string; mode: string; status: 'direct' | 'transcode' };
export type TorrentFile = { index: number; path: string; size: number; progress: number; wanted: boolean; video: boolean; blocked: boolean };
export type TorrentInfo = {
  hash: string; name: string; size: number; done: number; progress: number; status: string; mode: string; keep: boolean;
  down: number; up: number; peers: number; seeds: number; ratio: number; eta: number; priority: number; addedAt: number; doneAt: number;
  files?: TorrentFile[]; imported?: string[]; error?: string; fileCount: number; known: number; hint?: string;
};
export type TorrentOverview = {
  running: boolean; error?: string; vpnDown: boolean; diskLow: boolean; freeGB: number; down: number; up: number; iface: string;
  killSwitch: boolean; port: number; items: TorrentInfo[]; dhtNodes: number;
};
export type StreamStatus = {
  hash: string; name: string; fileIndex: number; fileName: string; size: number; buffered: number; target: number; ready: boolean;
  position: number; down: number; seeds: number; peers: number; known: number; dhtNodes: number; hint?: string; pieces: number[]; numPieces: number; pieceSize: number; progress: number;
};
export type TorrentSettings = {
  downloadDir: string; moviesDir: string; showsDir: string; port: number; downKBs: number; upKBs: number; seedRatio: number;
  seedMinutes: number; bindIface: string; killSwitch: boolean; encryption: string; dht: boolean; pex: boolean; upnp: boolean;
  proxy: string; bufferMB: number; bufferPct: number; minFreeGB: number; maxActive: number; scheduleFrom: number; scheduleTo: number;
  autoImport: boolean; acceptedRisk: boolean; trackers: string;
};
export type SearchResult = { indexer: string; title: string; link: string; magnet?: string; size: number; seeds: number; peers: number; quality: string; lang: string; date: string; score: number; best: boolean };
export type CatalogItem = {
  kind: 'movie' | 'tv'; id: number; title: string; origTitle: string; year: number; poster: string; backdrop: string;
  overview: string; rating: number; votes: number; genreIds: number[];
};
export type CatalogPage = { items: CatalogItem[]; page: number; pages: number };
export type CatalogTitle = CatalogItem & {
  genres: string[]; runtime: number; tagline: string; cert: string; imdbId: string; countries: string[] | null;
  seasons?: { number: number; name: string; episodes: number; year: number }[];
  cast: CastMember[]; related: CatalogItem[];
};
export type Indexer = { name: string; url: string; apiKey: string; on: boolean };
export type SearchPrefs = { quality: string; language: string; maxSizeGB: number; minSeeds: number };
export type Caps = { video: string[]; audio: string[]; container: string[]; maxHeight: number; hdr: boolean };
export type Device = { id: string; name: string; model: string; proto: string; addr: string; port: number; manual: boolean; online: boolean; busy: boolean; caps: Caps };
export type CastStatus = {
  deviceId: string; deviceName: string; state: string; position: number; duration: number; volume: number; muted: boolean;
  itemId: number; title: string; transcode: string; audioLabel: string; subLabel: string; error?: string;
  fileId: number; torrent?: string; file: number; encoder?: string; audio: number; sub: number;
};
export type Check = { id: string; name: string; ok: boolean; warn: boolean; detail: string };
export type Library = { id: number; path: string; kind: string; online: boolean };
export type Settings = {
  metaLang: string; uiLang: string; hwaccel: string; quality: string; maxStreams: string; tonemap: string; externalPlayer: string;
  useExternal: string; setupDone: string; subLang: string; autoNext: string; tmdbKeySet: boolean; tmdbKeyTail?: string; tmdbApi: string; tmdbImg: string; tmdbVia: 'cub' | 'direct' | 'custom'; passwordSet: boolean;
  hw: { ffmpegFound: boolean; version: string; encoders: string[] | null; zscale: boolean }; encoder: string;
};

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin', headers: {} };
  if (body instanceof FormData) init.body = body;
  else if (body !== undefined) {
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)['Content-Type'] = 'application/json';
  }
  const res = await fetch(path, init);
  const text = await res.text();
  let data: any = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!res.ok) {
    if (res.status === 401 && path !== '/api/auth/login') window.dispatchEvent(new CustomEvent('hc:unauthorized'));
    throw new ApiError(res.status, (data && data.error) || res.statusText);
  }
  return data as T;
}

export const api = {
  get: <T,>(p: string) => req<T>('GET', p),
  post: <T,>(p: string, b?: unknown) => req<T>('POST', p, b ?? {}),
  put: <T,>(p: string, b?: unknown) => req<T>('PUT', p, b ?? {}),
  patch: <T,>(p: string, b?: unknown) => req<T>('PATCH', p, b ?? {}),
  del: <T,>(p: string) => req<T>('DELETE', p),
};

export function qs(params: Record<string, string | number | boolean | undefined | null>) {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') u.set(k, String(v));
  const s = u.toString();
  return s ? '?' + s : '';
}

// Codec support of this browser / WebView, sent with play requests.
let capsCache: string | null = null;
export function clientCaps(): string {
  if (capsCache !== null) return capsCache;
  const ms = (window as any).MediaSource as typeof MediaSource | undefined;
  const can = (t: string) => !!ms && ms.isTypeSupported(t);
  const p: string[] = [];
  if (can('video/mp4; codecs="hvc1.2.4.L153.B0"') || can('video/mp4; codecs="hev1.1.6.L93.B0"')) p.push('hevc=1');
  if (can('audio/mp4; codecs="ac-3"') || can('audio/mp4; codecs="ec-3"')) p.push('ac3=1');
  if (window.matchMedia && window.matchMedia('(dynamic-range: high)').matches) p.push('hdr=1');
  // Chromium (Edge WebView2, Chrome) opens Matroska through <video src>, so
  // MKV files play as they are; the player falls back to a remux if not.
  const v = document.createElement('video');
  const brands: { brand: string }[] = (navigator as any).userAgentData?.brands || [];
  if (v.canPlayType('video/x-matroska') || v.canPlayType('video/mkv') || brands.some((b) => /Chromium/.test(b.brand))) p.push('mkv=1');
  capsCache = p.join('&');
  return capsCache;
}

export function fmtTime(sec: number): string {
  if (!isFinite(sec) || sec < 0) sec = 0;
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = Math.floor(sec % 60);
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}` : `${m}:${String(s).padStart(2, '0')}`;
}

export function fmtLeft(sec: number, t: (k: string) => string): string {
  const m = Math.max(1, Math.round(sec / 60));
  if (m < 60) return `${t('left')} ${m} ${t('min')}`;
  return `${t('left')} ${Math.floor(m / 60)} ${t('h')} ${m % 60} ${t('min')}`;
}

export function fmtSize(b: number): string {
  if (b >= 1 << 30) return (b / (1 << 30)).toFixed(1).replace('.', ',') + ' ГБ';
  if (b >= 1 << 20) return Math.round(b / (1 << 20)) + ' МБ';
  if (b >= 1 << 10) return Math.round(b / (1 << 10)) + ' КБ';
  return b + ' Б';
}

export function fmtSpeed(bps: number): string {
  if (bps < 1024) return '—';
  if (bps >= 1 << 20) return (bps / (1 << 20)).toFixed(1) + ' МБ/с';
  return Math.round(bps / 1024) + ' КБ/с';
}

export function fmtRuntime(min: number): string {
  if (!min) return '';
  const h = Math.floor(min / 60), m = min % 60;
  return h ? `${h} ч ${m} мин` : `${m} мин`;
}
