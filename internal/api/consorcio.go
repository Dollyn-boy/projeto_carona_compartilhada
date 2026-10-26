package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

type Servidor struct {
	empresaID string
	repo      RepositorioConsorcio
}

type RepositorioConsorcio interface {
	ListarTrechosDisponiveis(data time.Time, passageiros int) ([]dominio.ArestaDisponivel, error)
}

func NovoServidor(empresaID string, repo RepositorioConsorcio) *Servidor {
	return &Servidor{
		empresaID: empresaID,
		repo:      repo,
	}
}

func (s *Servidor) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/health", s.health)
	mux.HandleFunc("/v1/trechos", s.trechos)

	return mux
}

func Iniciar(endereco, empresaID string, repo *estado.Repositorio) error {
	servidor := NovoServidor(empresaID, repo) // *estado.Repositorio é aceito aqui implicitamente!

	httpServidor := &http.Server{
		Addr:              endereco,
		Handler:           servidor.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return httpServidor.ListenAndServe()
}

func (s *Servidor) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escreverErro(w, http.StatusMethodNotAllowed, "metodo nao permitido")
		return
	}

	escreverJSON(w, http.StatusOK, protocolo.HealthResposta{
		Status:    "ok",
		EmpresaID: s.empresaID,
	})
}

func (s *Servidor) trechos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escreverErro(w, http.StatusMethodNotAllowed, "metodo nao permitido")
		return
	}

	valorData := r.URL.Query().Get("data")
	if valorData == "" {
		escreverErro(w, http.StatusBadRequest, "parametro data obrigatorio")
		return
	}

	data, err := time.ParseInLocation("2006-01-02", valorData, time.Local)
	if err != nil {
		escreverErro(w, http.StatusBadRequest, "data deve usar o formato AAAA-MM-DD")
		return
	}

	valorPassageiros := r.URL.Query().Get("passageiros")
	passageiros, err := strconv.Atoi(valorPassageiros)
	if err != nil || passageiros < 1 {
		escreverErro(w, http.StatusBadRequest, "passageiros deve ser um inteiro maior que zero")
		return
	}

	trechos, err := s.repo.ListarTrechosDisponiveis(data, passageiros)
	if err != nil {
		escreverErro(w, http.StatusBadRequest, err.Error())
		return
	}

	resposta := protocolo.TrechosDisponiveisResposta{
		EmpresaID: s.empresaID,
		Trechos:   make([]protocolo.TrechoDisponivelDados, 0, len(trechos)),
	}

	for _, aresta := range trechos {
		t := aresta.Trecho

		resposta.Trechos = append(resposta.Trechos, protocolo.TrechoDisponivelDados{
			Trecho: protocolo.TrechoDados{
				EmpresaID: t.EmpresaID,
				IDCarona:  t.IDCarona,
				Indice:    t.Indice,
				Origem:    string(t.Origem),
				Destino:   string(t.Destino),
			},
			AssentosDisponiveis: aresta.AssentosDisponiveis,
			Preco:               int(aresta.Preco),
		})
	}

	escreverJSON(w, http.StatusOK, resposta)
}

func escreverJSON(w http.ResponseWriter, status int, valor any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(valor); err != nil {
		// A resposta HTTP ja pode ter sido iniciada.
		// Registre o erro em log no servidor se necessario.
		_ = fmt.Errorf("codificar resposta JSON: %w", err)
	}
}

func escreverErro(w http.ResponseWriter, status int, mensagem string) {
	escreverJSON(w, status, protocolo.ErroREST{Erro: mensagem})
}
