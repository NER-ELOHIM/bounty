package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Sherlock lists audit contests: a protocol posts a prize pool, and whoever
// finds valid vulnerabilities in the published code shares it.
//
// Investigated on 2026-10-06, when the issue-bounty platforms turned out to be
// empty (Algora's listing answers 404; Opire's public feed held five rewards in
// total, the largest $100 on an unknown repository). Contests are where reward
// money still moves, and Sherlock is the one platform found with a public JSON
// feed and a robots.txt that allows reading it:
//
//	mainnet-contest.sherlock.xyz/contests?status=RUNNING  → live contests
//	mainnet-contest.sherlock.xyz/contests?status=CREATED  → announced ones
//
// Not implemented, so nobody repeats the walk: CodeHawks renders its list on
// the client and its /api/contests answers 400 without a session; Code4rena has
// no public listing route; Immunefi's bounties.json is public but describes
// standing bug-bounty programmes (232 of them, 6 MB), not dated work.
//
// Contests are rare — the feed showed three between June and October 2026 — so
// most runs return nothing from here, and that is the correct answer.
type Sherlock struct {
	Client *http.Client
	Base   string // overridden in tests
}

// NewSherlock builds a client with an explicit timeout on every request.
func NewSherlock(timeout time.Duration) *Sherlock {
	return &Sherlock{
		Client: &http.Client{Timeout: timeout},
		Base:   "https://mainnet-contest.sherlock.xyz",
	}
}

// Contest is one public audit contest.
type Contest struct {
	ID        int
	Title     string
	Summary   string
	Kind      string // the platform's label, e.g. "Public Bug Bounty"
	PrizePool int    // in units of Token
	Token     string
	StartsAt  time.Time
	EndsAt    time.Time
}

// URL is the contest's public page.
func (c Contest) URL() string {
	return fmt.Sprintf("https://audits.sherlock.xyz/contests/%d", c.ID)
}

// Contests returns the public contests in one status ("RUNNING", "CREATED").
// Private contests are left out: they are invitation-only.
func (s *Sherlock) Contests(ctx context.Context, status string) ([]Contest, error) {
	q := url.Values{}
	q.Set("status", status)
	q.Set("per_page", "100")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Base+"/contests?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "iode-adapter-bounty/2.0")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying Sherlock: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sherlock returned HTTP %d", resp.StatusCode)
	}

	var body struct {
		Items []struct {
			ID        int    `json:"id"`
			Title     string `json:"title"`
			Summary   string `json:"short_description"`
			Kind      string `json:"type_label"`
			Private   bool   `json:"private"`
			PrizePool int    `json:"prize_pool"`
			Token     string `json:"token"`
			StartsAt  int64  `json:"starts_at"`
			EndsAt    int64  `json:"ends_at"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("parsing the Sherlock response: %w", err)
	}

	out := make([]Contest, 0, len(body.Items))
	for _, it := range body.Items {
		if it.Private {
			continue
		}
		out = append(out, Contest{
			ID:        it.ID,
			Title:     it.Title,
			Summary:   it.Summary,
			Kind:      it.Kind,
			PrizePool: it.PrizePool,
			Token:     it.Token,
			StartsAt:  time.Unix(it.StartsAt, 0).UTC(),
			EndsAt:    time.Unix(it.EndsAt, 0).UTC(),
		})
	}
	return out, nil
}
