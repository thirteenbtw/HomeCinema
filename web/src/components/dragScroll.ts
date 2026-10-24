// Horizontal rows (.scroll-x) have no scrollbars: they scroll with the mouse
// wheel and by dragging, installed once for the whole app by delegation.

function rowAt(t: EventTarget | null): HTMLElement | null {
  return t instanceof Element ? (t.closest('.scroll-x') as HTMLElement | null) : null;
}

export function installDragScroll(): () => void {
  const onWheel = (e: WheelEvent) => {
    const row = rowAt(e.target);
    if (!row || e.ctrlKey || Math.abs(e.deltaX) > Math.abs(e.deltaY)) return; // touchpads scroll sideways natively
    const max = row.scrollWidth - row.clientWidth;
    if (max <= 0) return;
    const d = e.deltaMode === 1 ? e.deltaY * 40 : e.deltaY;
    // At either end, let the page scroll on.
    if ((d < 0 && row.scrollLeft <= 0) || (d > 0 && row.scrollLeft >= max - 1)) return;
    e.preventDefault();
    row.scrollLeft += d;
  };

  let drag: { row: HTMLElement; x: number; left: number; moved: boolean; id: number } | null = null;
  const onDown = (e: PointerEvent) => {
    if (e.pointerType !== 'mouse' || e.button !== 0) return; // touch scrolls natively
    const row = rowAt(e.target);
    if (!row || row.scrollWidth <= row.clientWidth) return;
    drag = { row, x: e.clientX, left: row.scrollLeft, moved: false, id: e.pointerId };
  };
  const onMove = (e: PointerEvent) => {
    if (!drag || e.pointerId !== drag.id) return;
    const dx = e.clientX - drag.x;
    if (!drag.moved && Math.abs(dx) < 6) return;
    if (!drag.moved) { drag.moved = true; drag.row.classList.add('dragging'); }
    drag.row.scrollLeft = drag.left - dx;
  };
  const onUp = () => {
    if (!drag) return;
    const d = drag;
    drag = null;
    if (!d.moved) return;
    d.row.classList.remove('dragging');
    // Swallow the click that ends a drag so a poster under the cursor does not open.
    const stop = (ev: Event) => { ev.stopPropagation(); ev.preventDefault(); };
    window.addEventListener('click', stop, { capture: true, once: true });
    setTimeout(() => window.removeEventListener('click', stop, { capture: true }), 0);
  };

  window.addEventListener('wheel', onWheel, { passive: false });
  window.addEventListener('pointerdown', onDown);
  window.addEventListener('pointermove', onMove);
  window.addEventListener('pointerup', onUp);
  window.addEventListener('pointercancel', onUp);
  return () => {
    window.removeEventListener('wheel', onWheel);
    window.removeEventListener('pointerdown', onDown);
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onUp);
  };
}
