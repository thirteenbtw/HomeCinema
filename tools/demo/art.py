"""Procedural cinematic artwork: posters, backdrops, portraits."""
import math, random
import numpy as np
from PIL import Image, ImageDraw, ImageFilter, ImageFont

FD = "/usr/share/fonts/opentype/inter/"
FONTS = {
    "black": FD + "InterDisplay-Black.otf",
    "bold": FD + "InterDisplay-Bold.otf",
    "light": FD + "InterDisplay-Light.otf",
    "thin": FD + "InterDisplay-Thin.otf",
    "serif": "/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf",
    "serif2": "/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
}


def font(kind, size):
    try:
        return ImageFont.truetype(FONTS[kind], size)
    except OSError:
        return ImageFont.truetype(FONTS["bold"], size)


def hexrgb(h):
    h = h.lstrip("#")
    return np.array([int(h[i:i + 2], 16) for i in (0, 2, 4)], dtype=np.float32)


def noise1d(n, rng, octaves=5, rough=0.5):
    out = np.zeros(n)
    amp, freq = 1.0, 3
    for _ in range(octaves):
        pts = rng.uniform(-1, 1, freq + 2)
        xs = np.linspace(0, freq + 1, n)
        out += amp * np.interp(xs, np.arange(freq + 2), pts)
        amp *= rough
        freq *= 2
    return out


