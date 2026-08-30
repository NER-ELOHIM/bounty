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
	"strings"
	"time"

	"github.com/Amadeus-22/bounty/internal/contract"
	"github.com/Amadeus-22/bounty/internal/rank"
	"github.com/Amadeus-22/bounty/internal/source"
)

const version = "iode-adapter-bounty 1.0"

// config é o objeto livre que vem em config na requisição.
type config struct {
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
	if len(cfg.Labels) == 0 {
		cfg.Labels = []string{"bounty"}
	}
	if cfg.TimeoutS <= 0 {
		cfg.TimeoutS = 20
	}

	// Uma consulta por rótulo. O limite da Search API é 30 por minuto, então
	// uma lista longa de rótulos estoura a cota e não coleta nada.
	if len(cfg.Labels) > source.SearchLimit {
		return &contract.Error{
			Code: contract.ExitBadConfig,
			Err:  fmt.Errorf("%d rótulos passam do limite de %d consultas por minuto da Search API", len(cfg.Labels), source.SearchLimit),
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

	for _, label := range cfg.Labels {
		found, err := gh.Search(ctx, label, "", req.Since, cfg.PerPage)
		if err != nil {
			failed++
			warnings = append(warnings, fmt.Sprintf("rótulo %q: %v", label, err))
			continue
		}
		for _, issue := range found {
			if seen[issue.ID] {
				continue // a mesma issue casa com mais de um rótulo
			}
			seen[issue.ID] = true
			issues = append(issues, issue)
		}
	}

	// Todas as consultas falharam: erro recuperável, o motor tenta de novo.
	if failed == len(cfg.Labels) {
		return &contract.Error{
			Code: contract.ExitRetryable,
			Err:  fmt.Errorf("todas as %d consultas falharam: %s", failed, strings.Join(warnings, "; ")),
		}
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
