import { useEffect, useRef, useState } from 'react';
import { api, type CatalogTitle } from '../api';
import { useApp } from '../state';
import { tr } from '../i18n';
import { Icon } from './Icon';

// Trailers come from YouTube (TMDB lists them). The player is muted, has no
// controls and stays invisible until it really plays, so a slow or blocked
// YouTube simply leaves the still in place.

export type TrailerRef = { kind: 'movie' | 'tv'; id: number };

const keys = new Map<string, Promise<string>>();

// trailerKey finds the YouTube key for a TMDB title, once per session.
export function trailerKey(r: TrailerRef): Promise<string> {
  const k = r.kind + r.id;
  let p = keys.get(k);
  if (!p) {
    p = api.get<{ title: CatalogTitle }>(`/api/catalog/title/${r.kind}/${r.id}`).then((d) => d.title.trailer || '').catch(() => '');
    keys.set(k, p);
  }
  return p;
}

export function rememberTrailer(r: TrailerRef, key: string) {
  keys.set(r.kind + r.id, Promise.resolve(key));
}

// useTrailer resolves a trailer after `delay` ms while `active`, when the
// user has trailers on and motion isn't reduced.
export function useTrailer(ref: TrailerRef | undefined | null, active: boolean, delay = 0) {
  const { prefs } = useApp();
  const [key, setKey] = useState('');
  const on = prefs.trailers !== false && !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const id = ref ? ref.kind + ref.id : '';
  useEffect(() => {
    setKey('');
    if (!ref || !ref.id || !active || !on) return;
    let gone = false;
    const t = window.setTimeout(() => { trailerKey(ref).then((k) => { if (!gone) setKey(k); }); }, delay);
    return () => { gone = true; window.clearTimeout(t); };
  }, [id, active, on]); // eslint-disable-line react-hooks/exhaustive-deps
  return key;
}

type YTProps = { ytKey: string; muted: boolean; onPlaying?: () => void; onEnd?: () => void; start?: number };

// YouTube fills its box like object-fit: cover and reports when it starts
// playing and when it ends.
export function YouTubeCover({ ytKey, muted, onPlaying, onEnd, start = 0 }: YTProps) {
  const frame = useRef<HTMLIFrameElement>(null);
  const [playing, setPlaying] = useState(false);
  const cb = useRef({ onPlaying, onEnd });
  cb.current = { onPlaying, onEnd };

  const send = (func: string, args: unknown[] = []) =>
    frame.current?.contentWindow?.postMessage(JSON.stringify({ event: 'command', func, args }), '*');

  useEffect(() => {
    const onMsg = (e: MessageEvent) => {
      if (e.source !== frame.current?.contentWindow || typeof e.data !== 'string') return;
      let m: { event?: string; info?: unknown };
      try { m = JSON.parse(e.data); } catch { return; }
      const state = m.event === 'onStateChange' ? m.info : m.event === 'infoDelivery' ? (m.info as { playerState?: number } | null)?.playerState : undefined;
      if (state === 1) { setPlaying(true); cb.current.onPlaying?.(); }
      if (state === 0) cb.current.onEnd?.();
    };
    window.addEventListener('message', onMsg);
    return () => window.removeEventListener('message', onMsg);
  }, []);
  useEffect(() => { send(muted ? 'mute' : 'unMute'); if (!muted) send('setVolume', [70]); }, [muted]);

  const src = `https://www.youtube-nocookie.com/embed/${encodeURIComponent(ytKey)}?autoplay=1&mute=1&controls=0&disablekb=1&fs=0&rel=0&iv_load_policy=3&playsinline=1&modestbranding=1&enablejsapi=1&start=${start}&origin=${encodeURIComponent(location.origin)}`;
  return (
    <div className="yt-cover" aria-hidden="true" style={{ opacity: playing ? 1 : 0 }}>
      <iframe ref={frame} src={src} title="trailer" tabIndex={-1} allow="autoplay; encrypted-media; picture-in-picture"
        onLoad={() => {
          // Ask the player to report its state, and repeat it in case the
          // first message lands before the player is ready.
          const hello = () => frame.current?.contentWindow?.postMessage(JSON.stringify({ event: 'listening', id: ytKey }), '*');
          hello(); window.setTimeout(hello, 600); window.setTimeout(hello, 1500);
          if (!muted) window.setTimeout(() => send('unMute'), 700);
        }} />
    </div>
  );
}

// A round sound toggle for a playing trailer.
export function MuteButton({ muted, onToggle, style }: { muted: boolean; onToggle: () => void; style?: React.CSSProperties }) {
  return (
    <button className="yt-mute" onClick={(e) => { e.preventDefault(); e.stopPropagation(); onToggle(); }} aria-pressed={!muted}
      aria-label={muted ? tr('Включить звук трейлера', 'Unmute trailer') : tr('Выключить звук трейлера', 'Mute trailer')} style={style}>
      <Icon name={muted ? 'mute' : 'volume'} size={16} />
    </button>
  );
}

// HeroTrailer plays a title's trailer behind a hero's text after a pause.
export function HeroTrailer({ trailer, delay = 2500 }: { trailer: TrailerRef | null | undefined; delay?: number }) {
  const [muted, setMuted] = useState(true);
  const [playing, setPlaying] = useState(false);
  const [done, setDone] = useState(false);
  const key = useTrailer(trailer, !done, delay);
  useEffect(() => { setPlaying(false); setDone(false); }, [trailer?.kind, trailer?.id]);
  if (!key || done) return null;
  return (
    <>
      <YouTubeCover ytKey={key} muted={muted} onPlaying={() => setPlaying(true)} onEnd={() => setDone(true)} />
      {playing && (
        <div style={{ position: 'absolute', right: 16, top: 16, zIndex: 2, display: 'flex', gap: 6 }}>
          <MuteButton muted={muted} onToggle={() => setMuted(!muted)} />
          <button className="yt-mute" onClick={() => setDone(true)} aria-label={tr('Остановить трейлер', 'Stop trailer')}><Icon name="close" size={14} /></button>
        </div>
      )}
    </>
  );
}
