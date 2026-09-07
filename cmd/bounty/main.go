// Command bounty é um adaptador do iode que procura issues com recompensa.
//
// Fala o contrato de adaptador versão 1: requisição JSON no stdin, resposta
// JSON no stdout, resultado no código de saída. Uso manual:
//
//	echo '{"contract":1,"project":"bounties","path":"/tmp","since":null,
//	       "config":{"labels":["bounty"],"languages":["go","php"]}}' | bounty
//
// O token do GitHub vem de IODE_GITHUB_TOKEN. O motor repassa ao subprocesso
// tudo que começa com IODE_, e nunca lê arquivo de credencial por conta
// própria: ver docs/SEGURANCA.md do iode.
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

// config é o objeto livre que vem em config na requisição.
type config struct {
	// Termos procurados no histórico de comentários. É a estratégia que
	// encontra bounty de projeto real; ver a nota em internal/source/github.go.
	Termos    []string `json:"termos"`
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
			return &contract.Error{Code: contract.ExitBadConfig, Err: fmt.Errorf("config inválida: %w", err)}
		}
	}
	if len(cfg.Termos) == 0 {
		cfg.Termos = []string{"/bounty"}
	}
	if cfg.TimeoutS <= 0 {
		cfg.TimeoutS = 60
	}

	if len(cfg.Termos) > source.SearchLimit {
		return &contract.Error{
			Code: contract.ExitBadConfig,
			Err:  fmt.Errorf("%d termos passam do limite de %d consultas por minuto da Search API", len(cfg.Termos), source.SearchLimit),
		}
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutS)*time.Second)
	defer cancel()

	gh := source.NewGitHub(os.Getenv("IODE_GITHUB_TOKEN"), time.Duration(cfg.TimeoutS)*time.Second)
	if gh.Token == "" {
		defer func() {
			fmt.Fprintln(os.Stderr, "aviso: sem IODE_GITHUB_TOKEN, o limite da busca cai para 10 por minuto")
		}()
	}

	var (
		issues   []source.Issue
		warnings []string
		seen     = map[int64]bool{}
		failed   int
	)

	for _, termo := range cfg.Termos {
		found, err := gh.SearchComments(ctx, termo, req.Since, cfg.PerPage)
		if err != nil {
			failed++
			warnings = append(warnings, fmt.Sprintf("termo %q: %v", termo, err))
			continue
		}
		for _, issue := range found {
			if seen[issue.ID] {
				continue // a mesma issue casa com mais de um termo
			}
			seen[issue.ID] = true
			issues = append(issues, issue)
		}
	}

	if failed == len(cfg.Termos) {
		return &contract.Error{
			Code: contract.ExitRetryable,
			Err:  fmt.Errorf("todas as %d consultas falharam: %s", failed, strings.Join(warnings, "; ")),
		}
	}

	// Metadados dos repositórios, para julgar quais valem o tempo. Sai da API
	// principal (5.000/hora), e não da Search API (30/minuto), então a chamada
	// por repositório é barata.
	nomes := make([]string, 0, len(issues))
	vistos := map[string]bool{}
	for _, issue := range issues {
		if !vistos[issue.Repo] {
			vistos[issue.Repo] = true
			nomes = append(nomes, issue.Repo)
		}
	}
	repos, err := gh.Repos(ctx, nomes)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("metadados incompletos: %v", err))
	}

	qualidade := rank.DefaultQuality()
	if cfg.MinStars > 0 {
		qualidade.MinStars = cfg.MinStars
	}
	if cfg.MaxIdle > 0 {
		qualidade.MaxIdleDays = cfg.MaxIdle
	}

	antes := len(issues)
	issues, descartes := qualidade.Filtrar(issues, repos, time.Now())
	if antes > len(issues) {
		// O relato vira aviso: filtro que descarta em silêncio é filtro que
		// ninguém percebe estar calibrado errado.
		partes := make([]string, 0, len(descartes))
		for motivo, n := range descartes {
			partes = append(partes, fmt.Sprintf("%s: %d", motivo, n))
		}
		sort.Strings(partes)
		warnings = append(warnings, fmt.Sprintf("qualidade: %d de %d descartados (%s)",
			antes-len(issues), antes, strings.Join(partes, ", ")))
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
		"autor":    s.Issue.Author,
		"labels":   s.Issue.Labels,
		"comments": s.Issue.Comments,
		"score":    s.Score,
		"motivo":   s.Reason,
	}
	if s.Amount > 0 {
		meta["valor_usd"] = s.Amount
	}

	return contract.Item{
		// A URL é a chave natural: única, estável, e legível quando você
		// inspeciona o banco com sqlite3.
		Key:   s.Issue.HTMLURL,
		Kind:  "external",
		TS:    contract.FormatTS(s.Issue.CreatedAt),
		Title: s.Issue.Title,
		Body:  s.Issue.Body,
		Meta:  meta,
	}
}
