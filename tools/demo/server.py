"""Fake TMDB + Torznab for README screenshots. Everything is fictional."""
import json, os, re, sys, hashlib
from http.server import ThreadingHTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs
sys.path.insert(0, os.path.dirname(__file__))
from data import MOVIES, SHOWS, GENRES, PEOPLE

IMG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "img")
GN = {k: dict(v) for k, v in GENRES.items()}


def item(t, kind, media=False):
    id, ru, orig, year, _, _, _, genres, rating, tag, ov = t[:11]
    d = {"id": id, "poster_path": f"/p{id}.jpg", "backdrop_path": f"/b{id}.jpg", "overview": ov,
         "vote_average": rating, "vote_count": 1200 + id * 7, "genre_ids": genres, "popularity": 100 - id % 100}
    if kind == "movie":
        d.update(title=ru, original_title=orig, release_date=f"{year}-0{1 + id % 9}-1{id % 9}")
    else:
        d.update(name=ru, original_name=orig, first_air_date=f"{year}-0{1 + id % 9}-0{1 + id % 8}")
    if media:
        d["media_type"] = kind
    return d


def rot(lst, n):
    n %= len(lst)
    return lst[n:] + lst[:n]


def page(results):
    return {"page": 1, "total_pages": 1, "total_results": len(results), "results": results}


def cast_for(id):
    people = rot(PEOPLE, id)
    actors = [p for p in people if p[4] == "Acting"][:7]
    directors = [p for p in people if p[4] == "Directing"][:1]
    roles = ["Вера", "Андрей", "Капитан Лесков", "Марина", "Старик", "Соня", "Голос по радио"]
    cast = [{"id": p[0], "name": p[1], "character": roles[i], "profile_path": f"/h{p[0]}.jpg", "order": i} for i, p in enumerate(actors)]
    crew = [{"id": p[0], "name": p[1], "job": "Director", "department": "Directing", "profile_path": f"/h{p[0]}.jpg"} for p in directors]
    return cast, crew


def details(kind, id):
    src = MOVIES if kind == "movie" else SHOWS
    t = next((x for x in src if x[0] == id), None)
    if not t:
        return None
    d = item(t, kind)
    d["genres"] = [{"id": g, "name": GN[kind].get(g, GN["movie"].get(g, "драма"))} for g in t[7]]
    d["tagline"] = t[9]
    cast, crew = cast_for(id)
    d["credits"] = {"cast": cast, "crew": crew}
    d["production_countries"] = [{"name": "Россия"}]
    d["external_ids"] = {"imdb_id": ""}
    d["videos"] = {"results": []}
    d["recommendations"] = page([item(x, kind) for x in rot(src, id)[1:9]])
    if kind == "movie":
        d["runtime"] = 95 + id % 40
        d["release_dates"] = {"results": [{"iso_3166_1": "RU", "release_dates": [{"certification": ["12+", "16+", "18+"][id % 3]}]}]}
    else:
        d["episode_run_time"] = [45]
        d["content_ratings"] = {"results": [{"iso_3166_1": "RU", "rating": "16+"}]}
        d["seasons"] = [{"season_number": n, "name": f"Сезон {n}", "episode_count": 8, "air_date": f"{t[3] + n - 1}-03-01"} for n in range(1, t[11] + 1)]
        d["number_of_seasons"] = t[11]
    return d


def person(id):
    p = next((x for x in PEOPLE if x[0] == id), None)
    if not p:
        return None
    cast, crew = [], []
    for kind, src in (("movie", MOVIES), ("tv", SHOWS)):
        for t in src:
            c, cr = cast_for(t[0])
            for x in c:
                if x["id"] == id:
                    cast.append({**item(t, kind, True), "character": x["character"], "order": x["order"]})
            for x in cr:
                if x["id"] == id:
                    crew.append({**item(t, kind, True), "job": "Director", "department": "Directing"})
    return {"id": id, "name": p[1], "profile_path": f"/h{id}.jpg", "biography": p[5], "birthday": p[6], "deathday": None,
            "place_of_birth": p[7], "known_for_department": p[4], "gender": 1 if p[2] else 2, "imdb_id": "",
            "combined_credits": {"cast": cast, "crew": crew}}


def match(t, q):
    q = q.lower()
    return q in t[1].lower() or q in t[2].lower()


