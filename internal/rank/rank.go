// Package rank orders bounties by fit with your stack and by the amount offered.
//
// Nothing here is model heuristics: it is counting and string matching, so the
// result is the same on every run with the same input.
package rank

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Amadeus-22/bounty/internal/source"
)

// amountRe catches "$500", "US$ 1,200" and "1500 USD" in labels and titles.
var amountRe = regexp.MustCompile(`(?i)(?:us)?\$\s?([0-9][0-9.,]*)|([0-9][0-9.,]*)\s?(?:usd|dólares|dolares)`)

// Scored is an issue with its computed score and the reason for it.
type Scored struct {
	Issue  source.Issue
	Score  int
	Amount int    // whole dollars; 0 when no amount could be read
	Reason string // human-readable; goes into the item meta
}

// Amount extracts the largest monetary value mentioned in s.
func Amount(s string) int {
	best := 0
	for _, m := range amountRe.FindAllStringSubmatch(s, -1) {
		digits := m[1]
		if digits == "" {
			digits = m[2]
		}
		// "1,200" and "1.200" both mean twelve hundred; the separator goes.
		digits = strings.NewReplacer(",", "", ".", "").Replace(digits)
		if v, err := strconv.Atoi(digits); err == nil && v > best {
			best = v
		}
	}
	return best
}

// Score ranks one issue against the languages the user actually writes.
// now is injected so the result is reproducible in tests.
func Score(issue source.Issue, languages []string, now time.Time) Scored {
	var reasons []string
	score := 0

	haystack := strings.ToLower(issue.Title + " " + strings.Join(issue.Labels, " "))
	for _, lang := range languages {
		if lang == "" {
			continue
		}
		if strings.Contains(haystack, strings.ToLower(lang)) {
			score += 40
			reasons = append(reasons, "matches "+lang)
			break
		}
	}

	amount := Amount(issue.Title + " " + strings.Join(issue.Labels, " "))
	switch {
	case amount >= 1000:
		score += 40
		reasons = append(reasons, "high value")
	case amount >= 200:
		score += 25
		reasons = append(reasons, "medium value")
	case amount > 0:
		score += 10
		reasons = append(reasons, "low value")
	}

	// A recent issue has not gathered competition yet; one too old is usually
	// abandoned, or already fixed without being closed.
	switch age := now.Sub(issue.CreatedAt); {
	case age < 7*24*time.Hour:
		score += 20
		reasons = append(reasons, "opened this week")
	case age < 30*24*time.Hour:
		score += 10
	case age > 365*24*time.Hour:
		score -= 20
		reasons = append(reasons, "open for over a year")
	}

	// Heavy discussion means someone is probably already on it.
	if issue.Comments > 15 {
		score -= 15
		reasons = append(reasons, "long discussion")
	}

	if reasons == nil {
		reasons = []string{"no strong signal"}
	}
	return Scored{Issue: issue, Score: score, Amount: amount, Reason: strings.Join(reasons, ", ")}
}

// Top scores every issue and returns them ordered, best first.
func Top(issues []source.Issue, languages []string, now time.Time, limit int) []Scored {
	scored := make([]Scored, 0, len(issues))
	for _, issue := range issues {
		scored = append(scored, Score(issue, languages, now))
	}

	// Insertion sort: the list is small, and this keeps the tie-break rule
	// (most recent first) explicit.
	for i := 1; i < len(scored); i++ {
		for j := i; j > 0; j-- {
			a, b := scored[j-1], scored[j]
			if a.Score > b.Score || (a.Score == b.Score && !a.Issue.CreatedAt.Before(b.Issue.CreatedAt)) {
				break
			}
			scored[j-1], scored[j] = b, a
		}
	}

	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}
