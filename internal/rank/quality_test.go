package rank

import (
	"testing"
	"time"

	"github.com/Amadeus-22/bounty/internal/source"
)

func healthyRepo(now time.Time) source.Repo {
	return source.Repo{FullName: "asterisk/asterisk", Stars: 2400, PushedAt: now.Add(-24 * time.Hour)}
}

func TestApprovesAHealthyRepository(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	if ok, reason := DefaultQuality().Approve(healthyRepo(now), now); !ok {
		t.Fatalf("should have approved, refused with %q", reason)
	}
}

func TestRefuses(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	q := DefaultQuality()

	casos := []struct {
		name   string
		repo   source.Repo
		reason string
	}{
		{"unknown", source.Repo{}, "not found"},
		{"fork", source.Repo{FullName: "x/qdrant", Stars: 9000, Fork: true, PushedAt: now}, "fork"},
		{"archived", source.Repo{FullName: "x/y", Stars: 900, Archived: true, PushedAt: now}, "archived"},
		{"too few stars", source.Repo{FullName: "x/y", Stars: 60, PushedAt: now}, "stars"},
		{"stalled", source.Repo{FullName: "x/y", Stars: 900, PushedAt: now.Add(-200 * 24 * time.Hour)}, "no commit"},
		// The three names seen in the real 2026-09-07 collection.
		{"farm", source.Repo{FullName: "SecureBananaLabs/bug-bounty", Stars: 900, PushedAt: now}, "farm"},
		{"playground", source.Repo{FullName: "xevrion-v2/agent-playground", Stars: 900, PushedAt: now}, "farm"},
		{"plaza", source.Repo{FullName: "z/bounty-plaza", Stars: 900, PushedAt: now}, "farm"},
	}

	for _, c := range casos {
		t.Run(c.name, func(t *testing.T) {
			ok, reason := q.Approve(c.repo, now)
			if ok {
				t.Fatalf("should have refused %s", c.repo.FullName)
			}
			if !contains(reason, c.reason) {
				t.Fatalf("reason %q does not mention %q", reason, c.reason)
			}
		})
	}
}

func TestFilterReportsWhatItDropped(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	issues := []source.Issue{
		{ID: 1, Repo: "asterisk/asterisk"},
		{ID: 2, Repo: "SecureBananaLabs/bug-bounty"},
		{ID: 3, Repo: "SecureBananaLabs/bug-bounty"},
		{ID: 4, Repo: "apagado/sumiu"},
	}
	repos := map[string]source.Repo{
		"asterisk/asterisk":           healthyRepo(now),
		"SecureBananaLabs/bug-bounty": {FullName: "SecureBananaLabs/bug-bounty", Stars: 800, PushedAt: now},
	}

	approved, dropped := DefaultQuality().Filter(issues, repos, now)
	if len(approved) != 1 || approved[0].ID != 1 {
		t.Fatalf("expected only issue 1, got %v", approved)
	}
	if total := dropped["bounty-farm name (bug-bounty)"]; total != 2 {
		t.Fatalf("expected 2 farm drops, got %d (%v)", total, dropped)
	}
	if dropped["repository not found"] != 1 {
		t.Fatalf("the missing repository should be reported: %v", dropped)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
