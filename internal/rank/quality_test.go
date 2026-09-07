package rank

import (
	"testing"
	"time"

	"github.com/Amadeus-22/bounty/internal/source"
)

func repoSaudavel(agora time.Time) source.Repo {
	return source.Repo{FullName: "asterisk/asterisk", Stars: 2400, PushedAt: agora.Add(-24 * time.Hour)}
}

func TestAprovaRepositorioSaudavel(t *testing.T) {
	agora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	if ok, motivo := DefaultQuality().Aprova(repoSaudavel(agora), agora); !ok {
		t.Fatalf("deveria aprovar, recusou por %q", motivo)
	}
}

func TestRecusa(t *testing.T) {
	agora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	q := DefaultQuality()

	casos := []struct {
		nome   string
		repo   source.Repo
		motivo string
	}{
		{"desconhecido", source.Repo{}, "não encontrado"},
		{"fork", source.Repo{FullName: "x/qdrant", Stars: 9000, Fork: true, PushedAt: agora}, "fork"},
		{"arquivado", source.Repo{FullName: "x/y", Stars: 900, Archived: true, PushedAt: agora}, "arquivado"},
		{"poucas estrelas", source.Repo{FullName: "x/y", Stars: 60, PushedAt: agora}, "estrelas"},
		{"parado", source.Repo{FullName: "x/y", Stars: 900, PushedAt: agora.Add(-200 * 24 * time.Hour)}, "sem commit"},
		// Os três nomes vistos na coleta real de 07/09/2026.
		{"fazenda", source.Repo{FullName: "SecureBananaLabs/bug-bounty", Stars: 900, PushedAt: agora}, "fazenda"},
		{"playground", source.Repo{FullName: "xevrion-v2/agent-playground", Stars: 900, PushedAt: agora}, "fazenda"},
		{"plaza", source.Repo{FullName: "z/bounty-plaza", Stars: 900, PushedAt: agora}, "fazenda"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			ok, motivo := q.Aprova(c.repo, agora)
			if ok {
				t.Fatalf("deveria recusar %s", c.repo.FullName)
			}
			if !contains(motivo, c.motivo) {
				t.Fatalf("motivo %q não menciona %q", motivo, c.motivo)
			}
		})
	}
}

func TestFiltrarRelataOsDescartes(t *testing.T) {
	agora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	issues := []source.Issue{
		{ID: 1, Repo: "asterisk/asterisk"},
		{ID: 2, Repo: "SecureBananaLabs/bug-bounty"},
		{ID: 3, Repo: "SecureBananaLabs/bug-bounty"},
		{ID: 4, Repo: "apagado/sumiu"},
	}
	repos := map[string]source.Repo{
		"asterisk/asterisk":           repoSaudavel(agora),
		"SecureBananaLabs/bug-bounty": {FullName: "SecureBananaLabs/bug-bounty", Stars: 800, PushedAt: agora},
	}

	aprovadas, descartes := DefaultQuality().Filtrar(issues, repos, agora)
	if len(aprovadas) != 1 || aprovadas[0].ID != 1 {
		t.Fatalf("esperava só a issue 1, veio %v", aprovadas)
	}
	if total := descartes["nome de fazenda de bounty (bug-bounty)"]; total != 2 {
		t.Fatalf("esperava 2 descartes por fazenda, veio %d (%v)", total, descartes)
	}
	if descartes["repositório não encontrado"] != 1 {
		t.Fatalf("o repositório ausente deveria ser relatado: %v", descartes)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
