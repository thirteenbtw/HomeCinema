package tmdb

import (
	"context"
	"net/url"
	"strconv"
)

// FindMovie finds the TMDB film for a title and release year, as Letterboxd
// lists them. TMDB search also matches alternative titles, so the English
// name Letterboxd uses finds a film whose original title differs. ok is false
// when nothing released within a year of it turns up.
func (c *Client) FindMovie(ctx context.Context, name string, year int) (CatalogItem, bool, error) {
	search := func(y int) ([]CatalogItem, error) {
		q := url.Values{"query": {name}, "language": {c.Lang()}, "include_adult": {"false"}}
		if y > 0 {
			q.Set("year", strconv.Itoa(y))
		}
		var r listResp
		if err := c.getCached(ctx, "/search/movie", q, &r); err != nil {
			return nil, err
		}
		return toPage(r, "movie").Items, nil
	}
	items, err := search(year)
	if err != nil {
		return CatalogItem{}, false, err
	}
	if year == 0 {
		if len(items) > 0 {
			return items[0], true, nil
		}
		return CatalogItem{}, false, nil
	}
	for _, it := range items {
		if it.Year == year {
			return it, true, nil
		}
	}
	if len(items) == 0 {
		// TMDB's "year" means the primary release; festival years differ
		if items, err = search(0); err != nil {
			return CatalogItem{}, false, err
		}
	}
	for _, it := range items {
		if it.Year >= year-1 && it.Year <= year+1 {
			return it, true, nil
		}
	}
	return CatalogItem{}, false, nil
}
