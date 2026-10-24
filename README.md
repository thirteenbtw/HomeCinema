<p align="center">
  <img src="docs/media/banner.jpg" alt="Rivo — Your movies. Any screen." width="100%">
</p>

<p align="center">
  <a href="https://github.com/thirteenbtw/homecinema/actions/workflows/build.yml"><img src="https://github.com/thirteenbtw/homecinema/actions/workflows/build.yml/badge.svg" alt="build"></a>
  <img src="https://img.shields.io/badge/Windows-10%20%7C%2011-5B6EE1?logo=windows&logoColor=white" alt="Windows 10 | 11">
  <img src="https://img.shields.io/badge/Go-server-00ADD8?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/React-UI-61DAFB?logo=react&logoColor=black" alt="React">
  <img src="https://img.shields.io/badge/Tauri-2-24C8DB?logo=tauri&logoColor=white" alt="Tauri 2">
  <img src="https://img.shields.io/badge/телеметрия-нет-8EDCC8" alt="Без телеметрии">
</p>

<p align="center">
  <b>Rivo</b> — домашний кинотеатр для Windows. Собирает фильмы и сериалы с ваших дисков в красивую библиотеку,<br>
  играет их в своём плеере, отправляет на телевизор и умеет смотреть торренты, пока они качаются.
</p>

<p align="center">
  <a href="#установка"><b>Скачать</b></a> ·
  <a href="#возможности">Возможности</a> ·
  <a href="#скриншоты">Скриншоты</a> ·
  <a href="#разработка">Разработка</a> ·
  <a href="#english">English</a>
</p>

<p align="center">
  <img src="docs/media/demo.gif" alt="Rivo в работе: главная, карточка фильма, плеер" width="800">
</p>

## Возможности

<table>
<tr>
<td width="50%" valign="top">

### 🎬 Библиотека
Укажите папки — Rivo сам найдёт фильмы и сериалы, разберёт имена вида `Title.2014.1080p`, `S01E02` или «Сезон 1 / Серия 2», подтянет постеры, описания и актёров из TMDB. Новые файлы появляются сами, отключённый диск не стирает записи.

</td>
<td width="50%" valign="top">

### ▶️ Плеер
Прямое воспроизведение или транскод на лету с аппаратным ускорением (NVENC, Quick Sync, AMF). HDR → SDR, выбор дорожки, субтитры SRT/ASS/VTT со сдвигом, горячие клавиши, автопереход к следующей серии.

</td>
</tr>
<tr>
<td valign="top">

### 📺 На телевизор
Находит Google Cast устройства в сети, работает как пульт: пауза, перемотка, громкость, смена дорожек. Позиция просмотра общая для ПК и ТВ — начали на одном, продолжили на другом.

</td>
<td valign="top">

### 🧲 Торренты
Magnet и `.torrent`, режим «Смотреть сразу» с последовательной загрузкой, очередь, лимиты, расписание. Привязка к VPN с kill switch. Готовое само переезжает в библиотеку.

</td>
</tr>
<tr>
<td valign="top">

### 🔎 Каталог
Тренды, новинки, подборки по жанрам и страницы актёров. На странице фильма — раздачи из ваших источников (JacRed, Jackett, Prowlarr), лучшая подсвечена.

</td>
<td valign="top">

### 👥 Для всей семьи
До пяти профилей с PIN, своей темой и историей. Светлая, тёмная и OLED-темы, «живой» фон. Интерфейс открывается и с телефона: `http://<IP ПК>:8096`.

</td>
</tr>
</table>

## Скриншоты

<table>
<tr>
<td width="50%"><img src="docs/media/home-light.jpg" alt="Главная"><p align="center"><sub>Главная: тренды и «Продолжить просмотр»</sub></p></td>
<td width="50%"><img src="docs/media/home-dark.jpg" alt="Главная, тёмная тема"><p align="center"><sub>Та же главная в тёмной теме</sub></p></td>
</tr>
<tr>
<td><img src="docs/media/title-light.jpg" alt="Страница фильма"><p align="center"><sub>Фильм: смотреть из библиотеки или сразу из раздачи</sub></p></td>
<td><img src="docs/media/show-dark.jpg" alt="Страница сериала"><p align="center"><sub>Сериал: сезоны, серии, дорожки и субтитры</sub></p></td>
</tr>
<tr>
<td><img src="docs/media/player.jpg" alt="Плеер"><p align="center"><sub>Плеер</sub></p></td>
<td><img src="docs/media/library-light.jpg" alt="Библиотека"><p align="center"><sub>Библиотека с ваших дисков</sub></p></td>
</tr>
<tr>
<td><img src="docs/media/person-light.jpg" alt="Страница актёра"><p align="center"><sub>Страница актёра</sub></p></td>
<td><img src="docs/media/torrents-light.jpg" alt="Торренты"><p align="center"><sub>Поиск раздач</sub></p></td>
</tr>
<tr>
<td><img src="docs/media/title-dark.jpg" alt="Страница фильма, тёмная тема"><p align="center"><sub>Раздачи с качеством, размером и сидами</sub></p></td>
<td><img src="docs/media/settings-light.jpg" alt="Настройки"><p align="center"><sub>Настройки: темы, акценты, папки</sub></p></td>
</tr>
</table>