def scene(W, H, pal, kind, seed):
    """Returns an RGB float array HxWx3 of a cinematic scene."""
    rng = np.random.default_rng(seed)
    sky_top, sky_bot, sun, land, accent = (hexrgb(c) for c in pal)
    y = np.linspace(0, 1, H)[:, None, None]
    img = sky_top * (1 - y) + sky_bot * y
    img = np.broadcast_to(img, (H, W, 3)).copy()
    yy, xx = np.mgrid[0:H, 0:W].astype(np.float32)
    s = min(W, H)
    horizon = H * rng.uniform(0.55, 0.68)

    # sun / moon with glow
    cx = W * rng.uniform(0.3, 0.7)
    cy = horizon - s * rng.uniform(0.12, 0.3)
    r = s * rng.uniform(0.11, 0.2)
    d = np.sqrt((xx - cx) ** 2 + (yy - cy) ** 2)
    glow = np.exp(-((d / (r * 3.2)) ** 2))[..., None]
    img = img * (1 - 0.65 * glow) + sun * 0.65 * glow
    disc = np.clip((r - d) / 2.0, 0, 1)[..., None]
    if kind == "eclipse":
        ring = np.clip(1 - np.abs(d - r) / 3.0, 0, 1)[..., None]
        img = img * (1 - disc) + (sky_top * 0.4) * disc
        img = img * (1 - ring) + sun * ring
    else:
        img = img * (1 - disc) + sun * disc
        # bands across the sun
        if kind in ("retro", "city"):
            for k in range(5):
                by = cy + r * (0.15 + k * 0.17)
                band = (np.abs(yy - by) < r * 0.025 * (k + 1)) & (d < r)
                img[band] = sky_bot

    def ridge(base, amp, color, rough=0.55, fog=0.0):
        nonlocal img
        line = base + amp * noise1d(W, rng, rough=rough)
        mask = (yy > line[None, :]).astype(np.float32)[..., None]
        depth = np.clip((yy - line[None, :]) / (H * 0.5), 0, 1)[..., None]
        col = color * (1 - 0.35 * depth)
        if fog:
            col = col * (1 - fog) + sky_bot * fog
        img = img * (1 - mask) + col * mask

    if kind in ("mountains", "eclipse", "retro"):
        ridge(horizon - s * 0.10, s * 0.10, land * 0.9 + sky_bot * 0.1, fog=0.45)
        ridge(horizon, s * 0.07, land * 0.7, fog=0.2)
        ridge(horizon + s * 0.12, s * 0.06, land * 0.35)
    elif kind == "city":
        x = 0
        while x < W:
            bw = rng.integers(int(s * 0.04), int(s * 0.12))
            bh = rng.uniform(0.08, 0.4) * H
            top = horizon + s * 0.05 - bh
            col = land * rng.uniform(0.35, 0.6)
            img[int(top):, x:x + bw] = col
            for wy in range(int(top) + 6, H, 9):
                for wx in range(x + 4, x + bw - 4, 7):
                    if rng.random() < 0.18:
                        img[wy:wy + 3, wx:wx + 3] = accent
            x += bw + rng.integers(0, 6)
        ridge(horizon + s * 0.22, s * 0.01, land * 0.15)
    elif kind == "sea":
        sea = yy > horizon
        t = np.clip((yy - horizon) / (H - horizon), 0, 1)[..., None]
        water = sky_bot * 0.55 * (1 - t) + land * 0.4 * t
        img = np.where(sea[..., None], water, img)
        # sun reflection
        refl = sea & (np.abs(xx - cx) < r * (0.4 + 1.5 * (yy - horizon) / H)) & (np.sin(yy * 0.9) > 0.2)
        img[refl] = img[refl] * 0.4 + sun * 0.6
    elif kind == "forest":
        ridge(horizon, s * 0.04, land * 0.6, fog=0.4)
        for layer, (n, sc, shade) in enumerate([(26, 0.22, 0.45), (16, 0.34, 0.25), (9, 0.5, 0.1)]):
            for _ in range(n):
                tx = rng.uniform(-0.05, 1.05) * W
                th = s * sc * rng.uniform(0.7, 1.3)
                base = horizon + s * (0.05 + 0.12 * layer)
                tri = (yy < base) & (yy > base - th) & (np.abs(xx - tx) < (yy - (base - th)) * 0.28)
                img[tri] = land * shade + sky_bot * 0.1
            img[(yy > horizon + s * (0.05 + 0.12 * layer))] = land * shade * 0.8
    elif kind == "desert":
        for k, f in enumerate([0.0, 0.08, 0.18]):
            line = horizon + s * f + s * 0.05 * np.sin(np.linspace(0, rng.uniform(2, 5), W) + rng.uniform(0, 6))
            mask = (yy > line[None, :])[..., None]
            img = np.where(mask, land * (0.85 - 0.25 * k) + sun * 0.1, img)

    # figure silhouette
    if rng.random() < 0.65 and kind != "city":
        fx = W * rng.uniform(0.25, 0.75)
        fh = s * rng.uniform(0.12, 0.2)
        fb = horizon + s * rng.uniform(0.08, 0.16)
        head = ((xx - fx) ** 2 + (yy - (fb - fh * 0.88)) ** 2) < (fh * 0.09) ** 2
        body = (yy > fb - fh * 0.78) & (yy < fb) & (np.abs(xx - fx) < fh * (0.12 + 0.1 * (yy - (fb - fh * 0.78)) / fh))
        sil = head | body
        img[sil] = sky_top * 0.12

    # light leak
    lx, ly = rng.uniform(0, W), rng.uniform(0, H * 0.4)
    leak = np.exp(-(((xx - lx) ** 2 + (yy - ly) ** 2) / (2 * (s * 0.5) ** 2)))[..., None]
    img = img + accent * 0.18 * leak
    # vignette + grain
    vd = np.sqrt(((xx / W - 0.5) * 1.3) ** 2 + ((yy / H - 0.5) * 1.1) ** 2)
    img *= (1 - 0.55 * np.clip(vd - 0.25, 0, 1))[..., None]
    img += rng.normal(0, 5, (H, W, 1))
    return np.clip(img, 0, 255)


def _fit(draw, text, kind, maxw, size):
    while size > 10:
        f = font(kind, size)
        if draw.textlength(text, font=f) <= maxw:
            return f
        size -= 2
    return font(kind, size)


def poster(path, title, orig, tagline, pal, kind, seed, style=0, W=500, H=750):
    im = Image.fromarray(scene(W, H, pal, kind, seed).astype(np.uint8))
    # darken bottom for text
    grad = Image.linear_gradient("L").resize((W, H))
    shade = Image.new("RGB", (W, H), (8, 10, 18))
    im = Image.composite(shade, im, grad.point(lambda v: max(0, v - 120) * 1.6))
    d = ImageDraw.Draw(im)
    accent = tuple(int(v) for v in hexrgb(pal[4]))
    words = title.upper() if style != 2 else title
    kindf = ["black", "light", "serif"][style % 3]
    lines = words.split("\n") if "\n" in words else _wrap(d, words, kindf, W - 70, 76)
    y = H - 70 - len(lines) * 66 - (22 if style == 2 else 0)
    for ln in lines:
        f = _fit(d, ln, kindf, W - 60, 76)
        tw = d.textlength(ln, font=f)
        d.text(((W - tw) / 2, y), ln, font=f, fill=(245, 244, 240))
        y += f.size + 4
    ft = font("bold", 13)
    t = orig.upper()
    d.text(((W - d.textlength(t, font=ft)) / 2, H - 52), t, font=ft, fill=accent)
    fl = font("light", 15)
    d.text(((W - d.textlength(tagline, font=fl)) / 2, 34), tagline, font=fl, fill=(235, 235, 235))
    fb = font("light", 9)
    credits = "РЕЖИССЁР  ·  ОПЕРАТОР  ·  КОМПОЗИТОР  ·  ПРОДЮСЕРЫ  ·  В РОЛЯХ"
    d.text(((W - d.textlength(credits, font=fb)) / 2, H - 28), credits, font=fb, fill=(150, 150, 160))
    im.save(path, quality=88)


