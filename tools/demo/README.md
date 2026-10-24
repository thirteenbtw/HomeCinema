# Демо-данные для скриншотов

`run.sh` пересобирает картинки для главного README (`docs/media`): поднимает Rivo с поддельным TMDB и Torznab (`server.py`), небольшой библиотекой из коротких роликов и снимает экраны через Playwright.

Все фильмы, сериалы, люди и постеры здесь выдуманы: `data.py` — каталог, `art.py` рисует постеры, фоны и портреты. Настоящих данных TMDB или чужих постеров в репозитории нет.

```bash
WORK=/tmp/rivo-demo tools/demo/run.sh
```

Нужны Go, Node с пакетом `playwright`, Python 3 с `numpy` и `pillow`, ffmpeg. Сначала соберите интерфейс (`cd web && npm ci && npm run build`).
