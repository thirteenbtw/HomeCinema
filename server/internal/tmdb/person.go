package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// People: a person's page (photo, biography, what they are known for and
// the whole filmography) and the lists that lead to it.

type PersonCard struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Photo    string `json:"photo"`
	KnownFor string `json:"knownFor"`
}

type PersonCredit struct {
	CatalogItem
	Role  string   `json:"role"`  // character or job
	Dept  string   `json:"dept"`  // acting | directing | other
	Depts []string `json:"depts"` // every department of this title's credits
	Date  string   `json:"date"`  // release / first air date
	Order int      `json:"order"` // billing order for acting credits
}

type PersonPage struct {
	ID       int64          `json:"id"`
	Name     string         `json:"name"`
	Photo    string         `json:"photo"`
	Backdrop string         `json:"backdrop"`
	Bio      string         `json:"bio"`
	Birthday string         `json:"birthday"`
	Deathday string         `json:"deathday"`
	Place    string         `json:"place"`
	Dept     string         `json:"dept"`
	Gender   int            `json:"gender"` // TMDB: 1 female, 2 male, 0/3 unknown
	ImdbID   string         `json:"imdbId"`
	KnownFor []CatalogItem  `json:"knownFor"`
	Upcoming []PersonCredit `json:"upcoming"`
	Credits  []PersonCredit `json:"credits"`
}

type personCredit struct {
	listItem
	Character string `json:"character"`
	Job       string `json:"job"`
	Dept      string `json:"department"`
	Order     int    `json:"order"`
}

// Person loads a person with their combined movie and TV credits.
func (c *Client) Person(ctx context.Context, id int64) (PersonPage, error) {
	var d struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Profile  string `json:"profile_path"`
		Bio      string `json:"biography"`
		Birthday string `json:"birthday"`
		Deathday string `json:"deathday"`
		Place    string `json:"place_of_birth"`
		Dept     string `json:"known_for_department"`
		Gender   int    `json:"gender"`
		ImdbID   string `json:"imdb_id"`
		Credits  struct {
			Cast []personCredit `json:"cast"`
			Crew []personCredit `json:"crew"`
		} `json:"combined_credits"`
	}
	if err := c.getCached(ctx, fmt.Sprintf("/person/%d", id), url.Values{"language": {c.Lang()}, "append_to_response": {"combined_credits"}}, &d); err != nil {
		return PersonPage{}, err
	}
	if d.Bio == "" && !strings.HasPrefix(c.Lang(), "en") {
		var en struct {
			Bio string `json:"biography"`
		}
		if c.getCached(ctx, fmt.Sprintf("/person/%d", id), url.Values{"language": {"en-US"}}, &en) == nil {
			d.Bio = en.Bio
		}
	}
	p := PersonPage{ID: d.ID, Name: d.Name, Photo: ImageURL("w500", d.Profile), Bio: d.Bio, Birthday: d.Birthday, Deathday: d.Deathday,
		Place: d.Place, Dept: d.Dept, Gender: d.Gender, ImdbID: d.ImdbID, KnownFor: []CatalogItem{}, Upcoming: []PersonCredit{}, Credits: []PersonCredit{}}

	today := time.Now().Format("2006-01-02")
	seen := map[string]int{} // one entry per title, acting first
	add := func(pc personCredit, dept string) {
		if pc.Adult || (pc.MediaType != "movie" && pc.MediaType != "tv") {
			return
		}
		it := pc.listItem.toItem(pc.MediaType)
		date := pc.Release
		if pc.MediaType == "tv" {
			date = pc.FirstAir
		}
		role := pc.Character
		if dept != "acting" {
			role = jobName(pc.Job, c.Lang())
		}
		key := it.Kind + strconv.FormatInt(it.ID, 10)
		if i, ok := seen[key]; ok {
			if !slices.Contains(p.Credits[i].Depts, dept) {
				p.Credits[i].Depts = append(p.Credits[i].Depts, dept)
			}
			if role != "" && !strings.Contains(p.Credits[i].Role, role) {
				p.Credits[i].Role = strings.TrimPrefix(p.Credits[i].Role+", "+role, ", ")
			}
			return
		}
		seen[key] = len(p.Credits)
		p.Credits = append(p.Credits, PersonCredit{CatalogItem: it, Role: role, Dept: dept, Depts: []string{dept}, Date: date, Order: pc.Order})
	}
	for _, cr := range d.Credits.Cast {
		add(cr, "acting")
	}
	for _, cr := range d.Credits.Crew {
		dept := "other"
		if cr.Job == "Director" {
			dept = "directing"
		}
		add(cr, dept)
	}

	// Known for: the most voted titles; the hero uses the first one's still.
	byVotes := append([]PersonCredit(nil), p.Credits...)
	sort.SliceStable(byVotes, func(i, j int) bool { return byVotes[i].Votes > byVotes[j].Votes })
	for _, cr := range byVotes {
		if len(p.KnownFor) >= 12 {
			break
		}
		if cr.Date != "" && cr.Date <= today && (cr.Dept != "acting" || cr.Order < 15) {
			p.KnownFor = append(p.KnownFor, cr.CatalogItem)
		}
	}
	for _, it := range p.KnownFor {
		if it.Backdrop != "" {
			p.Backdrop = strings.Replace(it.Backdrop, "/w780/", "/w1280/", 1)
			break
		}
	}
	for _, cr := range p.Credits {
		if cr.Date == "" || cr.Date > today {
			p.Upcoming = append(p.Upcoming, cr)
		}
	}
	sort.SliceStable(p.Upcoming, func(i, j int) bool { return dateOrMax(p.Upcoming[i].Date) < dateOrMax(p.Upcoming[j].Date) })
	return p, nil
}

