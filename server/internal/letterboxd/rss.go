package letterboxd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Site is letterboxd.com, replaced in tests.
var Site = "https://letterboxd.com"

var userRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)

// User takes a username or a profile link ("letterboxd.com/name/") and
// returns the bare username, or "" when it is neither.
func User(s string) string {
	s = strings.TrimSpace(s)
	if u, err := url.Parse(s); err == nil && strings.Contains(u.Host, "letterboxd.com") {
		s = strings.Split(strings.Trim(u.Path, "/"), "/")[0]
	} else if i := strings.Index(s, "letterboxd.com/"); i >= 0 {
		s = strings.Split(strings.Trim(s[i+len("letterboxd.com/"):], "/"), "/")[0]
	}
	s = strings.TrimPrefix(s, "@")
	if !userRe.MatchString(s) {
		return ""
	}
	return s
}

// FetchRSS loads a member's public feed: their last 50 or so diary entries.
func FetchRSS(ctx context.Context, client *http.Client, user string) ([]Entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Site+"/"+url.PathEscape(user)+"/rss/", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) HomeCinema")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("letterboxd.com не отвечает: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("на Letterboxd нет пользователя %q", user)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("letterboxd.com ответил %s", resp.Status)
	}
	return ParseRSS(io.LimitReader(resp.Body, 16<<20))
}
