#!/usr/bin/env bash
# Starts Rivo against a fake TMDB/Torznab with a small fictional library,
# then takes the README screenshots, GIF and banner into docs/media.
# Needs: go, node with playwright, python3 with numpy + pillow, ffmpeg.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
W=${WORK:-$(mktemp -d)}
B=http://127.0.0.1:8096
cd "$W"

python3 "$HERE/gen.py"
python3 "$HERE/server.py" & FAKE=$!
(cd "$ROOT/server" && go build -o "$W/rivo" ./cmd/homecinema)

# library: short clips cut from the backdrops
mk() { ffmpeg -v error -y -loop 1 -i "$HERE/img/b$1.jpg" -f lavfi -i anullsrc=r=48000:cl=stereo -t 40 \
  -vf "scale=1920:1080,zoompan=z='1+0.0008*on':d=1:s=1280x720:fps=24,format=yuv420p" -c:v libx264 -preset veryfast -crf 26 -c:a aac -shortest "$2"; }
mkdir -p "media/Фильмы" "media/Сериалы/Маяк/Season 1" "media/Сериалы/Vostok Station/Season 1"
for x in "101 Последний рейс (2025)" "105 Красная пустыня (2025)" "106 Затмение (2024)" "103 Город без сна (2025)" "115 Светлячки (2024)" "104 Соль (2023)"; do
  mk "${x%% *}" "media/Фильмы/${x#* }.mp4"; done
for e in 1 2 3 4; do
  mk 201 "media/Сериалы/Маяк/Season 1/Маяк S01E0$e.mp4"
  mk 202 "media/Сериалы/Vostok Station/Season 1/Vostok Station S01E0$e.mp4"; done

./rivo -data ./data -listen 127.0.0.1:8096 > rivo.log 2>&1 & RIVO=$!
trap 'kill $RIVO $FAKE 2>/dev/null' EXIT
sleep 2
post() { curl -s -X "$1" "$B$2" -H 'content-type: application/json' -d "$3" >/dev/null; }
post POST /api/libraries "{\"path\":\"$W/media/Фильмы\",\"kind\":\"movies\"}"
post POST /api/libraries "{\"path\":\"$W/media/Сериалы\",\"kind\":\"shows\"}"
sleep 6 # let the scan finish before matching starts
post PUT /api/settings '{"tmdbVia":"custom","tmdbApi":"http://127.0.0.1:9100/3","tmdbImg":"http://127.0.0.1:9100/t/p","tmdbKey":"fakekey1234567890","setupDone":"1"}'
post PUT /api/indexers '{"indexers":[{"name":"Jackett","url":"http://127.0.0.1:9100/torznab","apiKey":"","on":true}],"prefs":{"quality":"1080p","language":"rus","minSeeds":3}}'
sleep 6
for a in "4 26" "11 14" "3 31"; do set -- $a; post POST /api/items/$1/progress "{\"position\":$2,\"duration\":40,\"device\":\"pc\"}"; done

mkdir -p shots
node "$HERE/shot2.js" '[["light","/","home"],["dark","/","home-dark"],["light","/catalog/movie/105","title"],["dark","/catalog/movie/106","title-dark"],["light","/library/movies","movies"],["dark","/show/2","show-dark"],["light","/person/301","person"],["light","/settings","settings"],
["light","/torrents","torrents-search","(async()=>{await p.getByText(\"Поиск\",{exact:true}).first().click();await p.waitForTimeout(500);const ok=p.getByText(\"Понимаю, продолжить\");if(await ok.count())await ok.click();const i=p.getByPlaceholder(\"Название фильма или сериала\");await i.fill(\"Затмение\");await i.press(\"Enter\");await p.waitForTimeout(3000)})()"],
["dark","/play/3","player","(async()=>{await p.evaluate(()=>{const v=document.querySelector(\"video\");v.currentTime=4;v.pause()});await p.waitForTimeout(1500);await p.mouse.move(710,520);await p.waitForTimeout(400)})()"]]'
node "$HERE/banner.js"
node "$HERE/rec.js"
ffmpeg -v error -y -ss 0.8 -i vid/*.webm -vf "fps=10,scale=800:-1:flags=lanczos,hqdn3d=4:4:6:6,split[a][b];[a]palettegen=max_colors=160:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" "$ROOT/docs/media/demo.gif"
python3 - "$ROOT/docs/media" <<'PY'
import sys
from PIL import Image
out = sys.argv[1] + '/'
m = {'banner': 'banner', 'home': 'home-light', 'home-dark': 'home-dark', 'title': 'title-light', 'title-dark': 'title-dark', 'show-dark': 'show-dark',
     'movies': 'library-light', 'player': 'player', 'person': 'person-light', 'torrents-search': 'torrents-light', 'settings': 'settings-light'}
for src, dst in m.items():
    im = Image.open(f'shots/{src}.png').convert('RGB')
    w = 1600 if src == 'banner' else 1920
    im.resize((w, round(im.height * w / im.width)), Image.LANCZOS).save(out + dst + '.jpg', quality=87, optimize=True, progressive=True)
PY
echo "done: $ROOT/docs/media"
