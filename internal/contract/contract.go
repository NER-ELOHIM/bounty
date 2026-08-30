// Package contract implementa o contrato de adaptador do iode, versão 1.
//
// Ver docs/CONTRATOS.md no repositório do motor: requisição JSON no stdin,
// resposta JSON no stdout, resultado no código de saída.
package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
	"unicode/utf8"
)

// Version is the adapter contract this binary speaks.
const Version = 1

// Exit codes defined by the contract.
const (
	ExitOK           = 0 // sucesso
	ExitRetryable    = 1 // erro recuperável, o motor tenta de novo
	ExitIncompatible = 2 // versão de contrato desconhecida
	ExitBadConfig    = 3 // configuração inválida, falha permanente
)

// Limits the engine enforces. We truncate before it has to.
const (
	MaxTitle = 500
	MaxBody  = 100 * 1024
	MaxMeta  = 16 * 1024
)

// Request is what the engine writes to our stdin.
type Request struct {
	Contract int             `json:"contract"`
	Project  string          `json:"project"`
	Path     string          `json:"path"`
	Since    *time.Time      `json:"since"`
	Config   json.RawMessage `json:"config"`
}

// Item is one collected entry.
type Item struct {
	Key   string         `json:"key"`
	Kind  string         `json:"kind"`
	TS    string         `json:"ts"`
	Title string         `json:"title"`
	Body  string         `json:"body,omitempty"`
	Meta  map[string]any `json:"meta,omitempty"`
}

// Response is what we write to stdout.
type Response struct {
	Contract int      `json:"contract"`
	Items    []Item   `json:"items"`
	Warnings []string `json:"warnings,omitempty"`
}

// Error carries the exit code the contract asks for in each failure mode.
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func errf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

// ReadRequest parses and validates the request on r.
func ReadRequest(r io.Reader) (*Request, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return nil, errf(ExitRetryable, "lendo stdin: %w", err)
	}
	if len(raw) == 0 {
		return nil, errf(ExitBadConfig, "requisição vazia no stdin")
	}

	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		// Um `since` sem offset chega aqui: o parser de time.Time do
		// encoding/json exige RFC 3339 completo, que é o que queremos.
		return nil, errf(ExitBadConfig, "requisição inválida: %w", err)
	}

	// Versão desconhecida é código 2: o motor desabilita o adaptador em vez
	// de ficar retentando algo que nunca vai funcionar.
	if req.Contract != Version {
		return nil, errf(ExitIncompatible, "contrato %d incompatível, este adaptador fala %d", req.Contract, Version)
	}

	return &req, nil
}

// WriteResponse serialises the response, guarding the contract's size limits.
func WriteResponse(w io.Writer, items []Item, warnings []string) error {
	if items == nil {
		items = []Item{}
	}
	for i := range items {
		items[i].Title = Truncate(items[i].Title, MaxTitle)
		items[i].Body = Truncate(items[i].Body, MaxBody)
		if err := shrinkMeta(&items[i]); err != nil {
			return errf(ExitRetryable, "item %s: %w", items[i].Key, err)
		}
	}

	out, err := json.Marshal(Response{Contract: Version, Items: items, Warnings: warnings})
	if err != nil {
		return errf(ExitRetryable, "serializando resposta: %w", err)
	}
	if _, err := w.Write(append(out, '\n')); err != nil {
		return errf(ExitRetryable, "escrevendo stdout: %w", err)
	}
	return nil
}

// shrinkMeta drops the largest keys until meta fits the contract's budget.
func shrinkMeta(item *Item) error {
	for {
		encoded, err := json.Marshal(item.Meta)
		if err != nil {
			return fmt.Errorf("serializando meta: %w", err)
		}
		if len(encoded) <= MaxMeta {
			return nil
		}
		var widest string
		var widestSize int
		for k, v := range item.Meta {
			b, _ := json.Marshal(v) // erro já capturado acima, no meta inteiro
			if len(b) > widestSize {
				widest, widestSize = k, len(b)
			}
		}
		if widest == "" {
			return fmt.Errorf("meta não cabe em %d bytes e não há chave a remover", MaxMeta)
		}
		delete(item.Meta, widest)
	}
}

// Truncate cuts to maxBytes without splitting a rune.
func Truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// FormatTS renders t in RFC 3339 with an offset, which the contract requires.
func FormatTS(t time.Time) string { return t.Format(time.RFC3339) }