func dateOrMax(d string) string {
	if d == "" {
		return "9999"
	}
	return d
}

type personList struct {
	Page    int `json:"page"`
	Pages   int `json:"total_pages"`
	Results []struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Profile  string `json:"profile_path"`
		Adult    bool   `json:"adult"`
		Dept     string `json:"known_for_department"`
		KnownFor []struct {
			Title string `json:"title"`
			Name  string `json:"name"`
		} `json:"known_for"`
	} `json:"results"`
}

// People lists popular people, or finds people by name when query is set.
func (c *Client) People(ctx context.Context, query string, page int) ([]PersonCard, int, error) {
	vals := url.Values{"language": {c.Lang()}, "page": {strconv.Itoa(max(page, 1))}, "include_adult": {"false"}}
	path := "/person/popular"
	if query != "" {
		path = "/search/person"
		vals.Set("query", query)
	}
	var r personList
	if err := c.getCached(ctx, path, vals, &r); err != nil {
		return nil, 0, err
	}
	out := []PersonCard{}
	for _, x := range r.Results {
		if x.Adult {
			continue
		}
		var titles []string
		for _, k := range x.KnownFor {
			if t := k.Title + k.Name; t != "" && len(titles) < 2 {
				titles = append(titles, t)
			}
		}
		out = append(out, PersonCard{ID: x.ID, Name: x.Name, Photo: ImageURL("w342", x.Profile), KnownFor: strings.Join(titles, ", ")})
	}
	return out, min(r.Pages, 500), nil
}

type videoList struct {
	Results []struct {
		Key      string `json:"key"`
		Site     string `json:"site"`
		Type     string `json:"type"`
		Lang     string `json:"iso_639_1"`
		Official bool   `json:"official"`
	} `json:"results"`
}

func videoLangs(lang string) string {
	l := strings.SplitN(lang, "-", 2)[0]
	if l == "en" {
		return "en,null"
	}
	return l + ",en,null"
}

// trailer picks a YouTube trailer, preferring the UI language and official
// uploads, then teasers.
func (v videoList) trailer(lang string) string {
	l := strings.SplitN(lang, "-", 2)[0]
	best, bestScore := "", -1
	for _, x := range v.Results {
		if x.Site != "YouTube" || x.Key == "" {
			continue
		}
		score := 0
		switch x.Type {
		case "Trailer":
			score += 4
		case "Teaser":
			score += 2
		default:
			continue
		}
		if x.Lang == l {
			score += 3
		}
		if x.Official {
			score++
		}
		if score > bestScore {
			best, bestScore = x.Key, score
		}
	}
	return best
}

var jobsRu = map[string]string{
	"Director": "режиссёр", "Producer": "продюсер", "Executive Producer": "исполнительный продюсер", "Writer": "сценарист",
	"Screenplay": "сценарий", "Story": "автор сюжета", "Novel": "автор романа", "Original Music Composer": "композитор",
	"Director of Photography": "оператор", "Editor": "монтаж", "Creator": "создатель", "Characters": "автор персонажей",
}

// jobName translates the common crew jobs for the Russian interface.
func jobName(job, lang string) string {
	if strings.HasPrefix(lang, "ru") {
		if r, ok := jobsRu[job]; ok {
			return r
		}
	}
	return job
}
