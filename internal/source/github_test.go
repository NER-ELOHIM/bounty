package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The test server binds to loopback: no test talks to the real GitHub.
func servidor(t *testing.T, status int, corpo string) *GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("version header missing: %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(corpo))
	}))
	t.Cleanup(srv.Close)

	gh := NewGitHub("token-de-teste", 5*time.Second)
	gh.Base = srv.URL
	return gh
}

const respostaOK = `{"total_count":1,"incomplete_results":false,"items":[{
  "id":42,"number":7,"title":"Fix pagination","body":"details",
  "html_url":"https://github.com/dono/nome/issues/7",
  "repository_url":"https://api.github.com/repos/dono/nome",
  "user":{"login":"alguem"},
  "labels":[{"name":"bounty"},{"name":"$300"}],
  "comments":3,
  "created_at":"2026-08-28T14:32:10Z","updated_at":"2026-08-29T10:00:00Z"}]}`

func TestSearchInterpretaResposta(t *testing.T) {
	gh := servidor(t, http.StatusOK, respostaOK)

	issues, err := gh.Search(context.Background(), "bounty", "go", nil, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	got := issues[0]
	if got.Repo != "dono/nome" {
		t.Errorf("Repo = %q, esperado dono/nome", got.Repo)
	}
	if got.Author != "alguem" {
		t.Errorf("Author = %q", got.Author)
	}
	if len(got.Labels) != 2 || got.Labels[1] != "$300" {
		t.Errorf("Labels = %v", got.Labels)
	}
	if !got.CreatedAt.Equal(time.Date(2026, 8, 28, 14, 32, 10, 0, time.UTC)) {
		t.Errorf("CreatedAt = %v", got.CreatedAt)
	}
}

func TestSearchLimiteDeRequisicoes(t *testing.T) {
	gh := servidor(t, http.StatusForbidden, `{"message":"rate limit"}`)

	_, err := gh.Search(context.Background(), "bounty", "", nil, 50)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "IODE_GITHUB_TOKEN") {
		t.Errorf("a mensagem deveria dizer como resolver: %v", err)
	}
}

func TestSearchStatusInesperado(t *testing.T) {
	gh := servidor(t, http.StatusInternalServerError, `oops`)

	if _, err := gh.Search(context.Background(), "bounty", "", nil, 50); err == nil {
		t.Fatal("expected an error for HTTP 500")
	}
}

func TestSearchRespeitaContextoCancelado(t *testing.T) {
	gh := servidor(t, http.StatusOK, respostaOK)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := gh.Search(ctx, "bounty", "", nil, 50); err == nil {
		t.Fatal("expected an error on a cancelled context")
	}
}

func TestRepoFromAPIURL(t *testing.T) {
	if got := repoFromAPIURL("https://api.github.com/repos/dono/nome"); got != "dono/nome" {
		t.Errorf("got %q", got)
	}
	if got := repoFromAPIURL("lixo"); got != "lixo" {
		t.Errorf("entrada inesperada deveria voltar intacta, veio %q", got)
	}
}

func TestCommentsDevolveOsCorpos(t *testing.T) {
	gh := servidor(t, http.StatusOK, `[
	  {"body":"/bounty $150","author_association":"MEMBER","created_at":"2026-08-28T14:32:10Z","user":{"login":"dono","type":"User"}},
	  {"body":"## 💎 $150 bounty","author_association":"NONE","created_at":"2026-08-28T14:32:11Z","user":{"login":"plataforma[bot]","type":"Bot"}},
	  {"body":"/attempt #7","author_association":"NONE","created_at":"2026-08-29T09:00:00Z","user":{"login":"visita","type":"User"}}]`)

	got, err := gh.Comments(context.Background(), "dono/nome", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Comment{
		{Body: "/bounty $150", Author: "dono", CreatedAt: time.Date(2026, 8, 28, 14, 32, 10, 0, time.UTC), Maintainer: true},
		{Body: "## 💎 $150 bounty", Author: "plataforma[bot]", CreatedAt: time.Date(2026, 8, 28, 14, 32, 11, 0, time.UTC), Maintainer: true},
		{Body: "/attempt #7", Author: "visita", CreatedAt: time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC), Maintainer: false},
	}
	if len(got) != len(want) {
		t.Fatalf("Comments = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Comments[%d] = %+v, esperado %+v", i, got[i], want[i])
		}
	}
}

func TestCommentsFalhaForaDe200(t *testing.T) {
	gh := servidor(t, http.StatusNotFound, `{"message":"Not Found"}`)

	if _, err := gh.Comments(context.Background(), "dono/nome", 7); err == nil {
		t.Fatal("expected an error for HTTP 404")
	}
}

func TestSearchCommentsExcluiIssuesComPRVinculado(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"total_count":0,"items":[]}`))
	}))
	t.Cleanup(srv.Close)

	gh := NewGitHub("token-de-teste", 5*time.Second)
	gh.Base = srv.URL
	if _, err := gh.SearchComments(context.Background(), "/bounty", nil, 50); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{`"/bounty" in:comments`, "is:open", "-linked:pr"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q is missing %q", query, want)
		}
	}
}
