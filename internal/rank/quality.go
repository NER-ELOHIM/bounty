package rank

import (
	"strings"
	"time"

	"github.com/Amadeus-22/bounty/internal/source"
)

// Quality decides whether a repository is worth someone's time.
//
// It exists for a measured reason, not a theoretical one. On 2026-09-07 a
// label-based collection returned 28 issues, and 23 of them came from a single
// synthetic repository, with duplicated issues ("reissue via #743") and a
// "reward:" label carrying no value. The other five came from one-person
// personal repositories.
//
// The cause is economic: a reward attracts people farming rewards. Since coding
// agents became cheap, standing up a repository with a hundred fake issues and
// a bounty label became a way to show up in search results. Without this
// filter, that is what the hunt returns.
type Quality struct {
	// MinStars is the main cut. It does not measure code quality; it measures
	// that someone besides the owner has used the thing. Farms have no stars.
	MinStars int
	// MaxIdleDays drops abandoned projects: a bounty on a repository that has
	// been still for a year is almost never paid, because nobody is there to
	// merge it.
	MaxIdleDays int
	// AllowForks permits forks. The default is not to: a fork carrying a bounty
	// is the most common farm disguise, because it inherits the original's
	// appearance without inheriting the maintainer who pays.
	AllowForks bool
}

// DefaultQuality is calibrated for quality over volume.
//
// Two hundred stars and ninety days of activity is a high bar, and it costs
// supply: on a real sample it drops most of the results. That is the intent —
// one issue a week that pays is worth more than fifteen that do not.
//
// What each cut buys:
//
//	200 stars   a project someone besides the owner uses, whose maintainer has
//	            a reputation to lose by not paying what was promised
//	90 days     a living project. A bounty on a still repository is almost
//	            never paid, because nobody is there to merge it
//	no forks    a fork carrying a bounty is the commonest farm disguise: it
//	            looks like the famous project, but whoever pays is not there
//
// There is a real cost and it should be said: the more famous the project, the
// more people compete for the same bounty. If the hunt starts returning only
// issues with ten comments on them, the star cut is the first to come down.
func DefaultQuality() Quality {
	return Quality{MinStars: 200, MaxIdleDays: 90, AllowForks: false}
}

// farmNames are repository-name patterns that on their own almost always mean
// a farm. A real project is rarely called "something-bounties": it has a name
// of its own, and rewards are an operational detail, not its identity.
var farmNames = []string{
	"bounty-hunter", "bounties", "bug-bounty", "bounty-plaza",
	"agent-playground", "oss-hunter", "test-repo", "sandbox",
}

// Approve reports whether the repository passes, and why it did not when it fails.
func (q Quality) Approve(r source.Repo, now time.Time) (bool, string) {
	if r.FullName == "" {
		return false, "repository not found"
	}
	if !q.AllowForks && r.Fork {
		return false, "is a fork"
	}
	if r.Archived {
		return false, "is archived"
	}
	if r.Stars < q.MinStars {
		return false, "has fewer than " + itoa(q.MinStars) + " stars"
	}
	if q.MaxIdleDays > 0 && !r.PushedAt.IsZero() {
		if days := int(now.Sub(r.PushedAt).Hours() / 24); days > q.MaxIdleDays {
			return false, "no commit in " + itoa(days) + " days"
		}
	}

	name := strings.ToLower(r.FullName)
	if i := strings.IndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	for _, pattern := range farmNames {
		if strings.Contains(name, pattern) {
			return false, "bounty-farm name (" + pattern + ")"
		}
	}

	return true, ""
}

// Filter applies the cut to the issue list and returns what passed alongside a
// per-reason account of what did not. That account becomes a warning in the
// adapter's response: a filter that discards silently is a filter nobody
// notices is miscalibrated.
func (q Quality) Filter(
	issues []source.Issue,
	repos map[string]source.Repo,
	now time.Time,
) (approved []source.Issue, dropped map[string]int) {
	dropped = map[string]int{}

	for _, issue := range issues {
		ok, reason := q.Approve(repos[issue.Repo], now)
		if !ok {
			dropped[reason]++
			continue
		}
		approved = append(approved, issue)
	}

	return approved, dropped
}

// itoa avoids importing strconv for the sake of two error messages.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
