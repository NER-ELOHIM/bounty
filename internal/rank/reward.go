package rank

import (
	"regexp"
	"strconv"
	"strings"

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

// Reward is what the comment history says about an issue's bounty.
type Reward struct {
	Amount   int  // whole dollars; 0 when no amount could be read
	Command  bool // a /bounty or /reward command, or the bot's reply, was found
	Attempts int  // comments announcing an attempt or a claim
}

// ReadReward extracts the reward and the competition from an issue's comments.
//
// A reward counts only when a maintainer or a bot wrote it: a visitor asking
// "would you consider a $200 bounty?" has offered nothing. Attempts count from
// anyone, since that is exactly who announces them. The largest amount wins: a
// bounty that was raised keeps its earlier comment.
func ReadReward(comments []source.Comment) Reward {
	var r Reward
	for _, c := range comments {
		if attemptRe.MatchString(c.Body) {
			r.Attempts++
		}
		if !c.Maintainer {
			continue
		}
		for _, re := range []*regexp.Regexp{rewardCommandRe, rewardBotRe} {
			for _, m := range re.FindAllStringSubmatch(c.Body, -1) {
				r.Command = true
				if v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil && v > r.Amount {
					r.Amount = v
				}
			}
		}
		if bountyCommandRe.MatchString(c.Body) {
			r.Command = true
		}
	}
	return r
}
