// Package source busca bounties abertos em plataformas públicas.
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SearchLimit é o teto do GitHub para a Search API: 30 requisições por minuto
// com token, 10 sem. É bem mais apertado que o limite da API geral, então uma
// consulta por rótulo por execução é o que cabe.
const SearchLimit = 30

// Issue is one open issue carrying a bounty label.
type Issue struct {
	ID        int64
	Number    int
	Title     string
	Body      string
	HTMLURL   string
	Repo      string
	Author    string
	Labels    []string
	Comments  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GitHub queries the GitHub Search API for issues.
type GitHub struct {
	Client *http.Client
	Token  string // opcional; sem ele o limite cai para 10 req/min
	Base   string // sobrescrito nos testes
}

// NewGitHub builds a client with an explicit timeout on every request.
func NewGitHub(token string, timeout time.Duration) *GitHub {
	return &GitHub{
		Client: &http.Client{Timeout: timeout},
		Token:  token,
		Base:   "https://api.github.com",
	}
}

type searchResponse struct {
	TotalCount        int  `json:"total_count"`
	IncompleteResults bool `json:"incomplete_results"`
	Items             []struct {
		ID      int64  `json:"id"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		RepoURL string `json:"repository_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Comments  int       `json:"comments"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"items"`
}

// Search runs one query. label and language may be empty; since filters by
// creation date and keeps the result set from growing without bound.
func (g *GitHub) Search(ctx context.Context, label, language string, since *time.Time, perPage int) ([]Issue, error) {
	terms := []string{"is:issue", "is:open"}
	if label != "" {
		terms = append(terms, `label:"`+label+`"`)
	}
	if language != "" {
		terms = append(terms, "language:"+language)
	}
	if since != nil {
		terms = append(terms, "created:>="+since.UTC().Format("2006-01-02"))
	}

	return g.buscar(ctx, strings.Join(terms, " "), perPage)
}

// buscar executa uma consulta já montada contra a Search API.
//
// A montagem da consulta fica com quem chama, porque cada estratégia de caça
// monta a sua; o que se repete — paginação, cabeçalho, limite de requisições,
// tradução da resposta — mora aqui.
func (g *GitHub) buscar(ctx context.Context, consulta string, perPage int) ([]Issue, error) {
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	q := url.Values{}
	q.Set("q", consulta)
	q.Set("per_page", strconv.Itoa(perPage))
	q.Set("sort", "created")
	q.Set("order", "desc")

	endpoint := g.Base + "/search/issues?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("montando requisição: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "iode-adapter-bounty/1.0")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("consultando o GitHub: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // corpo somente-leitura

	switch {
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		reset := resp.Header.Get("X-RateLimit-Reset")
		return nil, fmt.Errorf("limite de requisições atingido (reset em %s); use IODE_GITHUB_TOKEN", reset)
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("token rejeitado pelo GitHub")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub devolveu HTTP %d", resp.StatusCode)
	}

	var body searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("interpretando resposta: %w", err)
	}

	issues := make([]Issue, 0, len(body.Items))
	for _, it := range body.Items {
		labels := make([]string, 0, len(it.Labels))
		for _, l := range it.Labels {
			labels = append(labels, l.Name)
		}
		issues = append(issues, Issue{
			ID:        it.ID,
			Number:    it.Number,
			Title:     it.Title,
			Body:      it.Body,
			HTMLURL:   it.HTMLURL,
			Repo:      repoFromAPIURL(it.RepoURL),
			Author:    it.User.Login,
			Labels:    labels,
			Comments:  it.Comments,
			CreatedAt: it.CreatedAt,
			UpdatedAt: it.UpdatedAt,
		})
	}
	return issues, nil
}

// repoFromAPIURL turns https://api.github.com/repos/dono/nome into dono/nome.
func repoFromAPIURL(raw string) string {
	const marker = "/repos/"
	if i := strings.Index(raw, marker); i >= 0 {
		return raw[i+len(marker):]
	}
	return raw
}

// FASE 2: Algora como segunda fonte.
//
// Investigado em 30/08/2026 e NÃO implementado: a API pública não responde como
// a documentação descreve. O que foi testado, para ninguém repetir o caminho:
//
//	api.algora.io/api/orgs/{org}/bounties  → 301 para algora.io, que devolve 404
//	algora.io/api/orgs/{org}/bounties      → 404 em cinco organizações diferentes
//	api.algora.io/bounties                 → 406 com Accept: application/json,
//	                                         404 sem Accept — inconsistente
//
// A rota /bounties existe (406 é negociação de conteúdo, não rota ausente), mas
// não achei a combinação de cabeçalho que ela aceita. Antes de implementar,
// confirmar contra github.com/algora-io/sdk, que é o cliente oficial.

// ── Busca por comentário, e não por rótulo ──────────────────────────────────
//
// Investigado em 07/09/2026, e é o que substitui a estratégia de rótulo.
//
// A Algora e a Polar, as duas plataformas que financiavam issue de código
// aberto, saíram desse mercado: a Algora virou contratação e desativou a API
// (a rota /api/trpc/bounty.list existe, responde 200 e devolve sempre
// {"items":[]} — a lógica está comentada no fonte dela), e a Polar virou
// meio de pagamento para produtos, sem nenhum endpoint de issue ou reward.
//
// O que sobrou é o rastro no próprio GitHub: `/bounty` é o comando que cria
// uma recompensa numa issue, e ele fica no histórico de comentários. Medido no
// mesmo dia, comparando as três buscas:
//
//	label:"💎 Bounty"       →   558 resultados, quase todos fazenda
//	label:"bounty"          → 4.281 resultados, fazenda e micro-bounty de cripto
//	"/bounty" in:comments   → 17.997, e aqui aparecem asterisk/asterisk,
//	                          vllm-project/vllm-omni, circlefin/arc-node
//
// Rótulo virou ruído porque qualquer um cria um; comentário com o comando é
// rastro de quem usou a plataforma de verdade.

// Repo é o mínimo sobre um repositório que permite julgar se ele é sério.
type Repo struct {
	FullName  string
	Stars     int
	Fork      bool
	Archived  bool
	Language  string
	PushedAt  time.Time
	OpenIssue int
}

// SearchComments procura issues abertas cujo histórico de comentários contém
// o termo. É a busca que encontra bounty de projeto real.
func (g *GitHub) SearchComments(ctx context.Context, termo string, since *time.Time, perPage int) ([]Issue, error) {
	terms := []string{strconv.Quote(termo) + " in:comments", "is:issue", "is:open"}
	if since != nil {
		// updated, e não created: uma issue antiga que ganhou bounty ontem é
		// exatamente a que interessa, e created a esconderia para sempre.
		terms = append(terms, "updated:>="+since.UTC().Format("2006-01-02"))
	}

	return g.buscar(ctx, strings.Join(terms, " "), perPage)
}

// Repos busca os metadados de vários repositórios.
//
// Vale a chamada extra: a Search API não deixa filtrar issue por estrela do
// repositório, então a qualidade só dá para julgar depois. E o custo é baixo
// porque isto sai da API principal, com 5.000 chamadas por hora, e não da
// Search API, que dá 30 por minuto.
func (g *GitHub) Repos(ctx context.Context, nomes []string) (map[string]Repo, error) {
	saida := make(map[string]Repo, len(nomes))

	for _, nome := range nomes {
		if nome == "" || saida[nome].FullName != "" {
			continue
		}
		select {
		case <-ctx.Done():
			// Devolve o que já deu tempo de buscar: meia lista de repositórios
			// julgados é melhor que erro, e quem chama decide o que fazer.
			return saida, ctx.Err()
		default:
		}

		repo, err := g.repo(ctx, nome)
		if err != nil {
			continue // repositório apagado ou privado; a issue cai fora no filtro
		}
		saida[nome] = repo
	}

	return saida, nil
}

func (g *GitHub) repo(ctx context.Context, fullName string) (Repo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.Base+"/repos/"+fullName, nil)
	if err != nil {
		return Repo{}, fmt.Errorf("montando requisição: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "iode-adapter-bounty/2.0")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

	resp, err := g.Client.Do(req)
	if err != nil {
		return Repo{}, fmt.Errorf("consultando %s: %w", fullName, err)
	}
	defer resp.Body.Close() //nolint:errcheck // corpo somente-leitura

	if resp.StatusCode != http.StatusOK {
		return Repo{}, fmt.Errorf("%s devolveu HTTP %d", fullName, resp.StatusCode)
	}

	var body struct {
		FullName        string    `json:"full_name"`
		StargazersCount int       `json:"stargazers_count"`
		Fork            bool      `json:"fork"`
		Archived        bool      `json:"archived"`
		Language        string    `json:"language"`
		PushedAt        time.Time `json:"pushed_at"`
		OpenIssues      int       `json:"open_issues_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Repo{}, fmt.Errorf("interpretando %s: %w", fullName, err)
	}

	return Repo{
		FullName:  body.FullName,
		Stars:     body.StargazersCount,
		Fork:      body.Fork,
		Archived:  body.Archived,
		Language:  body.Language,
		PushedAt:  body.PushedAt,
		OpenIssue: body.OpenIssues,
	}, nil
}