TORRENTS = ["{ru} / {orig} ({y}) WEB-DL 2160p HDR | Дубляж", "{ru} / {orig} ({y}) WEB-DL 1080p | Дубляж, Original",
            "{ru} / {orig} ({y}) BDRip 1080p | MVO", "{ru} / {orig} ({y}) WEB-DLRip 720p | Дубляж"]


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def send(self, code, body, ctype="application/json"):
        if isinstance(body, (dict, list)):
            body = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        u = urlparse(self.path)
        q = {k: v[0] for k, v in parse_qs(u.query).items()}
        p = u.path
        m = re.match(r"^/t/p/\w+/(\w+\.jpg)$", p)
        if m:
            f = os.path.join(IMG, m.group(1))
            if os.path.exists(f):
                return self.send(200, open(f, "rb").read(), "image/jpeg")
            return self.send(404, b"no", "text/plain")
        if p.startswith("/torznab"):
            return self.torznab(q.get("q", ""))
        if not p.startswith("/3/"):
            return self.send(404, {"status_message": "nope"})
        p = p[2:]
        movies = [item(t, "movie") for t in MOVIES]
        shows = [item(t, "tv") for t in SHOWS]
        lists = {
            "/configuration": {"images": {}},
            "/trending/movie/week": page(rot(movies, 0)), "/movie/popular": page(rot(movies, 4)),
            "/movie/top_rated": page(sorted(movies, key=lambda x: -x["vote_average"])), "/movie/now_playing": page(rot(movies, 8)),
            "/movie/upcoming": page(rot(movies, 11)),
            "/trending/tv/week": page(shows), "/tv/popular": page(rot(shows, 2)), "/tv/top_rated": page(sorted(shows, key=lambda x: -x["vote_average"])),
            "/tv/on_the_air": page(rot(shows, 4)), "/tv/airing_today": page(rot(shows, 5)),
            "/genre/movie/list": {"genres": [{"id": i, "name": n} for i, n in GENRES["movie"]]},
            "/genre/tv/list": {"genres": [{"id": i, "name": n} for i, n in GENRES["tv"]]},
            "/person/popular": page([{"id": x[0], "name": x[1], "profile_path": f"/h{x[0]}.jpg", "known_for_department": x[4],
                                      "known_for": [{"title": MOVIES[i % len(MOVIES)][1]} for i in range(x[0], x[0] + 2)]} for x in PEOPLE]),
        }
        if p in lists:
            return self.send(200, lists[p])
        if p.startswith("/discover/"):
            kind = p.split("/")[2]
            src = movies if kind == "movie" else shows
            g = q.get("with_genres")
            res = [x for x in src if not g or int(g) in x["genre_ids"]] or src
            return self.send(200, page(res))
        if p == "/search/multi":
            s = q.get("query", "")
            return self.send(200, page([item(t, "movie", True) for t in MOVIES if match(t, s)] + [item(t, "tv", True) for t in SHOWS if match(t, s)]))
        if p in ("/search/movie", "/search/tv"):
            s = q.get("query", "")
            kind = p.split("/")[2]
            src = MOVIES if kind == "movie" else SHOWS
            return self.send(200, page([item(t, kind) for t in src if match(t, s)]))
        if p == "/search/person":
            s = q.get("query", "").lower()
            return self.send(200, page([{"id": x[0], "name": x[1], "profile_path": f"/h{x[0]}.jpg", "known_for_department": x[4], "known_for": []} for x in PEOPLE if s in x[1].lower()]))
        m = re.match(r"^/tv/(\d+)/season/(\d+)/episode/(\d+)$", p)
        if m:
            e = int(m.group(3))
            names = ["Пилот", "Сигнал", "Вторая смена", "Белая ночь", "Тихий час", "Точка невозврата", "Буря", "Рассвет"]
            return self.send(200, {"name": names[(e - 1) % 8], "overview": "Новая серия: герои оказываются ближе к разгадке, чем думали.",
                                   "still_path": f"/b{m.group(1)}.jpg", "vote_average": 8.0, "runtime": 45})
        m = re.match(r"^/(movie|tv|person)/(\d+)$", p)
        if m:
            d = person(int(m.group(2))) if m.group(1) == "person" else details(m.group(1), int(m.group(2)))
            if d:
                return self.send(200, d)
        return self.send(404, {"status_message": "not found"})

    def torznab(self, query):
        t = next((x for x in MOVIES + SHOWS if x[1].lower() in query.lower() or x[2].lower() in query.lower()), None)
        items = []
        if t:
            for i, f in enumerate(TORRENTS):
                title = f.format(ru=t[1], orig=t[2], y=t[3])
                h = hashlib.sha1(title.encode()).hexdigest()
                size = [21, 9, 12, 2][i] * 1024 ** 3 + 345 * 1024 ** 2
                items.append(f"""<item><title>{title}</title><size>{size}</size><pubDate>Mon, 0{i + 1} Sep 2025 10:00:00 +0000</pubDate>
<torznab:attr name="seeders" value="{[312, 1840, 655, 98][i]}"/><torznab:attr name="peers" value="{[40, 210, 80, 12][i]}"/>
<torznab:attr name="infohash" value="{h}"/></item>""")
        body = f"""<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>{''.join(items)}</channel></rss>"""
        self.send(200, body.encode(), "application/rss+xml")


ThreadingHTTPServer(("127.0.0.1", 9100), H).serve_forever()
