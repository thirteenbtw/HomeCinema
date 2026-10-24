package torrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// JacRed is a public parser, the one Lampa uses by default: it indexes the
// Russian trackers ahead of time, so search works without Jackett or tracker
// accounts. It answers with Jackett's JSON results API.
const KindJacRed = "jacred"

// JacRedURL is the public instance added for new users; JacRedMirrors are
// tried when it does not answer.
var JacRedURL = "https://jac.red"

var JacRedMirrors = []string{"https://jacred.su", "https://jacred.xyz"}

func defaultIndexers() []Indexer {
	return []Indexer{{Name: "JacRed", URL: JacRedURL, Kind: KindJacRed, On: true}}
}

// isJacRedSite reports a public JacRed instance entered as a Torznab source
// by its bare address, where only its JSON API answers.
func isJacRedSite(in Indexer) bool {
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || strings.Trim(u.Path, "/") != "" {
		return false
	}
	for _, h := range append([]string{JacRedURL}, JacRedMirrors...) {
		if hu, err := url.Parse(h); err == nil && strings.EqualFold(hu.Host, u.Host) {
			return true
		}
	}
	return false
}

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

func nonEmpty(list []string) []string {
	var out []string
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

type jackettResults struct {
	Results []struct {
		Tracker     string `json:"Tracker"`
		Title       string `json:"Title"`
		Link        string `json:"Link"`
		MagnetUri   string `json:"MagnetUri"`
		Size        int64  `json:"Size"`
		Seeders     int    `json:"Seeders"`
		Peers       int    `json:"Peers"`
		PublishDate string `json:"PublishDate"`
	} `json:"Results"`
}

// queryJacRed asks the instance and, for the default one, its mirrors all at
// once: public instances come and go, and a dead one should cost nothing.
// The first answer with releases wins.
func queryJacRed(ctx context.Context, client *http.Client, in Indexer, query string, tq *TitleQuery) ([]Result, error) {
	hosts := []string{in.URL}
	if strings.TrimRight(in.URL, "/") == JacRedURL {
		hosts = append(hosts, JacRedMirrors...)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type answer struct {
		host string
		res  []Result
		err  error
	}
	ch := make(chan answer, len(hosts))
	for _, h := range hosts {
		go func(h string) {
			res, err := jacRedFrom(ctx, client, in, h, query, tq)
			ch <- answer{h, res, err}
		}(h)
	}
	// An instance can answer quickly with nothing (an empty or outdated
	// index), so an empty answer waits for the others.
	var empty, fails []string
	for range hosts {
		a := <-ch
		switch {
		case a.err == nil && len(a.res) > 0:
			return a.res, nil
		case a.err == nil:
			u, _ := url.Parse(a.host)
			empty = append(empty, u.Host+": ничего не нашёл")
		default:
			fails = append(fails, a.err.Error())
		}
	}
	if len(fails) == 0 {
		return []Result{}, nil
	}
	return nil, errors.New(strings.Join(append(empty, fails...), "; "))
}

func jacRedFrom(ctx context.Context, client *http.Client, in Indexer, host, query string, tq *TitleQuery) ([]Result, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(host), "/") + "/api/v2.0/indexers/all/results")
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("неверный адрес %q", host)
	}
	q := u.Query()
	q.Set("apikey", in.APIKey)
	if tq != nil {
		// the way Lampa asks: the bare title plus the card's fields
		if tq.Year > 0 {
			query = strings.TrimSuffix(query, " "+strconv.Itoa(tq.Year))
			q.Set("year", strconv.Itoa(tq.Year))
		}
		names := nonEmpty(tq.Names)
		if len(names) > 0 {
			q.Set("title", names[0])
			q.Set("title_original", names[len(names)-1])
		}
		if tq.Show {
			q.Set("is_serial", "2")
		} else {
			q.Set("is_serial", "1")
		}
	}
	q.Set("Query", query)
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	req.Header.Set("Accept", "application/json")
	// public instances sit behind bot filters that turn away Go's default agent
	req.Header.Set("User-Agent", browserUA)
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%s: %v", u.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", u.Host, resp.StatusCode)
	}
	var body jackettResults
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%s отвечает не как JacRed", u.Host)
	}
	out := make([]Result, 0, len(body.Results))
	for _, it := range body.Results {
		r := Result{Indexer: in.Name, Title: it.Title, Link: it.MagnetUri, Size: it.Size, Seeds: it.Seeders, Peers: it.Peers, Date: it.PublishDate}
		if it.Tracker != "" {
			r.Indexer = in.Name + " · " + it.Tracker
		}
		if r.Link == "" {
			r.Link = it.Link
		}
		if r.Link == "" {
			continue
		}
		r.Quality = detectQuality(r.Title)
		r.Lang = detectLang(r.Title)
		out = append(out, r)
	}
	return out, nil
}
