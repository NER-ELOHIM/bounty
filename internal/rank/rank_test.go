package rank

import (
	"testing"
	"time"

	"github.com/Amadeus-22/bounty/internal/source"
)

var agora = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

func TestAmount(t *testing.T) {
	casos := []struct {
		in   string
		want int
	}{
		{"Fix parser $500", 500},
		{"bounty: US$ 1,200", 1200},
		{"paga 1.500 USD", 1500},
		{"sem valor nenhum", 0},
		{"$50 ou $300, o maior vale", 300},
	}
	for _, c := range casos {
		if got := Amount(c.in); got != c.want {
			t.Errorf("Amount(%q) = %d, esperado %d", c.in, got, c.want)
		}
	}
}

func TestScoreCasaComLinguagem(t *testing.T) {
	issue := source.Issue{Title: "Corrigir parser em Go", CreatedAt: agora.Add(-2 * 24 * time.Hour)}
	comMatch := Score(issue, []string{"go", "php"}, agora)
	semMatch := Score(issue, []string{"rust"}, agora)
	if comMatch.Score <= semMatch.Score {
		t.Errorf("casar linguagem deveria pontuar mais: %d vs %d", comMatch.Score, semMatch.Score)
	}
}

func TestScorePenalizaIssueVelhaEDiscutida(t *testing.T) {
	nova := source.Issue{Title: "x", CreatedAt: agora.Add(-24 * time.Hour)}
	velha := source.Issue{Title: "x", CreatedAt: agora.Add(-400 * 24 * time.Hour), Comments: 40}
	if Score(velha, nil, agora).Score >= Score(nova, nil, agora).Score {
		t.Error("issue velha e muito discutida deveria pontuar menos")
	}
}

func TestTopOrdenaELimita(t *testing.T) {
	issues := []source.Issue{
		{ID: 1, Title: "sem sinal", CreatedAt: agora.Add(-200 * 24 * time.Hour)},
		{ID: 2, Title: "bounty Go $2000", CreatedAt: agora.Add(-1 * 24 * time.Hour)},
		{ID: 3, Title: "algo em Go", CreatedAt: agora.Add(-10 * 24 * time.Hour)},
	}
	top := Top(issues, []string{"go"}, agora, 2)
	if len(top) != 2 {
		t.Fatalf("limite ignorado: vieram %d", len(top))
	}
	if top[0].Issue.ID != 2 {
		t.Errorf("primeiro = issue %d, esperado 2", top[0].Issue.ID)
	}
	if top[0].Score < top[1].Score {
		t.Error("the result is not in descending order")
	}
}

func TestTopListaVazia(t *testing.T) {
	if got := Top(nil, []string{"go"}, agora, 10); len(got) != 0 {
		t.Errorf("expected an empty list, got %d", len(got))
	}
}
