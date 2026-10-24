"""Draws the fictional posters, backdrops and portraits into img/."""
import os, sys
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from art import poster, backdrop, portrait
from data import MOVIES, SHOWS, PEOPLE, PALS

out = os.path.join(HERE, "img")
os.makedirs(out, exist_ok=True)
for t in MOVIES + SHOWS:
    id, ru, orig, year, kind, pal, style = t[:7]
    poster(f"{out}/p{id}.jpg", ru, orig, t[9], PALS[pal], kind, id, style)
    backdrop(f"{out}/b{id}.jpg", PALS[pal], kind, id)
for p in PEOPLE:
    portrait(f"{out}/h{p[0]}.jpg", PALS[p[3]], p[0], p[2])