def _wrap(d, text, kindf, maxw, size):
    words = text.split()
    if len(words) <= 1:
        return [text]
    f = font(kindf, size)
    if d.textlength(text, font=f) <= maxw:
        return [text]
    best = None
    for i in range(1, len(words)):
        a, b = " ".join(words[:i]), " ".join(words[i:])
        w = max(d.textlength(a, font=f), d.textlength(b, font=f))
        if best is None or w < best[0]:
            best = (w, [a, b])
    return best[1]


def backdrop(path, pal, kind, seed, W=1280, H=720):
    Image.fromarray(scene(W, H, pal, kind, seed).astype(np.uint8)).save(path, quality=88)


def portrait(path, pal, seed, female=False, W=300, H=450):
    rng = random.Random(seed)
    a, b = hexrgb(pal[0]), hexrgb(pal[1])
    y = np.linspace(0, 1, H)[:, None, None]
    img = np.broadcast_to(a * (1 - y) + b * y, (H, W, 3)).copy()
    yy, xx = np.mgrid[0:H, 0:W].astype(np.float32)
    cx = W * rng.uniform(0.45, 0.55)
    skin = hexrgb(rng.choice(["#c99a7c", "#b88466", "#d8b096", "#8d5d43", "#e2c0a5"]))
    hair = hexrgb(rng.choice(["#1d1612", "#3b2a1e", "#6b4a2b", "#a07a4a", "#2a2a2e", "#c9c4bc"]))
    shirt = hexrgb(rng.choice(["#1c2233", "#2d2d2d", "#41352c", "#e8e4dc", "#28403a", "#5b2b2b"]))
    hr = W * 0.2
    hy = H * 0.4
    # shoulders
    sh = ((xx - cx) / (W * 0.52)) ** 2 + ((yy - H * 1.02) / (H * 0.36)) ** 2 < 1
    img[sh] = shirt
    neck = (np.abs(xx - cx) < hr * 0.42) & (yy > hy) & (yy < H * 0.72)
    img[neck] = skin * 0.8
    # hair behind (long for some)
    if female:
        hb = ((((xx - cx) / (hr * 1.42)) ** 2 + ((yy - hy) / (hr * 1.5)) ** 2) < 1) | ((np.abs(xx - cx) < hr * 1.3) & (yy > hy) & (yy < hy + hr * 1.9))
        img[hb] = hair
    face = (((xx - cx) / hr) ** 2 + ((yy - hy) / (hr * 1.28)) ** 2) < 1
    # side lighting on face
    light = np.clip(0.75 + 0.35 * (xx - cx) / hr, 0.55, 1.1)[..., None]
    img = np.where(face[..., None], skin * light, img)
    top = (((xx - cx) / (hr * 1.08)) ** 2 + ((yy - (hy - hr * 0.55)) / (hr * 0.85)) ** 2 < 1) & (yy < hy - hr * 0.35)
    img[top] = hair
    # eyes hint
    for ex in (cx - hr * 0.38, cx + hr * 0.38):
        eye = ((xx - ex) / (hr * 0.13)) ** 2 + ((yy - (hy - hr * 0.05)) / (hr * 0.06)) ** 2 < 1
        img[eye] = skin * 0.35
    rim = np.exp(-(((xx - W) ** 2) / (2 * (W * 0.25) ** 2)))[..., None]
    img = img + hexrgb(pal[2]) * 0.15 * rim
    img += np.random.default_rng(seed).normal(0, 4, (H, W, 1))
    im = Image.fromarray(np.clip(img, 0, 255).astype(np.uint8)).filter(ImageFilter.GaussianBlur(1.2))
    im.save(path, quality=88)
