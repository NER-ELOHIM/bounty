package rank

import (
	"strings"
	"time"

	"github.com/Amadeus-22/bounty/internal/source"
)

// Quality decide se um repositório merece o tempo de alguém.
//
// Existe por um motivo medido, não teórico. Em 07/09/2026, uma coleta por
// rótulo trouxe 28 issues, e 23 delas vinham de um único repositório sintético,
// com issues duplicadas ("reissue via #743") e rótulo "reward:" sem valor. As
// outras cinco vinham de repositórios pessoais de uma pessoa só.
//
// A causa é econômica: recompensa atrai quem quer farmar recompensa. Desde que
// agente de código ficou barato, criar um repositório com cem issues falsas e
// rótulo de bounty virou estratégia de quem quer aparecer em busca. Sem este
// filtro, é isso que a caça devolve.
type Quality struct {
	// MinStars é o corte principal. Não mede qualidade de código; mede que
	// alguém além do dono já usou aquilo. Fazenda não tem estrela.
	MinStars int
	// MaxIdleDays descarta projeto abandonado: bounty em repositório parado há
	// um ano quase nunca é pago, porque não há quem faça o merge.
	MaxIdleDays int
	// AllowForks permite fork. O padrão é não: fork com bounty é o disfarce
	// mais comum de fazenda, porque herda as estrelas do original na aparência
	// sem herdar o mantenedor que paga.
	AllowForks bool
}

// DefaultQuality são os cortes calibrados para qualidade acima de volume.
//
// 200 estrelas e 90 dias sem parar é uma régua alta, e ela custa oferta: numa
// amostra real ela derruba a maioria dos resultados. É o que se quer aqui —
// vale mais uma issue por semana que renda do que quinze que não rendem.
//
// O que cada corte compra:
//
//	200 estrelas   projeto que alguém além do dono usa, e cujo mantenedor tem
//	               reputação a perder se não pagar o que prometeu
//	90 dias        projeto vivo. Bounty em repositório parado quase nunca é
//	               pago, porque não há quem faça o merge
//	sem fork       fork com bounty é o disfarce mais comum de fazenda: parece
//	               o projeto famoso, mas quem paga não está lá
//
// Há um custo real e ele deve ser dito: quanto mais famoso o projeto, mais
// gente disputa o mesmo bounty. Se a caça começar a devolver só issue com dez
// comentários, o corte de estrelas é o primeiro a baixar.
func DefaultQuality() Quality {
	return Quality{MinStars: 200, MaxIdleDays: 90, AllowForks: false}
}

// nomesSuspeitos são padrões no nome do repositório que, sozinhos, quase sempre
// indicam fazenda. Um projeto real raramente se chama "algo-bounties": ele tem
// um nome próprio e recompensa é detalhe operacional, não a identidade dele.
var nomesSuspeitos = []string{
	"bounty-hunter", "bounties", "bug-bounty", "bounty-plaza",
	"agent-playground", "oss-hunter", "test-repo", "sandbox",
}

// Aprova diz se o repositório passa, e por que não passou quando falha.
func (q Quality) Aprova(r source.Repo, agora time.Time) (bool, string) {
	if r.FullName == "" {
		return false, "repositório não encontrado"
	}
	if !q.AllowForks && r.Fork {
		return false, "é um fork"
	}
	if r.Archived {
		return false, "está arquivado"
	}
	if r.Stars < q.MinStars {
		return false, "tem menos de " + itoa(q.MinStars) + " estrelas"
	}
	if q.MaxIdleDays > 0 && !r.PushedAt.IsZero() {
		if dias := int(agora.Sub(r.PushedAt).Hours() / 24); dias > q.MaxIdleDays {
			return false, "sem commit há " + itoa(dias) + " dias"
		}
	}

	nome := strings.ToLower(r.FullName)
	if i := strings.IndexByte(nome, '/'); i >= 0 {
		nome = nome[i+1:]
	}
	for _, suspeito := range nomesSuspeitos {
		if strings.Contains(nome, suspeito) {
			return false, "nome de fazenda de bounty (" + suspeito + ")"
		}
	}

	return true, ""
}

// Filtrar aplica o corte à lista de issues e devolve as aprovadas mais o
// relato do que caiu, por motivo. O relato vira aviso na resposta do
// adaptador: um filtro que descarta em silêncio é um filtro que ninguém
// percebe estar calibrado errado.
func (q Quality) Filtrar(
	issues []source.Issue,
	repos map[string]source.Repo,
	agora time.Time,
) (aprovadas []source.Issue, descartes map[string]int) {
	descartes = map[string]int{}

	for _, issue := range issues {
		ok, motivo := q.Aprova(repos[issue.Repo], agora)
		if !ok {
			descartes[motivo]++
			continue
		}
		aprovadas = append(aprovadas, issue)
	}

	return aprovadas, descartes
}

// itoa evita importar strconv só para duas mensagens de erro.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negativo := n < 0
	if negativo {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negativo {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
