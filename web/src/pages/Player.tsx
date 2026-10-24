import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import Hls from 'hls.js';
import { api, clientCaps, fmtSize, fmtTime, type Item, type Playback, type StreamStatus } from '../api';
import { useApp } from '../state';
import { tr } from '../i18n';
import { Icon, PauseGlyph, PlayGlyph } from '../components/Icon';

type Cue = { start: number; end: number; text: string };
const speeds = [0.5, 0.75, 1, 1.25, 1.5, 2];

function parseVTT(src: string): Cue[] {
  const toSec = (s: string) => {
    const p = s.trim().replace(',', '.').split(':').map(Number);
    return p.length === 3 ? p[0] * 3600 + p[1] * 60 + p[2] : p[0] * 60 + p[1];
  };
  const out: Cue[] = [];
  for (const block of src.replace(/\r/g, '').split(/\n\n+/)) {
    const lines = block.split('\n');
    const i = lines.findIndex((l) => l.includes('-->'));
    if (i < 0) continue;
    const [a, b] = lines[i].split('-->');
    const text = lines.slice(i + 1).join('\n').replace(/<[^>]+>/g, '').replace(/\{\\[^}]*\}/g, '').trim();
    if (text) out.push({ start: toSec(a), end: toSec(b.trim().split(/\s+/)[0]), text });
  }
  return out;
}

