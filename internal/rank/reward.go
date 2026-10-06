package rank

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Amadeus-22/bounty/internal/source"
)

// The reward lives in the comment history, in one of three shapes: the command
// a maintainer typed ("/bounty $150", "/reward 50"), or the line the platform's
// bot posted in reply ("## 💎 $150 bounty"). Only these are read. A dollar sign
// anywhere else in a discussion is usually a price, a shell variable or a quote,
// and reading it would invent rewards that nobody offered.
var (
	rewardCommandRe = regexp.MustCompile(`(?mi)^\s*/(?:bounty|reward)\s+(?:us)?\$?\s?([0-9][0-9,]*)`)
	rewardBotRe     = regexp.MustCompile(`(?i)\$\s?([0-9][0-9,]*)\s+bounty`)
	// A bare "/bounty" with no amount still shows the platform was used.
	bountyCommandRe = regexp.MustCompile(`(?mi)^\s*/(?:bounty|reward)\b`)
	// "/attempt #12", "/claim #12" and "/try" are how hunters announce themselves.
	attemptRe = regexp.MustCompile(`(?mi)^\s*/(?:attempt|claim|try)\b`)
)

// RewardMaxAge is how long a posted reward is believed. Past it, the reward is
// more likely forgotten than waiting: measured on 2026-10-06, the only bounties
// found on a PHP project (coollabsio/coolify, $150 and $100) dated from June
// 2024 and had been taken up by a maintainer two months later.
const RewardMaxAge = 180 * 24 * time.Hour

// closedPlatformBots posted rewards for platforms that no longer pay them.
// Algora left the bounty market; its listing answers 404.
var closedPlatformBots = map[string]bool{
	"algora-pbc[bot]": true,
}

// Reward is what the comment history says about an issue's bounty.
type Reward struct {
	Amount   int  // whole dollars; 0 when no amount could be read
	Command  bool // a live /bounty or /reward command, or the bot's reply, was found
	Expired  bool // a reward was posted, but too long ago or through a closed platform
	Attempts int  // comments announcing an attempt or a claim
}

// ReadReward extracts the reward and the competition from an issue's comments.
//
// A reward counts only when a maintainer or a bot wrote it: a visitor asking
// "would you consider a $200 bounty?" has offered nothing. Attempts count from
// anyone, since that is exactly who announces them. The largest amount wins: a
// bounty that was raised keeps its earlier comment.
//
// A reward older than RewardMaxAge, or posted by the bot of a platform that has
// closed, is not read as one; it only sets Expired, so the caller can say why
// the issue was dropped. now is injected so the result is reproducible in tests.
func ReadReward(comments []source.Comment, now time.Time) Reward {
	var r Reward
	for _, c := range comments {
		if attemptRe.MatchString(c.Body) {
			r.Attempts++
		}
		if !c.Maintainer {
			continue
		}

		amount, found := 0, bountyCommandRe.MatchString(c.Body)
		for _, re := range []*regexp.Regexp{rewardCommandRe, rewardBotRe} {
			for _, m := range re.FindAllStringSubmatch(c.Body, -1) {
				found = true
				if v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil && v > amount {
					amount = v
				}
			}
		}
		if !found {
			continue
		}

		stale := !c.CreatedAt.IsZero() && now.Sub(c.CreatedAt) > RewardMaxAge
		if stale || closedPlatformBots[c.Author] {
			r.Expired = true
			continue
		}
		r.Command = true
		if amount > r.Amount {
			r.Amount = amount
		}
	}
	return r
}
