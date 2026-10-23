// Russian and English UI strings. Keys are short; missing English falls back to Russian.
const ru = {
  home: 'Главная', movies: 'Фильмы', shows: 'Сериалы', torrents: 'Торренты', devices: 'Устройства', favorites: 'Избранное', later: 'Посмотреть позже',
  settings: 'Настройки', profile: 'Профиль', theme: 'Тема', back: 'Назад', forward: 'Вперёд',
  searchPh: 'Фильмы, сериалы и люди', all: 'Все', people: 'Люди', liveBg: 'Живой фон', bgOff: 'Фон выкл.',
  watch: 'Смотреть', continue: 'Продолжить', toTv: 'На ТВ', details: 'Подробнее', inList: 'В список', inListOn: 'В списке',
  addFav: 'В избранное', markWatched: 'Отметить просмотренным', watched: 'Просмотрено',
  continueWatching: 'Продолжить просмотр', syncNote: 'Позиция синхронизируется между ПК и ТВ', popular: 'Популярное в библиотеке',
  popularNote: 'Что чаще всего смотрят профили этого дома', recent: 'Недавно добавленные', recentNote: 'Из папок и завершённых загрузок',
  seeAll: 'Все ›', genre: 'ЖАНР', left: 'Ещё', min: 'мин', h: 'ч', scanning: 'Сканирование', found: 'найдено новых файлов',
  libEmpty: 'Библиотека пуста', libEmptyHint: 'Добавьте папку с фильмами или сериалами — через минуту здесь появятся постеры.',
  addFolder: 'Добавить папку', cast: 'В ролях', similar: 'Похожее в библиотеке', similarNote: 'По жанрам и ключевым словам TMDB',
  tracks: 'Дорожки', audio: 'Аудио', subtitles: 'Субтитры', off: 'Выключены', whereplays: 'Где пойдёт', file: 'Файл',
  video: 'Видео', size: 'Размер', wrongMatch: 'Неверное совпадение? Исправить', origNote: 'Перевода описания нет — показан оригинал',
  noOverview: 'Описание появится, когда библиотека получит данные TMDB.', seasons: 'Сезоны', season: 'Сезон', episodes: 'серий',
  nextUp: 'Следующая серия', all2: 'Все',
  sort: 'Сортировка', sortAdded: 'Недавно добавленные', sortTitle: 'По названию', sortYear: 'По году', sortRating: 'По рейтингу',
  anyGenre: 'Все жанры', unwatched: 'Не просмотрено', watchedF: 'Просмотрено', nothing: 'Ничего не найдено',
  pickDevice: 'Отправить на ТВ', noDevices: 'ТВ не найдены. Проверьте, что ТВ и ПК в одной сети, или добавьте по IP.',
  send: 'Отправить', cancel: 'Отмена', save: 'Сохранить', close: 'Закрыть', delete: 'Удалить', add: 'Добавить',
  loading: 'Загрузка…', error: 'Ошибка',
} as const;

export type Key = keyof typeof ru;

const en: Partial<Record<Key, string>> = {
  home: 'Home', movies: 'Movies', shows: 'Shows', torrents: 'Torrents', devices: 'Devices', favorites: 'Favorites', later: 'Watch later',
  settings: 'Settings', profile: 'Profile', theme: 'Theme', back: 'Back', forward: 'Forward',
  searchPh: 'Movies, shows and people', all: 'All', people: 'People', liveBg: 'Live background', bgOff: 'Background off',
  watch: 'Watch', continue: 'Continue', toTv: 'To TV', details: 'Details', inList: 'Add to list', inListOn: 'In list',
  addFav: 'Add to favorites', markWatched: 'Mark as watched', watched: 'Watched',
  continueWatching: 'Continue watching', syncNote: 'Position syncs between PC and TV', popular: 'Popular in the library',
  popularNote: 'What this household watches most', recent: 'Recently added', recentNote: 'From folders and finished downloads',
  seeAll: 'All ›', genre: 'GENRE', left: '', min: 'min left', h: 'h', scanning: 'Scanning', found: 'new files found',
  libEmpty: 'The library is empty', libEmptyHint: 'Add a folder with movies or shows — posters appear within a minute.',
  addFolder: 'Add folder', cast: 'Cast', similar: 'Similar in the library', similarNote: 'By TMDB genres and keywords',
  tracks: 'Tracks', audio: 'Audio', subtitles: 'Subtitles', off: 'Off', whereplays: 'Where it plays', file: 'File',
  video: 'Video', size: 'Size', wrongMatch: 'Wrong match? Fix it', origNote: 'No translation — showing the original',
  noOverview: 'The description appears once TMDB data is loaded.', seasons: 'Seasons', season: 'Season', episodes: 'episodes',
  nextUp: 'Next episode', all2: 'All',
  sort: 'Sort', sortAdded: 'Recently added', sortTitle: 'Title', sortYear: 'Year', sortRating: 'Rating',
  anyGenre: 'All genres', unwatched: 'Unwatched', watchedF: 'Watched', nothing: 'Nothing found',
  pickDevice: 'Send to TV', noDevices: 'No TVs found. Make sure the TV and PC share a network, or add one by IP.',
  send: 'Send', cancel: 'Cancel', save: 'Save', close: 'Close', delete: 'Delete', add: 'Add',
  loading: 'Loading…', error: 'Error',
};

let lang: 'ru' | 'en' = 'ru';
export function setLang(l: string) { lang = l === 'en' ? 'en' : 'ru'; document.documentElement.lang = lang; }
export function getLang() { return lang; }
export function t(k: Key | string): string {
  const key = k as Key;
  if (lang === 'en' && en[key] !== undefined) return en[key] as string;
  return (ru as Record<string, string>)[key] ?? k;
}
// tr picks between inline Russian and English strings for one-off texts.
export function tr(ruText: string, enText: string) { return lang === 'en' ? enText : ruText; }
