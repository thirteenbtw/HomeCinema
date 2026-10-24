// Likes, "watch later" and "watched" for catalog titles: one small store the
// whole app reads, so a heart toggled on a title page shows on every card.
import { useSyncExternalStore } from 'react';
import { api, type CatalogItem } from './api';

export type MarkList = 'liked' | 'later' | 'watched';
export type Mark = { list: MarkList; kind: 'movie' | 'tv'; id: number; title: string; origTitle: string; year: number; poster: string; rating: number; source: string; addedAt: number };
type Keys = Record<MarkList, Record<string, number>>; // "movie:123" → own rating (0 = none)

let keys: Keys = { liked: {}, later: {}, watched: {} };
let version = 0;
const subs = new Set<() => void>();
const emit = () => { version++; subs.forEach((f) => f()); };

export function loadMarks() {
  return api.get<Keys>('/api/marks/keys').then((k) => { keys = k; emit(); }).catch(() => {});
}

function subscribe(f: () => void) { subs.add(f); return () => { subs.delete(f); }; }

// useMarks re-renders the caller whenever any list changes.
export function useMarks() {
  useSyncExternalStore(subscribe, () => version);
  return {
    has: (list: MarkList, x: { kind: string; id: number }) => `${x.kind}:${x.id}` in keys[list],
    rating: (x: { kind: string; id: number }) => keys.watched[`${x.kind}:${x.id}`] || keys.liked[`${x.kind}:${x.id}`] || 0,
  };
}

export async function setMark(list: MarkList, x: Pick<CatalogItem, 'kind' | 'id' | 'title' | 'origTitle' | 'year' | 'poster'>, on: boolean) {
  const k = `${x.kind}:${x.id}`;
  const before = keys[list][k];
  const next = { ...keys[list] };
  if (on) next[k] = before || 0; else delete next[k];
  keys = { ...keys, [list]: next };
  emit();
  try {
    await api.post('/api/marks', { list, kind: x.kind, id: x.id, title: x.title, origTitle: x.origTitle, year: x.year, poster: x.poster, on });
  } catch (e) {
    loadMarks();
    throw e;
  }
}

export const markLabels: Record<MarkList, [string, string]> = {
  liked: ['Нравится', 'Liked'], later: ['Смотреть позже', 'Watch later'], watched: ['Просмотрено', 'Watched'],
};
