package torrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
func queryJacRed(ctx context.Context, client *http.Client, in Indexer, query string) ([]Result, error) {
	hosts := []string{in.URL}
	if strings.TrimRight(in.URL, "/") == JacRedURL {
		hosts = append(hosts, JacRedMirrors...)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type answer struct {
		res []Result
		err error
	}
	ch := make(chan answer, len(hosts))
	for _, h := range hosts {
		go func(h string) {
			res, err := jacRedFrom(ctx, client, in, h, query)
			ch <- answer{res, err}
		}(h)
	}
	var fails []string
	for range hosts {
		a := <-ch
		if a.err == nil {
			return a.res, nil
		}
		fails = append(fails, a.err.Error())
	}
	return nil, errors.New(strings.Join(fails, "; "))
}

func jacRedFrom(ctx context.Context, client *http.Client, in Indexer, host, query string) ([]Result, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(host), "/") + "/api/v2.0/indexers/all/results")
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("неверный адрес %q", host)
	}
	q := u.Query()
	q.Set("apikey", in.APIKey)
	q.Set("Query", query)
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	req.Header.Set("Accept", "application/json")
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