<sub>Фильмы, постеры и люди на скриншотах выдуманы и нарисованы для демонстрации; в настоящем приложении данные приходят из TMDB.</sub>

## Установка

1. Откройте [**Releases**](https://github.com/thirteenbtw/homecinema/releases) и скачайте `Rivo_<версия>_x64-setup.exe`.
   Пока релиза нет, свежая сборка лежит в [**Actions**](https://github.com/thirteenbtw/homecinema/actions/workflows/build.yml): выберите последний зелёный запуск и скачайте артефакт `Rivo-windows-installer` (нужен вход в GitHub).
2. Запустите установщик. Он ставит сервер, плеер и FFmpeg, а также открывает порт в брандмауэре только для частных сетей.
3. При первом запуске укажите папки с фильмами и сериалами. Всё остальное Rivo сделает сам.

Rivo работает только в вашей локальной сети и ничего никуда не отправляет. Данные хранятся в `%APPDATA%\HomeCinema` (имя папки осталось от прежнего названия, чтобы обновление не потеряло библиотеку). Если стояла старая версия HomeCinema, установщик удалит её сам.

## Как это устроено

| Часть | Где | Технологии |
|---|---|---|
| Медиасервер | [`server/`](server) | Go, SQLite (без cgo), ffmpeg/ffprobe, anacrolix/torrent, go-chromecast |
| Интерфейс | [`web/`](web) | React 18 + TypeScript + Vite, hls.js |
| Оболочка Windows | [`desktop/`](desktop) | Tauri 2: окно, трей, автозапуск, magnet-ссылки, установщик NSIS |

Интерфейс встраивается в бинарник сервера, поэтому тот же UI открывается в браузере телефона или ТВ.

<details>
<summary><b>Все возможности подробно</b></summary>

- **Библиотека.** Папки отслеживаются (fsnotify + пересканирование раз в 30 минут), разбор имён вида `Title.2014.1080p`, `S01E02`, `1x02`, «Сезон 1 / Серия 2». Метаданные, постеры и актёры из TMDB (ключ v3 или токен v4, либо через зеркала), ручное исправление совпадения. Отключённый диск не стирает записи.
- **Плеер.** Прямое воспроизведение с Range, либо HLS-транскод через ffmpeg с аппаратным ускорением (NVENC / Quick Sync / AMF / VAAPI, иначе CPU), тонмаппинг HDR → SDR, выбор аудиодорожки, субтитры SRT/ASS/VTT (внешние и встроенные, CP1251 распознаётся) со сдвигом, PGS/VobSub вжигаются. Горячие клавиши, скорость, автопереход к следующей серии, внешний плеер VLC/mpv.
- **Позиция просмотра** синхронизируется между ПК и ТВ; «Продолжить просмотр», избранное, «Посмотреть позже», отметки «просмотрено».
- **ТВ.** Поиск Google Cast устройств по mDNS, добавление по IP, пульт: пауза, перемотка, громкость, смена дорожек, «Продолжить на ПК». Плееры подключены через интерфейс `Renderer`, DLNA/AirPlay добавляются отдельными реализациями.
- **Торренты.** Magnet / `.torrent` (в том числе перетаскиванием), «Смотреть сразу» с последовательной загрузкой и буфером, карта кусков, очередь, приоритеты, лимиты скорости, расписание, раздача до рейтинга/времени, контроль свободного места. Привязка к интерфейсу VPN с kill switch, SOCKS5-прокси, шифрование. Готовые загрузки переносятся в библиотеку как `Название (Год)/…` и `Сериал/Season N/…`. Исполняемые файлы из раздач блокируются. Поиск только по источникам, которые выбирает пользователь (JacRed, Jackett / Prowlarr через Torznab).
- **Профили** (до 5) с PIN, своей темой и историей. Темы: светлая, тёмная, OLED; несколько акцентных цветов; «живой» фон.
- **Безопасность.** Сервер отвечает только адресам локальной сети, по желанию пароль для других устройств (на самом ПК не спрашивается). ТВ получают ссылки с подписанными токенами на 6 часов.
- **Обслуживание.** Резервная копия и восстановление БД, экспорт просмотренного в CSV, диагностика (ffmpeg, сеть, ТВ, TMDB, порт торрентов, VPN, диск), журнал.
- **Языки:** русский и английский.

</details>

## Разработка

Нужны Go 1.26+, Node 20+, ffmpeg в `PATH`; для оболочки ещё Rust и WebView2.

```bash
# сервер (API на :8096)
cd server && go run ./cmd/homecinema -dev -data ./devdata

# интерфейс с hot reload (прокси /api → :8096)
cd web && npm ci && npm run dev

# сборка UI внутрь сервера и тесты
cd web && npm run build
cd server && go test ./... && go build ./cmd/homecinema
```

<details>
<summary>Сборка оболочки Windows и флаги сервера</summary>

```bash
# положите бинарники с суффиксом целевой платформы:
#   desktop/src-tauri/binaries/rivo-server-x86_64-pc-windows-msvc.exe
#   desktop/src-tauri/binaries/ffmpeg-x86_64-pc-windows-msvc.exe
#   desktop/src-tauri/binaries/ffprobe-x86_64-pc-windows-msvc.exe
cd desktop && npm ci && npx tauri build
```

Готовый установщик собирает GitHub Actions ([`.github/workflows/build.yml`](.github/workflows/build.yml)) на `windows-latest`: артефакт `Rivo-windows-installer` у каждого запуска, а при пуше тега `v*` — релиз с `.exe`. Установщик кладёт рядом сервер и LGPL-сборку FFmpeg (BtbN).

Флаги сервера: `-data`, `-listen` (по умолчанию `0.0.0.0:8096`, при занятом порте пробует следующие), `-ffmpeg`, `-ffprobe`, `-dev`, `-exit-on-stdin-close` (для оболочки). Когда сервер готов, он печатает `HOMECINEMA_READY http://127.0.0.1:<порт>` — по этой строке оболочка открывает окно.

</details>

<details>
<summary>Фирменный стиль</summary>

Палитра: лаванда `#A5B4FC` (бренд), лёд `#E7ECF7` (фон), ирис `#5B6EE1` (действия), мята `#8EDCC8` (акценты, прогресс), ночь `#141A33` (текст, тёмная тема). Заголовки — Inter Tight 800–900, интерфейс — Inter. Знак — кольцо с наклонённым квадратом-искрой, логотип — «rıvo». Иконки приложения собираются из `desktop/app-icon.png` (`npm run icons` в `desktop/`), исходник — `desktop/app-icon.svg`.

</details>

<details>
<summary>Что проверено, а что нет</summary>

Проверено в Linux-контейнере: сканирование и разбор имён (юнит-тесты), прямое воспроизведение и HLS-транскод HEVC 10-bit на CPU в браузере, внешние субтитры, добавление торрента, раздача и автоимпорт в библиотеку, все экраны интерфейса. Rust-оболочка проходит `cargo check` под `x86_64-pc-windows-msvc`.

Не проверено вживую: запросы к настоящему TMDB, трансляция на реальный Chromecast, аппаратные кодировщики, привязка к VPN на Windows, собранный установщик.

</details>

## Лицензии и ответственность

FFmpeg поставляется в LGPL-сборке как отдельные `ffmpeg.exe` / `ffprobe.exe`. Данные о фильмах — [TMDB](https://www.themoviedb.org/); Rivo не связан с TMDB и не одобрен ею. Ответственность за законность загружаемого и раздаваемого через торренты контента лежит на пользователе; в приложении нет встроенного контента.

---

## English

**Rivo** is a home media server for Windows 10/11. Point it at your folders and it builds a poster library with TMDB metadata, plays everything in its own player (direct play or hardware-accelerated transcoding, HDR → SDR, subtitles), casts to Google Cast TVs with a remote and shared watch position, and streams torrents while they download (VPN binding with kill switch). Up to five profiles, light/dark/OLED themes, Russian and English UI. It works only inside your LAN, with no telemetry.

**Install:** grab `Rivo_<version>_x64-setup.exe` from [Releases](https://github.com/thirteenbtw/homecinema/releases), or the `Rivo-windows-installer` artifact from the latest green [Actions run](https://github.com/thirteenbtw/homecinema/actions/workflows/build.yml). **Build from source:** see [Разработка](#разработка) above — the commands are the same in any language.

<sub>Titles, posters and people in the screenshots are fictional and generated for the demo.</sub>
