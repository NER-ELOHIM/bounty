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

func TestScoreUsaRecompensaDosComentarios(t *testing.T) {
	semValor := source.Issue{Title: "x", CreatedAt: agora.Add(-24 * time.Hour)}
	comValor := semValor
	comValor.RewardUSD = 500

	got := Score(comValor, nil, agora)
	if got.Amount != 500 {
		t.Errorf("Amount = %d, esperado 500", got.Amount)
	}
	if got.Score <= Score(semValor, nil, agora).Score {
		t.Errorf("recompensa lida dos comentários deveria pontuar mais")
	}
}

func TestScorePenalizaTentativasComTeto(t *testing.T) {
	base := source.Issue{Title: "x", CreatedAt: agora.Add(-24 * time.Hour)}
	livre := Score(base, nil, agora).Score

	uma := base
	uma.Attempts = 1
	muitas := base
	muitas.Attempts = 9

	if got := Score(uma, nil, agora).Score; got != livre-10 {
		t.Errorf("uma tentativa: score = %d, esperado %d", got, livre-10)
	}
	if got := Score(muitas, nil, agora).Score; got != livre-30 {
		t.Errorf("nove tentativas: score = %d, esperado %d (teto)", got, livre-30)
	}
}

func TestScoreCasaLinguagemComoPalavraInteira(t *testing.T) {
	casos := []struct {
		titulo string
		labels []string
		want   bool
	}{
		{"Fix the Go client", nil, true},
		{"Fix pagination", []string{"lang: go"}, true},
		{"golang: fix pagination", nil, false},
		{"Translate the README", []string{"good first issue"}, false},
		{"Add category filter", nil, false},
	}
	for _, c := range casos {
		issue := source.Issue{Title: c.titulo, Labels: c.labels, CreatedAt: agora.Add(-40 * 24 * time.Hour)}
		got := Score(issue, []string{"go"}, agora).Score >= 40
		if got != c.want {
			t.Errorf("%q %v: casou=%v, esperado %v", c.titulo, c.labels, got, c.want)
		}
	}
}
