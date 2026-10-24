// A .torrent file dropped anywhere in the app opens the Torrents page and is
// added there; other drops never make the window navigate to the file.
let pending: File | null = null;

export function takeDroppedTorrent(): File | null {
  const f = pending;
  pending = null;
  return f;
}

const hasFiles = (e: DragEvent) => !!e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files');

export function installTorrentDrop(open: () => void): () => void {
  const over = (e: DragEvent) => { if (hasFiles(e)) e.preventDefault(); };
  const drop = (e: DragEvent) => {
    if (e.defaultPrevented || !hasFiles(e)) return; // the Torrents page handled it
    e.preventDefault();
    const f = Array.from(e.dataTransfer!.files).find((x) => /\.torrent$/i.test(x.name));
    if (f) { pending = f; open(); }
  };
  window.addEventListener('dragover', over);
  window.addEventListener('drop', drop);
  return () => { window.removeEventListener('dragover', over); window.removeEventListener('drop', drop); };
}
