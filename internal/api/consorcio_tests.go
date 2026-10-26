package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"vaijunto/internal/dominio"
)

// repositorioFake substitui o repositório real nos testes.
type repositorioFake struct {
	dataRecebida         time.Time
	passageirosRecebidos int
	chamadas             int
	erro                 error
}

func (r *repositorioFake) ListarTrechosDisponiveis(
	data time.Time,
	passageiros int,
) ([]dominio.ArestaDisponivel, error) {
	r.chamadas++
	r.dataRecebida = data
	r.passageirosRecebidos = passageiros

	if r.erro != nil {
		return nil, r.erro
	}

	return []dominio.ArestaDisponivel{}, nil
}

func novoHandler(repo *repositorioFake) http.Handler {
	return NovoServidor("empresa-a", repo).Handler()
}

// TestHealth verifica o endpoint de identificação da empresa.
func TestHealth(t *testing.T) {
	handler := novoHandler(&repositorioFake{})

	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/health",
		nil,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status esperado %d; obtido %d",
			http.StatusOK,
			rec.Code,
		)
	}

	var resposta struct {
		Status    string `json:"status"`
		EmpresaID string `json:"empresa_id"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resposta); err != nil {
		t.Fatalf("resposta JSON inválida: %v", err)
	}

	if resposta.Status != "ok" {
		t.Errorf("status esperado %q; obtido %q", "ok", resposta.Status)
	}

	if resposta.EmpresaID != "empresa-a" {
		t.Errorf(
			"empresa esperada %q; obtida %q",
			"empresa-a",
			resposta.EmpresaID,
		)
	}
}

// TestTrechosParametrosInvalidos verifica entradas que devem
// ser recusadas antes de consultar os trechos.
func TestTrechosParametrosInvalidos(t *testing.T) {
	casos := []struct {
		nome string
		url  string
	}{
		{
			nome: "data ausente",
			url:  "/v1/trechos?passageiros=2",
		},
		{
			nome: "data em formato inválido",
			url:  "/v1/trechos?data=15-11-2026&passageiros=2",
		},
		{
			nome: "passageiros ausente",
			url:  "/v1/trechos?data=2099-11-15",
		},
		{
			nome: "passageiros igual a zero",
			url:  "/v1/trechos?data=2099-11-15&passageiros=0",
		},
		{
			nome: "passageiros não numérico",
			url:  "/v1/trechos?data=2099-11-15&passageiros=abc",
		},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			repo := &repositorioFake{}
			handler := novoHandler(repo)

			req := httptest.NewRequest(
				http.MethodGet,
				tc.url,
				nil,
			)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf(
					"esperado HTTP 400; obtido %d: %s",
					rec.Code,
					rec.Body.String(),
				)
			}

			if repo.chamadas != 0 {
				t.Errorf(
					"o repositório não deveria ser consultado; chamadas=%d",
					repo.chamadas,
				)
			}
		})
	}
}

// TestTrechosConsultaRepositorio verifica se os parâmetros
// HTTP chegam corretamente ao repositório.
func TestTrechosConsultaRepositorio(t *testing.T) {
	repo := &repositorioFake{}
	handler := novoHandler(repo)

	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/trechos?data=2099-11-15&passageiros=2",
		nil,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"esperado HTTP 200; obtido %d: %s",
			rec.Code,
			rec.Body.String(),
		)
	}

	if repo.chamadas != 1 {
		t.Fatalf(
			"esperada uma chamada ao repositório; obtidas %d",
			repo.chamadas,
		)
	}

	dataEsperada, _ := time.Parse("2006-01-02", "2099-11-15")

	if !repo.dataRecebida.Equal(dataEsperada) {
		t.Errorf(
			"data esperada %v; obtida %v",
			dataEsperada,
			repo.dataRecebida,
		)
	}

	if repo.passageirosRecebidos != 2 {
		t.Errorf(
			"esperados 2 passageiros; obtido %d",
			repo.passageirosRecebidos,
		)
	}

	var resposta struct {
		EmpresaID string            `json:"empresa_id"`
		Trechos   []json.RawMessage `json:"trechos"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resposta); err != nil {
		t.Fatalf("resposta JSON inválida: %v", err)
	}

	if resposta.EmpresaID != "empresa-a" {
		t.Errorf("empresa inesperada: %s", resposta.EmpresaID)
	}

	if resposta.Trechos == nil {
		t.Error("a resposta deveria apresentar uma lista de trechos")
	}
}

// TestHealthMetodoNaoPermitido verifica a rejeição de POST
// em um endpoint definido como GET.
func TestHealthMetodoNaoPermitido(t *testing.T) {
	handler := novoHandler(&repositorioFake{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/health",
		nil,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf(
			"esperado HTTP 405; obtido %d",
			rec.Code,
		)
	}
}

// TestTrechosFalhaRepositorio verifica o tratamento de falhas
// inesperadas ao consultar o repositório.
func TestTrechosFalhaRepositorio(t *testing.T) {
	repo := &repositorioFake{
		erro: errors.New("falha simulada"),
	}
	handler := novoHandler(repo)

	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/trechos?data=2099-11-15&passageiros=2",
		nil,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(
			"esperado HTTP 500; obtido %d: %s",
			rec.Code,
			rec.Body.String(),
		)
	}
}
