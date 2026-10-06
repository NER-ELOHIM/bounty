package rank

import (
	"testing"

	"github.com/Amadeus-22/bounty/internal/source"
)

func mantenedor(bodies ...string) []source.Comment {
	out := make([]source.Comment, 0, len(bodies))
	for _, b := range bodies {
		out = append(out, source.Comment{Body: b, Maintainer: true})
	}
	return out
}

func visitante(body string) source.Comment {
	return source.Comment{Body: body}
}

func TestReadReward(t *testing.T) {
	casos := []struct {
		nome     string
		comments []source.Comment
		want     Reward
	}{
		{"comando com valor", mantenedor("/bounty $150"), Reward{Amount: 150, Command: true}},
		{"comando sem cifrão", mantenedor("thanks!\n/bounty 1,200"), Reward{Amount: 1200, Command: true}},
		{"comando reward", mantenedor("/reward 50"), Reward{Amount: 50, Command: true}},
		{"resposta do bot", mantenedor("## 💎 $300 bounty [• Acme](https://example.com)"), Reward{Amount: 300, Command: true}},
		{"comando sem valor", mantenedor("/bounty"), Reward{Command: true}},
		{"o maior valor vale", mantenedor("/bounty $100", "/bounty $250"), Reward{Amount: 250, Command: true}},
		{
			"tentativas de visitantes contam",
			append(mantenedor("/bounty $100"), visitante("/attempt #7"), visitante("I'll take it\n/claim #7")),
			Reward{Amount: 100, Command: true, Attempts: 2},
		},
		{
			"visitante pedindo recompensa não é recompensa",
			[]source.Comment{visitante("Would the Core Team consider allocating a $200 bounty to this?"), visitante("/bounty $500")},
			Reward{},
		},
		{"palavra solta não é recompensa", mantenedor("is there a bounty for this?", "it costs $20 a month"), Reward{}},
		{"comando no meio da frase não conta", mantenedor("you can type /bounty $50 to fund it"), Reward{}},
		{"sem comentários", nil, Reward{}},
	}
	for _, c := range casos {
		if got := ReadReward(c.comments); got != c.want {
			t.Errorf("%s: ReadReward = %+v, esperado %+v", c.nome, got, c.want)
		}
	}
}
