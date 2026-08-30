package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestReadRequestValida(t *testing.T) {
	in := `{"contract":1,"project":"b","path":"/tmp","since":"2026-08-28T00:00:00-03:00","config":{"limit":5}}`
	req, err := ReadRequest(strings.NewReader(in))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if req.Project != "b" {
		t.Errorf("project = %q, esperado %q", req.Project, "b")
	}
	if req.Since == nil {
		t.Fatal("since deveria ter sido interpretado")
	}
	if got := FormatTS(*req.Since); got != "2026-08-28T00:00:00-03:00" {
		t.Errorf("since = %q", got)
	}
}

func TestReadRequestCodigosDeSaida(t *testing.T) {
	casos := []struct {
		nome string
		in   string
		want int
	}{
		{"contrato desconhecido", `{"contract":99}`, ExitIncompatible},
		{"stdin vazio", ``, ExitBadConfig},
		{"json quebrado", `{isso não é json`, ExitBadConfig},
		{"since sem offset", `{"contract":1,"since":"2026-08-28T00:00:00"}`, ExitBadConfig},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := ReadRequest(strings.NewReader(c.in))
			if err == nil {
				t.Fatal("esperava erro")
			}
			var ce *Error
			if !errors.As(err, &ce) {
				t.Fatalf("erro não carrega código: %v", err)
			}
			if ce.Code != c.want {
				t.Errorf("código = %d, esperado %d (%v)", ce.Code, c.want, err)
			}
		})
	}
}

func TestTruncateNaoParteRune(t *testing.T) {
	// "ação": a=1 byte, ç=2, ã=2, o=1. Cortar em 2 tem que voltar para 1.
	if got := Truncate("ação", 2); got != "a" {
		t.Errorf("Truncate = %q, esperado %q", got, "a")
	}
	if got := Truncate("ação", 100); got != "ação" {
		t.Errorf("string curta foi alterada: %q", got)
	}
}

func TestWriteResponseTruncaTitulo(t *testing.T) {
	var buf bytes.Buffer
	longo := strings.Repeat("a", MaxTitle+50)
	if err := WriteResponse(&buf, []Item{{Key: "k", Kind: "external", TS: "2026-08-28T00:00:00-03:00", Title: longo}}, nil); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if len(resp.Items[0].Title) != MaxTitle {
		t.Errorf("título com %d bytes, esperado %d", len(resp.Items[0].Title), MaxTitle)
	}
}

func TestWriteResponseEncolheMeta(t *testing.T) {
	var buf bytes.Buffer
	item := Item{
		Key: "k", Kind: "external", TS: "2026-08-28T00:00:00-03:00", Title: "t",
		Meta: map[string]any{"gigante": strings.Repeat("x", MaxMeta*2), "repo": "dono/nome"},
	}
	if err := WriteResponse(&buf, []Item{item}, nil); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if _, ainda := resp.Items[0].Meta["gigante"]; ainda {
		t.Error("a chave gigante deveria ter sido removida")
	}
	if resp.Items[0].Meta["repo"] != "dono/nome" {
		t.Error("a chave pequena deveria ter sobrevivido")
	}
}

func TestWriteResponseSemItens(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteResponse(&buf, nil, nil); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// items nunca pode sair como null: o motor espera uma lista.
	if !bytes.Contains(buf.Bytes(), []byte(`"items":[]`)) {
		t.Errorf("esperava items vazio como lista, veio %s", buf.String())
	}
	if bytes.Contains(buf.Bytes(), []byte("warnings")) {
		t.Error("warnings deveria ser omitido quando vazio")
	}
}
