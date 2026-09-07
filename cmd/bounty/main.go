// Command bounty is an engine adapter that hunts for issues carrying a reward.
//
// It speaks version 1 of the adapter contract: a JSON request on stdin, a JSON
// response on stdout, the outcome in the exit code. Manual use:
//
//	echo '{"contract":1,"project":"bounties","path":"/tmp","since":null,
//	       "config":{"labels":["bounty"],"languages":["go","php"]}}' | bounty
//
// The GitHub token comes from IODE_GITHUB_TOKEN. The engine forwards anything
// prefixed IODE_ to the subprocess and never reads a credential file itself.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Amadeus-22/bounty/internal/contract"
	"github.com/Amadeus-22/bounty/internal/rank"
	"github.com/Amadeus-22/bounty/internal/source"
)

const version = "iode-adapter-bounty 1.0"

// config is the free-form object carried in the request's config field.
type config struct {
	// Terms searched in the comment history. This is the strategy that finds
	// bounties on real projects; see the note in internal/source/github.go.
	Terms     []string `json:"terms"`
	MinStars  int      `json:"min_stars"`
	MaxIdle   int      `json:"max_idle_days"`
	Labels    []string `json:"labels"`
	Languages []string `json:"languages"`
	Limit     int      `json:"limit"`
	PerPage   int      `json:"per_page"`
	TimeoutS  int      `json:"timeout_seconds"`
}

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Printf("%s, contrato %d\n", version, contract.Version)
			os.Exit(contract.ExitOK)
		}
	}

	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var ce *contract.Error
		if errors.As(err, &ce) {
			os.Exit(ce.Code)
		}
		os.Exit(contract.ExitRetryable)
	}
}

func run(ctx context.Context) error {
	req, err := contract.ReadRequest(os.Stdin)
	if err != nil {
		return err
	}

	cfg := config{Limit: 25, PerPage: 50, TimeoutS: 20}
	if len(req.Config) > 0 {
		if err := json.Unmarshal(req.Config, &cfg); err != nil {
			return &contract.Error{Code: contract.ExitBadConfig, Err: fmt.Errorf("invalid config: %w", err)}
		}
	}
	if len(cfg.Terms) == 0 {
		cfg.Terms = []string{"/bounty"}
	}
	if cfg.TimeoutS <= 0 {
		cfg.TimeoutS = 60
	}

	if len(cfg.Terms) > source.SearchLimit {
		return &contract.Error{
			Code: contract.ExitBadConfig,
			Err:  fmt.Errorf("%d terms exceed the Search API limit of %d queries per minute", len(cfg.Terms), source.SearchLimit),
		}
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutS)*time.Second)
	defer cancel()

	gh := source.NewGitHub(os.Getenv("IODE_GITHUB_TOKEN"), time.Duration(cfg.TimeoutS)*time.Second)
	if gh.Token == "" {
		defer func() {
			fmt.Fprintln(os.Stderr, "warning: without IODE_GITHUB_TOKEN the search limit drops to 10 per minute")
		}()
	}

	var (
		issues   []source.Issue
		warnings []string
		seen     = map[int64]bool{}
		failed   int
	)

	for _, term := range cfg.Terms {
		found, err := gh.SearchComments(ctx, term, req.Since, cfg.PerPage)
		if err != nil {
			failed++
			warnings = append(warnings, fmt.Sprintf("term %q: %v", term, err))
			continue
		}
		for _, issue := range found {
			if seen[issue.ID] {
				continue // the same issue matches more than one term
			}
			seen[issue.ID] = true
			issues = append(issues, issue)
		}
	}

	if failed == len(cfg.Terms) {
		return &contract.Error{
			Code: contract.ExitRetryable,
			Err:  fmt.Errorf("all %d queries failed: %s", failed, strings.Join(warnings, "; ")),
		}
	}

	// Repository metadata, to judge which ones are worth the time. This comes
	// from the core API (5,000/hour) rather than the Search API (30/minute), so
	// a call per repository is cheap.
	names := make([]string, 0, len(issues))
	listed := map[string]bool{}
	for _, issue := range issues {
		if !listed[issue.Repo] {
			listed[issue.Repo] = true
			names = append(names, issue.Repo)
		}
	}
	repos, err := gh.Repos(ctx, names)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("incomplete metadata: %v", err))
	}

	quality := rank.DefaultQuality()
	if cfg.MinStars > 0 {
		quality.MinStars = cfg.MinStars
	}
	if cfg.MaxIdle > 0 {
		quality.MaxIdleDays = cfg.MaxIdle
	}

	before := len(issues)
	issues, dropped := quality.Filter(issues, repos, time.Now())
	if before > len(issues) {
		// The account becomes a warning: a filter that discards silently is one
		// nobody notices is miscalibrated.
		parts := make([]string, 0, len(dropped))
		for reason, n := range dropped {
			parts = append(parts, fmt.Sprintf("%s: %d", reason, n))
		}
		sort.Strings(parts)
		warnings = append(warnings, fmt.Sprintf("quality: %d of %d dropped (%s)",
			before-len(issues), before, strings.Join(parts, ", ")))
	}

	now := time.Now()
	items := make([]contract.Item, 0, len(issues))
	for _, scored := range rank.Top(issues, cfg.Languages, now, cfg.Limit) {
		items = append(items, toItem(scored))
	}

	return contract.WriteResponse(os.Stdout, items, warnings)
}

func toItem(s rank.Scored) contract.Item {
	meta := map[string]any{
		"repo":     s.Issue.Repo,
		"url":      s.Issue.HTMLURL,
		"author":   s.Issue.Author,
		"labels":   s.Issue.Labels,
		"comments": s.Issue.Comments,
		"score":    s.Score,
		"reason":   s.Reason,
	}
	if s.Amount > 0 {
		meta["amount_usd"] = s.Amount
	}

	return contract.Item{
		// The URL is the natural key: unique, stable, and readable when you
		// inspect the database by hand.
		Key:   s.Issue.HTMLURL,
		Kind:  "external",
		TS:    contract.FormatTS(s.Issue.CreatedAt),
		Title: s.Issue.Title,
		Body:  s.Issue.Body,
		Meta:  meta,
	}
}
