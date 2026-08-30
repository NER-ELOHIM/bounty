// Package rank ordena bounties por aderência à sua stack e ao valor ofertado.
//
// Nada aqui é heurística de modelo: é contagem e casamento de string, para o
// resultado ser o mesmo em toda execução com a mesma entrada.
package rank

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Amadeus-22/bounty/internal/source"
)

// amountRe pega "$500", "US$ 1,200", "1500 USD" em rótulos e títulos.
var amountRe = regexp.MustCompile(`(?i)(?:us)?\$\s?([0-9][0-9.,]*)|([0-9][0-9.,]*)\s?(?:usd|dólares|dolares)`)

// Scored is an issue with its computed score and the reason for it.
type Scored struct {
	Issue  source.Issue
	Score  int
	Amount int    // em dólares inteiros; 0 quando não foi possível ler
	Reason string // legível, vai para o meta do item
}

// Amount extracts the largest monetary value mentioned in s.
func Amount(s string) int {
	best := 0
	for _, m := range amountRe.FindAllStringSubmatch(s, -1) {
		digits := m[1]
		if digits == "" {
			digits = m[2]
		}
		// "1,200" e "1.200" são mil e duzentos; separador de milhar some.
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
			reasons = append(reasons, "casa com "+lang)
			break
		}
	}

	amount := Amount(issue.Title + " " + strings.Join(issue.Labels, " "))
	switch {
	case amount >= 1000:
		score += 40
		reasons = append(reasons, "valor alto")
	case amount >= 200:
		score += 25
		reasons = append(reasons, "valor médio")
	case amount > 0:
		score += 10
		reasons = append(reasons, "valor baixo")
	}

	// Issue recente ainda não juntou concorrência; velha demais costuma estar
	// abandonada ou já resolvida sem fechar.
	switch age := now.Sub(issue.CreatedAt); {
	case age < 7*24*time.Hour:
		score += 20
		reasons = append(reasons, "aberta esta semana")
	case age < 30*24*time.Hour:
		score += 10
	case age > 365*24*time.Hour:
		score -= 20
		reasons = append(reasons, "aberta há mais de um ano")
	}

	// Muita discussão significa que alguém provavelmente já está nela.
	if issue.Comments > 15 {
		score -= 15
		reasons = append(reasons, "discussão longa")
	}

	if reasons == nil {
		reasons = []string{"sem sinal forte"}
	}
	return Scored{Issue: issue, Score: score, Amount: amount, Reason: strings.Join(reasons, ", ")}
}

// Top scores every issue and returns them ordered, best first.
func Top(issues []source.Issue, languages []string, now time.Time, limit int) []Scored {
	scored := make([]Scored, 0, len(issues))
	for _, issue := range issues {
		scored = append(scored, Score(issue, languages, now))
	}

	// Ordenação por inserção: a lista é pequena e assim o critério de desempate
	// (mais recente primeiro) fica explícito.
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
