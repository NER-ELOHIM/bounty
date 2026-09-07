// Package source finds open bounties on public platforms.
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SearchLimit is GitHub's ceiling for the Search API: 30 requests per minute
// with a token, 10 without. That is far tighter than the general API's limit,
// so one query per term per run is what fits.
const SearchLimit = 30

// Issue is one open issue carrying a bounty label.
type Issue struct {
	ID        int64
	Number    int
	Title     string
	Body      string
	HTMLURL   string
	Repo      string
	Author    string
	Labels    []string
	Comments  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GitHub queries the GitHub Search API for issues.
type GitHub struct {
	Client *http.Client
	Token  string // optional; without it the limit drops to 10 req/min
	Base   string // overridden in tests
}

// NewGitHub builds a client with an explicit timeout on every request.
func NewGitHub(token string, timeout time.Duration) *GitHub {
	return &GitHub{
		Client: &http.Client{Timeout: timeout},
		Token:  token,
		Base:   "https://api.github.com",
	}
}

type searchResponse struct {
	TotalCount        int  `json:"total_count"`
	IncompleteResults bool `json:"incomplete_results"`
	Items             []struct {
		ID      int64  `json:"id"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		RepoURL string `json:"repository_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Comments  int       `json:"comments"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"items"`
}

// Search runs one query. label and language may be empty; since filters by
// creation date and keeps the result set from growing without bound.
func (g *GitHub) Search(ctx context.Context, label, language string, since *time.Time, perPage int) ([]Issue, error) {
	terms := []string{"is:issue", "is:open"}
	if label != "" {
		terms = append(terms, `label:"`+label+`"`)
	}
	if language != "" {
		terms = append(terms, "language:"+language)
	}
	if since != nil {
		terms = append(terms, "created:>="+since.UTC().Format("2006-01-02"))
	}

	return g.search(ctx, strings.Join(terms, " "), perPage)
}

// search runs an already-composed query against the Search API.
//
// Composing the query is the caller's job, because every hunting strategy
// composes its own; what repeats — paging, headers, rate-limit handling,
// translating the response — lives here.
func (g *GitHub) search(ctx context.Context, query string, perPage int) ([]Issue, error) {
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("per_page", strconv.Itoa(perPage))
	q.Set("sort", "created")
	q.Set("order", "desc")

	endpoint := g.Base + "/search/issues?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "iode-adapter-bounty/1.0")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying GitHub: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body

	switch {
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		reset := resp.Header.Get("X-RateLimit-Reset")
		return nil, fmt.Errorf("rate limit reached (resets at %s); set IODE_GITHUB_TOKEN", reset)
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("token rejected by GitHub")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
	}

	var body searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("parsing the response: %w", err)
	}

	issues := make([]Issue, 0, len(body.Items))
	for _, it := range body.Items {
		labels := make([]string, 0, len(it.Labels))
		for _, l := range it.Labels {
			labels = append(labels, l.Name)
		}
		issues = append(issues, Issue{
			ID:        it.ID,
			Number:    it.Number,
			Title:     it.Title,
			Body:      it.Body,
			HTMLURL:   it.HTMLURL,
			Repo:      repoFromAPIURL(it.RepoURL),
			Author:    it.User.Login,
			Labels:    labels,
			Comments:  it.Comments,
			CreatedAt: it.CreatedAt,
			UpdatedAt: it.UpdatedAt,
		})
	}
	return issues, nil
}

// repoFromAPIURL turns https://api.github.com/repos/owner/name into owner/name.
func repoFromAPIURL(raw string) string {
	const marker = "/repos/"
	if i := strings.Index(raw, marker); i >= 0 {
		return raw[i+len(marker):]
	}
	return raw
}

// FASE 2: Algora como segunda fonte.
//
// Investigated on 2026-08-30 and NOT implemented: the public API does not
// answer the way the documentation describes. What was tried, so nobody repeats
// the walk:
//
//	api.algora.io/api/orgs/{org}/bounties  → 301 to algora.io, which 404s
//	algora.io/api/orgs/{org}/bounties      → 404 across five organisations
//	api.algora.io/bounties                 → 406 with Accept: application/json,
//	                                         404 without — inconsistent
//
// Closed on 2026-09-07 by reading Algora's own source: the live route is
// /api/trpc/bounty.list, and its controller renders an empty list
// unconditionally. There is no header combination that would have worked.

// ── Searching comments rather than labels ──────────────────────────────────
//
// Investigated on 2026-09-07, and this is what replaces the label strategy.
//
// Algora and Polar, the two platforms that funded open-source issues, have left
// that market: Algora pivoted to hiring and switched its API off (the route
// /api/trpc/bounty.list exists, answers 200, and always returns {"items":[]} —
// the query logic is commented out in its source), and Polar became a payments
// platform for products, with no issue or reward endpoint at all.
//
// What is left is the trail on GitHub itself: `/bounty` is the command that
// creates a reward on an issue, and it stays in the comment history. Measured
// the same day, comparing three searches:
//
//	label:"💎 Bounty"       →    558 results, nearly all farms
//	label:"bounty"          →  4,281 results, farms and crypto micro-bounties
//	"/bounty" in:comments   → 17,997, and here asterisk/asterisk,
//	                          vllm-project/vllm-omni and circlefin/arc-node appear
//
// Labels became noise because anyone can create one; a comment carrying the
// command is the trace of someone who actually used the platform.
//
// One honest limit: GitHub tokenises the query and drops the slash, so this
// matches the word "bounty" in comments rather than the command specifically.
// It is a better filter than labels, not an exact one, and it does not recover
// the amount — that lives in the comment body and is not fetched yet.

// Repo is the minimum about a repository needed to judge whether it is serious.
type Repo struct {
	FullName  string
	Stars     int
	Fork      bool
	Archived  bool
	Language  string
	PushedAt  time.Time
	OpenIssue int
}

// SearchComments looks for open issues whose comment history contains the
// term. This is the search that finds bounties on real projects.
func (g *GitHub) SearchComments(ctx context.Context, term string, since *time.Time, perPage int) ([]Issue, error) {
	terms := []string{strconv.Quote(term) + " in:comments", "is:issue", "is:open"}
	if since != nil {
		// updated, not created: an old issue that gained a bounty yesterday is
		// exactly the one that matters, and created would hide it forever.
		terms = append(terms, "updated:>="+since.UTC().Format("2006-01-02"))
	}

	return g.search(ctx, strings.Join(terms, " "), perPage)
}

// Repos fetches metadata for several repositories.
//
// The extra call earns its keep: the Search API cannot filter issues by the
// repository's star count, so quality can only be judged afterwards. And the
// cost is low because this comes out of the core API, at 5,000 calls an hour,
// rather than the Search API's 30 a minute.
func (g *GitHub) Repos(ctx context.Context, names []string) (map[string]Repo, error) {
	out := make(map[string]Repo, len(names))

	for _, name := range names {
		if name == "" || out[name].FullName != "" {
			continue
		}
		select {
		case <-ctx.Done():
			// Return whatever was fetched in time: half a list of judged
			// repositories beats an error, and the caller decides what to do.
			return out, ctx.Err()
		default:
		}

		repo, err := g.repo(ctx, name)
		if err != nil {
			continue // deleted or private repository; the issue fails the filter
		}
		out[name] = repo
	}

	return out, nil
}

func (g *GitHub) repo(ctx context.Context, fullName string) (Repo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.Base+"/repos/"+fullName, nil)
	if err != nil {
		return Repo{}, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "iode-adapter-bounty/2.0")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

	resp, err := g.Client.Do(req)
	if err != nil {
		return Repo{}, fmt.Errorf("querying %s: %w", fullName, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body

	if resp.StatusCode != http.StatusOK {
		return Repo{}, fmt.Errorf("%s returned HTTP %d", fullName, resp.StatusCode)
	}

	var body struct {
		FullName        string    `json:"full_name"`
		StargazersCount int       `json:"stargazers_count"`
		Fork            bool      `json:"fork"`
		Archived        bool      `json:"archived"`
		Language        string    `json:"language"`
		PushedAt        time.Time `json:"pushed_at"`
		OpenIssues      int       `json:"open_issues_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Repo{}, fmt.Errorf("parsing %s: %w", fullName, err)
	}

	return Repo{
		FullName:  body.FullName,
		Stars:     body.StargazersCount,
		Fork:      body.Fork,
		Archived:  body.Archived,
		Language:  body.Language,
		PushedAt:  body.PushedAt,
		OpenIssue: body.OpenIssues,
	}, nil
}