// Full-screen player (FR-7..FR-10): direct play or HLS transcode, own subtitle
// rendering with offset, tracks popover, hotkeys, progress sync, next episode.
export default function Player() {
  const { fileId, hash } = useParams();
  const [params] = useSearchParams();
  const nav = useNavigate();
  const { openCast, state, toast } = useApp();
  const itemId = Number(params.get('item') || 0);
  const torrentFile = Number(params.get('file') ?? -1);
  const video = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const [pb, setPb] = useState<Playback | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [cur, setCur] = useState(0); // movie time
  const [buffered, setBuffered] = useState(0);
  const [paused, setPaused] = useState(true);
  const [waiting, setWaiting] = useState(true);
  const [vol, setVol] = useState(() => Number(localStorage.getItem('hc.vol') ?? 1));
  const [muted, setMuted] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [panel, setPanel] = useState<null | 'subs' | 'audio' | 'speed'>(null);
  const [subIdx, setSubIdx] = useState<number>(() => Number(params.get('sub') ?? -1));
  const [cues, setCues] = useState<Cue[]>([]);
  const [subDelay, setSubDelay] = useState(0);
  const [hover, setHover] = useState<{ x: number; t: number } | null>(null);
  const [idle, setIdle] = useState(false);
  const [next, setNext] = useState<Item | null>(null);
  const [countdown, setCountdown] = useState<number | null>(null);
  const [stream, setStream] = useState<StreamStatus | null>(null);
  const [autoNext, setAutoNext] = useState(true);
  const lastSaved = useRef(0);
  const idleTimer = useRef<number>();
  const startAt = useRef(Number(params.get('start') || 0));
  const opts = useRef({ audio: params.get('audio') ?? '', burn: '', nodirect: false });

  const offset = pb?.mode === 'hls' ? pb.offset : 0;
  const duration = pb?.duration || video.current?.duration || 0;

  // Load (or reload, after an audio switch or a far seek) the playback session.
  const load = useCallback(async (start: number) => {
    setErr(null); setWaiting(true);
    const q = new URLSearchParams(clientCaps());
    if (start > 0) q.set('start', String(Math.floor(start)));
    if (opts.current.audio !== '') q.set('audio', opts.current.audio);
    if (opts.current.burn) { q.set('sub', opts.current.burn); q.set('burn', '1'); }
    if (opts.current.nodirect) q.set('nodirect', '1');
    try {
      const url = hash ? `/api/torrents/${hash}/play?file=${torrentFile}&${q}` : `/api/play/${fileId}?${q}`;
      const p = await api.get<Playback>(url);
      setPb((old) => {
        if (old?.session && old.session !== p.session) api.del(`/api/hls/${old.session}`).catch(() => {});
        return p;
      });
    } catch (e: any) { setErr(e.message); }
  }, [fileId, hash, torrentFile]);

  useEffect(() => { load(startAt.current); }, [load]);

  // Attach the source when the session changes.
  useEffect(() => {
    const v = video.current;
    if (!pb || !v) return;
    hlsRef.current?.destroy(); hlsRef.current = null;
    const seekTo = pb.mode === 'direct' ? pb.start : 0;
    const onMeta = () => { if (seekTo > 0) v.currentTime = seekTo; v.play().catch(() => setPaused(true)); };
    v.addEventListener('loadedmetadata', onMeta, { once: true });
    if (pb.mode === 'hls' && Hls.isSupported()) {
      const h = new Hls({ maxBufferLength: 60, manifestLoadingMaxRetry: 30, manifestLoadingRetryDelay: 1000, levelLoadingMaxRetry: 30 });
      h.on(Hls.Events.ERROR, (_e, d) => {
        if (!d.fatal) return;
        if (d.type === Hls.ErrorTypes.NETWORK_ERROR) h.startLoad();
        else if (d.type === Hls.ErrorTypes.MEDIA_ERROR) h.recoverMediaError();
        else setErr(tr('Ошибка воспроизведения потока', 'Stream playback error'));
      });
      h.loadSource(pb.url);
      h.attachMedia(v);
      hlsRef.current = h;
    } else {
      v.src = pb.url;
    }
    return () => v.removeEventListener('loadedmetadata', onMeta);
  }, [pb]);

  // The file is played as is when the browser should decode it; if it cannot
  // after all (an error, or a picture-less video), ask for a remux instead.
  const directFailed = useCallback(() => {
    if (pb?.mode !== 'direct' || opts.current.nodirect) return false;
    opts.current.nodirect = true;
    load(video.current?.currentTime || pb.start || 0);
    return true;
  }, [pb, load]);
  useEffect(() => {
    const v = video.current;
    if (!v || pb?.mode !== 'direct') return;
    const check = () => { if (v.videoWidth === 0) directFailed(); };
    v.addEventListener('loadedmetadata', check, { once: true });
    return () => v.removeEventListener('loadedmetadata', check);
  }, [pb, directFailed]);

  useEffect(() => () => { hlsRef.current?.destroy(); }, []);
  useEffect(() => () => { if (pb?.session) api.del(`/api/hls/${pb.session}`).catch(() => {}); }, [pb?.session]);

  // Subtitles: fetched in movie time (no server offset), shown by our own renderer.
  useEffect(() => {
    setCues([]);
    const t = pb?.subs.find((s) => s.index === subIdx);
    if (!t?.url || opts.current.burn) return;
    let gone = false;
    fetch(t.url.split('?')[0]).then((r) => (r.ok ? r.text() : Promise.reject(r.statusText)))
      .then((txt) => { if (!gone) setCues(parseVTT(txt)); })
      .catch(() => toast(tr('Не удалось загрузить субтитры', 'Could not load subtitles')));
    return () => { gone = true; };
  }, [pb, subIdx, toast]);

  // Next episode and autoplay preference.
  useEffect(() => {
    if (!itemId) return;
    api.get<{ next: Item | null }>(`/api/items/${itemId}`).then((r) => setNext(r.next)).catch(() => {});
    api.get<{ autoNext: string }>('/api/settings').then((s) => setAutoNext(s.autoNext !== '0')).catch(() => {});
  }, [itemId]);

  // Torrent buffer status while streaming.
  useEffect(() => {
    if (!hash) return;
    let stop = false;
    const tick = () => api.get<StreamStatus>(`/api/torrents/${hash}/stream?file=${torrentFile}&cells=120`).then((s) => !stop && setStream(s)).catch(() => {});
    tick();
    const id = window.setInterval(tick, 2000);
    return () => { stop = true; clearInterval(id); };
  }, [hash, torrentFile]);

  const saveProgress = useCallback((force = false) => {
    if (!itemId || !duration) return;
    const now = Date.now();
    if (!force && now - lastSaved.current < 10000) return;
    lastSaved.current = now;
    fetch(`/api/items/${itemId}/progress`, {
      method: 'POST', keepalive: true, headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin',
      body: JSON.stringify({ position: cur, duration, device: 'pc' }),
    }).catch(() => {});
  }, [itemId, cur, duration]);

  useEffect(() => { if (!paused) saveProgress(); }, [cur, paused, saveProgress]);
  useEffect(() => {
    const onHide = () => saveProgress(true);
    window.addEventListener('pagehide', onHide);
    return () => { window.removeEventListener('pagehide', onHide); };
  }, [saveProgress]);
  const saveRef = useRef(saveProgress); saveRef.current = saveProgress;
  useEffect(() => () => saveRef.current(true), []);

  useEffect(() => { if (video.current) { video.current.volume = vol; video.current.muted = muted; } try { localStorage.setItem('hc.vol', String(vol)); } catch { /* ignore */ } }, [vol, muted]);
  useEffect(() => { if (video.current) video.current.playbackRate = speed; }, [speed, pb]);

  // Seeking: inside the transcoded range locally, otherwise restart transcoding.
  const seek = (t: number) => {
    const v = video.current;
    if (!v || !pb) return;
    t = Math.max(0, Math.min(duration - 1, t));
    if (pb.mode === 'direct') { v.currentTime = t; return; }
    const local = t - pb.offset;
    let end = 0;
    for (let i = 0; i < v.seekable.length; i++) end = Math.max(end, v.seekable.end(i));
    if (local >= 0 && local <= end - 2) v.currentTime = local;
    else { setCur(t); load(t); }
  };
  const toggle = () => { const v = video.current; if (!v) return; if (v.paused) v.play().catch(() => {}); else v.pause(); };
  const fullscreen = () => { if (document.fullscreenElement) document.exitFullscreen(); else root.current?.requestFullscreen().catch(() => {}); };
  const close = () => { saveProgress(true); if (window.history.length > 1) nav(-1); else nav('/'); };

  const pickAudio = (idx: number) => { opts.current.audio = String(idx); setPanel(null); load(cur); };
  const pickSub = (i: number) => {
    const t = pb?.subs.find((s) => s.index === i);
    setPanel(null);
    setSubIdx(i);
    if (t?.bitmap || opts.current.burn) {
      // Picture subtitles (PGS/VobSub) can only be burned in by the server.
      opts.current.burn = t?.bitmap ? String(i) : '';
      load(cur);
    }
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('input,select,textarea')) return;
      switch (e.key) {
        case ' ': case 'k': case 'л': e.preventDefault(); toggle(); break;
        case 'ArrowLeft': e.preventDefault(); seek(cur - 10); break;
        case 'ArrowRight': e.preventDefault(); seek(cur + 10); break;
        case 'ArrowUp': e.preventDefault(); setVol((v) => Math.min(1, v + 0.1)); break;
        case 'ArrowDown': e.preventDefault(); setVol((v) => Math.max(0, v - 0.1)); break;
        case 'f': case 'F': case 'а': case 'А': fullscreen(); break;
        case 'm': case 'ь': setMuted((m) => !m); break;
        case 's': case 'S': case 'ы': case 'Ы': setPanel((p) => (p === 'subs' ? null : 'subs')); break;
        case 'Escape': if (panel) setPanel(null); else if (!document.fullscreenElement) close(); break;
      }
      wake();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  const wake = () => {
    setIdle(false);
    window.clearTimeout(idleTimer.current);
    idleTimer.current = window.setTimeout(() => setIdle(true), 3000);
  };
  useEffect(() => () => window.clearTimeout(idleTimer.current), []);

  // Next episode countdown.
  useEffect(() => {
    if (countdown === null) return;
    if (countdown <= 0) { goNext(); return; }
    const id = window.setTimeout(() => setCountdown((c) => (c === null ? null : c - 1)), 1000);
    return () => clearTimeout(id);
  }, [countdown]); // eslint-disable-line react-hooks/exhaustive-deps
  const goNext = () => {
    setCountdown(null);
    const f = next?.files?.[0];
    if (next && f) nav(`/play/${f.id}?item=${next.id}`, { replace: true });
  };

  const onTime = () => {
    const v = video.current!;
    const t = offset + v.currentTime;
    setCur(t);
    let b = 0;
    for (let i = 0; i < v.buffered.length; i++) if (v.buffered.start(i) <= v.currentTime + 1) b = Math.max(b, v.buffered.end(i));
    setBuffered(offset + b);
    if (next && autoNext && countdown === null && duration > 0 && duration - t < 25 && duration - t > 1 && !paused) setCountdown(10);
  };

  const subNow = cues.filter((c) => cur - subDelay >= c.start && cur - subDelay <= c.end).map((c) => c.text).join('\n');
  const pct = duration ? cur / duration : 0;
  const bpct = duration ? Math.min(1, buffered / duration) : 0;
  const curSub = pb?.subs.find((s) => s.index === subIdx);
  const showUI = !idle || paused || !!panel;
  const castNow = () => {
    video.current?.pause();
    if (hash) openCast({ torrent: hash, file: torrentFile, title: pb?.title });
    else openCast({ fileId: Number(fileId), start: cur, title: pb?.title, audio: opts.current.audio !== '' ? Number(opts.current.audio) : undefined, sub: subIdx >= 0 ? subIdx : undefined });
  };
  const external = async () => {
    try { await api.post(`/api/files/${fileId}/external?start=${Math.floor(cur)}`); video.current?.pause(); toast(tr('Открыто во внешнем плеере', 'Opened in external player')); } catch (e: any) { toast(e.message); }
  };

  const barX = (e: React.MouseEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    const x = Math.max(0, Math.min(r.width, e.clientX - r.left));
    return { x, t: (x / r.width) * duration };
  };

  return (
    <div ref={root} className="player" onMouseMove={wake} style={{ position: 'fixed', inset: 0, background: '#000000', color: '#FFFFFF', zIndex: 40, cursor: showUI ? 'default' : 'none', fontFamily: "'Inter',sans-serif" }}>
      <video ref={video} playsInline onClick={toggle} onDoubleClick={fullscreen}
        onTimeUpdate={onTime} onPlay={() => setPaused(false)} onPause={() => { setPaused(true); saveProgress(true); }}
        onWaiting={() => setWaiting(true)} onPlaying={() => setWaiting(false)} onCanPlay={() => setWaiting(false)}
        onEnded={() => { saveProgress(true); if (next && autoNext) goNext(); }}
        onError={() => { if (!hlsRef.current && !directFailed()) setErr(tr('Браузер не смог открыть файл. Попробуйте внешний плеер или отправьте на ТВ.', 'The browser could not open this file.')); }}
        style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', background: '#000000' }} />

      <header style={{ position: 'absolute', left: 0, right: 0, top: 0, minHeight: 64, display: 'flex', alignItems: 'center', gap: 16, padding: '0 20px', background: 'linear-gradient(#000000E6,#00000000)', opacity: showUI ? 1 : 0, transition: 'opacity .3s' }}>
        <div style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}>
          <span style={{ fontSize: 'calc(14px * var(--fs))', fontWeight: 700, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{pb?.title || tr('Загрузка…', 'Loading…')}</span>
          <span style={{ fontSize: 'calc(12px * var(--fs))', color: '#A9ADB4' }} title={pb?.reason}>{pb ? [pb.info || pb.summary, pb.encoder && pb.mode === 'hls' ? pb.encoder.toUpperCase() : ''].filter(Boolean).join(' · ') : ''}</span>
        </div>
        {itemId > 0 && (
          <span style={{ marginLeft: 'auto', display: 'inline-flex', alignItems: 'center', gap: 8, height: 30, padding: '0 12px', borderRadius: 999, background: '#FFFFFF1A', fontSize: 'calc(12px * var(--fs))', color: '#D4D7DC', whiteSpace: 'nowrap' }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: '#3CCB7F' }} />{tr('Позиция сохраняется · синхр. с ТВ', 'Position is saved · synced with TV')}
          </span>
        )}
        <button className="pb" onClick={close} aria-label={tr('Закрыть плеер', 'Close player')} style={{ background: '#FFFFFF1A', marginLeft: itemId ? 0 : 'auto', flex: 'none' }}><Icon name="close" size={18} /></button>
      </header>

      {(waiting || !pb) && !err && (
        <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', pointerEvents: 'none' }}>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12, fontSize: 'calc(13px * var(--fs))', color: '#D4D7DC' }}>
            <span className="spin" style={{ width: 36, height: 36, borderWidth: 3, borderColor: '#FFFFFF33', borderTopColor: 'var(--acc)' }} />
            {stream && !stream.ready ? `${tr('Буферизация', 'Buffering')} ${fmtSize(stream.buffered)} / ${fmtSize(stream.target)} · ${stream.seeds} ${tr('сидов', 'seeds')}`
              : pb?.mode === 'hls' ? tr('Готовим поток…', 'Preparing stream…') : ''}
          </div>
        </div>
      )}
      {err && (
        <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', padding: 24 }}>
          <div style={{ maxWidth: 460, textAlign: 'center', display: 'flex', flexDirection: 'column', gap: 14, alignItems: 'center' }}>
            <div style={{ fontSize: 'calc(16px * var(--fs))', fontWeight: 700 }}>{tr('Не удалось воспроизвести', 'Playback failed')}</div>
            <div style={{ fontSize: 'calc(13px * var(--fs))', color: '#C9CCD1' }}>{err}</div>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'center' }}>
              <button className="btn btn-w sm" onClick={() => load(cur)}>{tr('Повторить', 'Retry')}</button>
              {state?.local && !hash && <button className="btn btn-g sm" onClick={external}>{tr('Внешний плеер', 'External player')}</button>}
              <button className="btn btn-g sm" onClick={close}>{tr('Назад', 'Back')}</button>
            </div>
          </div>
        </div>
      )}

      {subNow && (
        <div style={{ position: 'absolute', left: 0, right: 0, bottom: showUI ? 150 : 60, textAlign: 'center', pointerEvents: 'none', transition: 'bottom .3s', padding: '0 24px' }}>
          <span style={{ display: 'inline-block', padding: '4px 10px', background: '#000000B3', fontSize: 'calc(clamp(18px,2.2vw,28px) * var(--fs))', fontWeight: 600, lineHeight: 1.35, whiteSpace: 'pre-line' }}>{subNow}</span>
        </div>
      )}

      {countdown !== null && next && (
        <div style={{ position: 'absolute', right: 24, bottom: 120, width: 300, borderRadius: 16, background: '#FFFFFF', color: '#111214', padding: 16, display: 'flex', flexDirection: 'column', gap: 10, boxShadow: '0 20px 60px #00000080' }}>
          <div style={{ fontSize: 'calc(12px * var(--fs))', color: '#6B6F76' }}>{tr('Следующая серия через', 'Next episode in')} {countdown} {tr('с', 's')}</div>
          <div style={{ fontWeight: 700 }}>С{next.season}·Э{next.episode} {next.epTitle}</div>
          <div style={{ display: 'flex', gap: 8 }}>
            <button className="btn btn-d sm" onClick={goNext}><PlayGlyph size={14} />{tr('Смотреть', 'Play')}</button>
            <button className="btn btn-l sm" style={{ background: '#F2F2F0', color: '#111214' }} onClick={() => setCountdown(null)}>{tr('Отмена', 'Cancel')}</button>
          </div>
        </div>
      )}

      {panel && pb && (
        <section aria-label={tr('Дорожки', 'Tracks')} style={{ position: 'absolute', right: 24, bottom: 108, width: 'min(340px, calc(var(--vw) - 48px))', maxHeight: 'calc(var(--vh) - 200px)', overflow: 'auto', borderRadius: 16, background: '#FFFFFF', color: '#111214', boxShadow: '0 20px 60px #00000080', padding: 14, display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div role="tablist" style={{ display: 'flex', gap: 4, padding: 4, borderRadius: 999, background: '#F2F2F0', marginBottom: 6 }}>
            {([['subs', tr('Субтитры', 'Subtitles')], ['audio', tr('Аудио', 'Audio')], ['speed', tr('Скорость', 'Speed')]] as const).map(([k, l]) => (
              <button key={k} role="tab" aria-selected={panel === k} onClick={() => setPanel(k)}
                style={{ flex: 1, height: 32, borderRadius: 999, border: 0, background: panel === k ? '#111214' : 'transparent', color: panel === k ? '#FFFFFF' : '#2A2C30', font: "600 calc(12px * var(--fs)) 'Inter',sans-serif", cursor: 'pointer' }}>{l}</button>
            ))}
          </div>
          {panel === 'subs' && (
            <>
              <Opt on={subIdx < 0} name={tr('Выключены', 'Off')} onClick={() => pickSub(-1)} />
              {pb.subs.map((s) => {
                const [name, ...meta] = s.label.split(' · ');
                return <Opt key={s.index} on={s.index === subIdx} name={name} meta={meta.join(' · ') + (s.bitmap ? ' · ' + tr('вжигаются', 'burned in') : '')} onClick={() => pickSub(s.index)} />;
              })}
              {pb.subs.length === 0 && <div style={{ fontSize: 'calc(12px * var(--fs))', color: '#6B6F76', padding: '4px 10px' }}>{hash ? tr('Субтитры потока доступны после загрузки', 'Stream subtitles appear after download') : tr('В файле нет субтитров. Положите .srt рядом с видео — он появится здесь.', 'No subtitles. Put an .srt next to the video.')}</div>}
              <div style={{ height: 1, background: '#ECECE9', margin: '4px 0' }} />
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 10px', fontSize: 'calc(12px * var(--fs))', color: '#4A4D52' }}>
                {tr('Сдвиг', 'Offset')}
                <button className="opt" aria-label={tr('Раньше на 0,1 с', 'Earlier by 0.1 s')} onClick={() => setSubDelay((d) => Math.round((d - 0.1) * 10) / 10)} style={{ width: 32, minHeight: 32, padding: 0, justifyContent: 'center', background: '#F2F2F0' }}>−</button>
                <span className="mono">{subDelay >= 0 ? '+' : ''}{subDelay.toFixed(1)} {tr('с', 's')}</span>
                <button className="opt" aria-label={tr('Позже на 0,1 с', 'Later by 0.1 s')} onClick={() => setSubDelay((d) => Math.round((d + 0.1) * 10) / 10)} style={{ width: 32, minHeight: 32, padding: 0, justifyContent: 'center', background: '#F2F2F0' }}>+</button>
                {subDelay !== 0 && <button className="opt" onClick={() => setSubDelay(0)} style={{ width: 'auto', minHeight: 32 }}>{tr('Сброс', 'Reset')}</button>}
              </div>
              {curSub && <div style={{ fontSize: 'calc(11px * var(--fs))', color: '#6B6F76', padding: '0 10px' }}>{tr('Сейчас:', 'Now:')} {curSub.label}</div>}
            </>
          )}
          {panel === 'audio' && pb.audio.map((a) => {
            const [name, ...meta] = a.label.split(' · ');
            return <Opt key={a.index} on={a.selected} name={name} meta={meta.join(' · ')} onClick={() => pickAudio(a.index)} />;
          })}
          {panel === 'audio' && pb.audio.length === 0 && <div style={{ fontSize: 'calc(12px * var(--fs))', color: '#6B6F76', padding: '4px 10px' }}>{tr('Звуковых дорожек нет', 'No audio tracks')}</div>}
          {panel === 'speed' && speeds.map((s) => <Opt key={s} on={speed === s} name={`${s}×`} onClick={() => { setSpeed(s); setPanel(null); }} />)}
        </section>
      )}

      <footer style={{ position: 'absolute', left: 0, right: 0, bottom: 0, padding: '12px 20px', background: 'linear-gradient(#00000000,#000000E6 40%)', display: 'flex', flexDirection: 'column', gap: 8, opacity: showUI ? 1 : 0, transition: 'opacity .3s', paddingTop: 32 }}>
        {stream && (
          <div style={{ display: 'flex', gap: 14, fontSize: 'calc(11px * var(--fs))', color: '#A9ADB4' }} className="mono">
            <span>{tr('Загружено', 'Downloaded')} {Math.round(stream.progress * 100)}%</span><span>↓ {fmtSize(stream.down)}/s</span><span>{stream.seeds} {tr('сидов', 'seeds')} · {stream.peers} {tr('пиров', 'peers')}</span>
          </div>
        )}
        <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
          <span className="mono" style={{ fontSize: 'calc(12px * var(--fs))', color: '#D4D7DC', minWidth: 52 }}>{fmtTime(cur)}</span>
          <div role="slider" tabIndex={0} aria-label={tr('Позиция', 'Position')} aria-valuemin={0} aria-valuemax={Math.round(duration)} aria-valuenow={Math.round(cur)} aria-valuetext={fmtTime(cur)}
            onMouseMove={(e) => setHover(barX(e))} onMouseLeave={() => setHover(null)} onClick={(e) => seek(barX(e).t)}
            onKeyDown={(e) => { if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') e.stopPropagation(); if (e.key === 'ArrowLeft') seek(cur - 5); if (e.key === 'ArrowRight') seek(cur + 5); }}
            style={{ flex: 1, height: 16, display: 'flex', alignItems: 'center', cursor: 'pointer', position: 'relative' }}>
            <div style={{ position: 'relative', width: '100%', height: 4, borderRadius: 2, background: '#FFFFFF2E' }}>
              <div style={{ position: 'absolute', left: 0, top: 0, height: 4, width: `${bpct * 100}%`, borderRadius: 2, background: '#FFFFFF59' }} />
              <div style={{ position: 'absolute', left: 0, top: 0, height: 4, width: `${pct * 100}%`, borderRadius: 2, background: 'var(--acc)' }} />
              <div style={{ position: 'absolute', left: `${pct * 100}%`, top: -5, width: 14, height: 14, marginLeft: -7, borderRadius: '50%', background: '#FFFFFF' }} />
              {hover && <div style={{ position: 'absolute', left: hover.x, bottom: 14, transform: 'translateX(-50%)', padding: '3px 8px', borderRadius: 6, background: '#FFFFFF', color: '#111214', fontSize: 'calc(11px * var(--fs))', fontWeight: 700, whiteSpace: 'nowrap' }}>{fmtTime(hover.t)}</div>}
            </div>
          </div>
          <span className="mono" style={{ fontSize: 'calc(12px * var(--fs))', color: '#D4D7DC', minWidth: 60, textAlign: 'right' }}>−{fmtTime(Math.max(0, duration - cur))}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'wrap' }}>
          <button className="pb" onClick={toggle} aria-label={paused ? tr('Смотреть', 'Play') : tr('Пауза', 'Pause')}>{paused ? <PlayGlyph size={20} /> : <PauseGlyph size={20} />}</button>
          <button className="pb" onClick={() => seek(cur - 10)} aria-label={tr('Назад 10 секунд', 'Back 10 seconds')}><Icon name="back10" /></button>
          <button className="pb" onClick={() => seek(cur + 10)} aria-label={tr('Вперёд 10 секунд', 'Forward 10 seconds')}><Icon name="fwd10" /></button>
          <button className="pb" onClick={() => setMuted((m) => !m)} aria-label={muted ? tr('Включить звук', 'Unmute') : tr('Выключить звук', 'Mute')}><Icon name={muted || vol === 0 ? 'mute' : 'volume'} /></button>
          <input type="range" className="rng" min={0} max={1} step={0.05} value={muted ? 0 : vol} aria-label={tr('Громкость', 'Volume')}
            onChange={(e) => { setVol(Number(e.target.value)); setMuted(false); }} style={{ width: 80 }} />
          <span className="hk" style={{ marginLeft: 16, fontSize: 'calc(12px * var(--fs))', color: '#A9ADB4' }}>{tr('Пробел — пауза · ← → 10 с · F — экран · S — субтитры', 'Space — pause · ← → 10 s · F — fullscreen · S — subtitles')}</span>
          <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 4 }}>
            {next && <button className="pb" onClick={goNext} aria-label={tr('Следующая серия', 'Next episode')} style={{ width: 'auto', padding: '0 12px', borderRadius: 999, font: "600 calc(12px * var(--fs)) 'Inter',sans-serif" }}>{tr('След. серия', 'Next')} ›</button>}
            <button className="pb" onClick={() => setPanel(panel === 'speed' ? null : 'speed')} aria-label={tr('Скорость', 'Speed')} style={{ width: 'auto', padding: '0 10px', borderRadius: 999, font: "700 calc(12px * var(--fs)) 'Inter',sans-serif" }}>{speed}×</button>
            <button className="pb" onClick={() => setPanel(panel && panel !== 'speed' ? null : 'subs')} aria-label={tr('Дорожки и субтитры', 'Tracks and subtitles')} aria-expanded={!!panel} style={{ background: panel ? '#FFFFFF2E' : undefined }}><Icon name="subs" /></button>
            <button className="pb" onClick={castNow} aria-label={tr('Отправить на ТВ', 'Send to TV')}><Icon name="cast" /></button>
            {state?.local && !hash && <button className="pb" onClick={external} aria-label={tr('Открыть во внешнем плеере', 'Open in external player')}><Icon name="external" /></button>}
            <button className="pb" onClick={fullscreen} aria-label={tr('Во весь экран', 'Fullscreen')}><Icon name="fullscreen" /></button>
          </div>
        </div>
      </footer>
    </div>
  );
}

function Opt({ on, name, meta, onClick }: { on: boolean; name: string; meta?: string; onClick: () => void }) {
  return (
    <button className="opt" onClick={onClick} aria-pressed={on}>
      <span style={{ width: 16, display: 'grid', placeItems: 'center', color: 'var(--acc-dk)' }}>{on && <Icon name="check" size={16} />}</span>
      <span style={{ flex: 1, fontWeight: on ? 700 : 500 }}>{name}</span>
      {meta && <span style={{ fontSize: 'calc(11px * var(--fs))', color: '#6B6F76' }}>{meta}</span>}
    </button>
  );
}
