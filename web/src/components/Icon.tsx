// Stroke icons from the design (24×24, currentColor).
import type { CSSProperties } from 'react';

const paths: Record<string, JSX.Element> = {
  home: <path d="M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" />,
  film: <><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4" /></>,
  tv: <><rect x="3" y="5" width="18" height="12" rx="2" /><path d="M8 21h8M12 17v4" /></>,
  download: <path d="M12 3v12M7 10l5 5 5-5M5 21h14" />,
  cast: <path d="M2 16a5 5 0 0 1 5 5M2 12a9 9 0 0 1 9 9M2 8V6a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-6" />,
  heart: <path d="M12 20s-7-4.5-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.5-7 10-7 10z" />,
  clock: <><circle cx="12" cy="12" r="8" /><path d="M12 8v4l3 2" /></>,
  theme: <><circle cx="12" cy="12" r="8" /><path d="M12 4a8 8 0 0 1 0 16z" fill="currentColor" /></>,
  sliders: <><path d="M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12" /><circle cx="16" cy="6" r="2" /><circle cx="10" cy="12" r="2" /><circle cx="18" cy="18" r="2" /></>,
  left: <path d="M15 6l-6 6 6 6" />,
  right: <path d="M9 6l6 6-6 6" />,
  search: <><circle cx="11" cy="11" r="7" /><path d="M20 20l-4-4" /></>,
  sparkle: <path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z" />,
  plus: <path d="M12 5v14M5 12h14" />,
  check: <path d="M5 12l5 5 9-10" />,
  close: <path d="M6 6l12 12M18 6L6 18" />,
  back10: <path d="M4 12a8 8 0 1 0 3-6.2M4 4v4h4" />,
  fwd10: <path d="M20 12a8 8 0 1 1-3-6.2M20 4v4h-4" />,
  volume: <path d="M4 9h4l5-4v14l-5-4H4zM17 9a4 4 0 0 1 0 6" />,
  mute: <path d="M4 9h4l5-4v14l-5-4H4zM17 9l4 6M21 9l-4 6" />,
  subs: <><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M7 13h4M13 13h4M7 16h8" /></>,
  external: <path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5" />,
  fullscreen: <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />,
  refresh: <path d="M21 12a9 9 0 1 1-3-6.7M21 4v5h-5" />,
  folder: <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />,
  magnet: <path d="M6 3v8a6 6 0 0 0 12 0V3M6 3h4v8a2 2 0 0 0 4 0V3h4" />,
  trash: <path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" />,
  pause2: <path d="M9 6v12M15 6v12" />,
  up: <path d="M12 19V5M6 11l6-6 6 6" />,
  lock: <><rect x="5" y="11" width="14" height="10" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></>,
  user: <><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></>,
  compass: <><circle cx="12" cy="12" r="9" /><path d="M15.5 8.5l-2 5-5 2 2-5z" /></>,
  edit: <path d="M4 20h4L20 8l-4-4L4 16zM14 6l4 4" />,
};

export function Icon({ name, size, style, className }: { name: keyof typeof paths | string; size?: number; style?: CSSProperties; className?: string }) {
  const s = size ? { width: size, height: size, ...style } : style;
  return (
    <svg className={'ic' + (className ? ' ' + className : '')} viewBox="0 0 24 24" aria-hidden="true" style={s}>
      {paths[name]}
    </svg>
  );
}

export function PlayGlyph({ size = 16 }: { size?: number }) {
  return <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true"><path d="M7 4l13 8-13 8z" fill="currentColor" /></svg>;
}

export function PauseGlyph({ size = 20 }: { size?: number }) {
  return <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true"><path d="M7 5h3v14H7zM14 5h3v14h-3z" fill="currentColor" /></svg>;
}

export function StopGlyph({ size = 16 }: { size?: number }) {
  return <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true"><rect x="6" y="6" width="12" height="12" rx="1" fill="currentColor" /></svg>;
}
