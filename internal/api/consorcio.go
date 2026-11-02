package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vaijunto/internal/consorcio"
	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

type Servidor struct {
	empresaID  string
	repo       RepositorioConsorcio
	transacoes RepositorioTransacoes
	peers      map[string]bool
}

type RepositorioConsorcio interface {
	ListarTrechosDisponiveis(data time.Time, passageiros int) ([]dominio.ArestaDisponivel, error)
}

type RepositorioTransacoes interface {
	PrepararTransacao(id, passageiro string, itinerario dominio.Itinerario) (estado.TransacaoLocal, error)
	MarcarTransacaoCoordenada(id string) error
	ConfirmarTransacao(id string) (dominio.Reserva, error)
	AbortarTransacao(id string) error
	ConsultarTransacao(id string) (estado.TransacaoLocal, error)
}

func NovoServidor(empresaID string, repo RepositorioConsorcio) *Servidor {
	servidor := &Servidor{
		empresaID: empresaID,
		repo:      repo,
		peers:     make(map[string]bool),
	}
	if transacoes, ok := repo.(RepositorioTransacoes); ok {
		servidor.transacoes = transacoes
	}
	return servidor
}

// NovoServidorComPeers registra quais empresas podem enviar comandos de
// transação para esta instância. A validação é adequada para a rede privada
// do consórcio; não substitui autenticação criptográfica em redes hostis.
func NovoServidorComPeers(empresaID string, repo RepositorioConsorcio, peers []consorcio.Peer) *Servidor {
	servidor := NovoServidor(empresaID, repo)
	for _, peer := range peers {
		servidor.peers[peer.EmpresaID] = true
	}
	return servidor
}

func (s *Servidor) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/health", s.health)
	mux.HandleFunc("/v1/trechos", s.trechos)
	mux.HandleFunc("POST /v1/transacoes/{id}/preparar", s.prepararTransacao)
	mux.HandleFunc("POST /v1/transacoes/{id}/confirmar", s.confirmarTransacao)
	mux.HandleFunc("POST /v1/transacoes/{id}/abortar", s.abortarTransacao)
	mux.HandleFunc("GET /v1/transacoes/{id}", s.consultarTransacao)

	return mux
}

func Iniciar(endereco, empresaID string, repo *estado.Repositorio) error {
	return IniciarComPeers(endereco, empresaID, repo, nil)
}

func IniciarComPeers(endereco, empresaID string, repo *estado.Repositorio, peers []consorcio.Peer) error {
	return IniciarComPeersContext(context.Background(), endereco, empresaID, repo, peers)
}

// IniciarComPeersContext inicia a API REST e executa Shutdown quando ctx é
// cancelado, aguardando handlers ativos terminarem antes de retornar.
func IniciarComPeersContext(
	ctx context.Context,
	endereco, empresaID string,
	repo *estado.Repositorio,
	peers []consorcio.Peer,
) error {
	if ctx == nil {
		return fmt.Errorf("contexto nao pode ser nil")
	}
	servidor := NovoServidorComPeers(empresaID, repo, peers)
	httpServidor := &http.Server{
		Addr:              endereco,
		Handler:           servidor.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		return fmt.Errorf("falha ao iniciar API REST em %s: %w", endereco, err)
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServidor.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		ctxShutdown, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelar()
		if err := httpServidor.Shutdown(ctxShutdown); err != nil {
			_ = httpServidor.Close()
			return fmt.Errorf("encerrar API REST: %w", err)
		}
		err := <-serveErr
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
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
		if errors.Is(err, estado.ErrParametroInvalido) {
			escreverErro(w, http.StatusBadRequest, err.Error())
			return
		}
		escreverErro(w, http.StatusInternalServerError, "falha interna ao consultar trechos")
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

// autorizarCoordenadora aceita somente comandos enviados por uma empresa
// cadastrada na lista de peers desta instância.
func (s *Servidor) autorizarCoordenadora(w http.ResponseWriter, r *http.Request) bool {
	coordenadora := strings.TrimSpace(r.Header.Get("X-VaiJunto-Empresa"))
	if coordenadora == "" || !s.peers[coordenadora] {
		escreverErro(w, http.StatusForbidden, "empresa coordenadora nao autorizada")
		return false
	}
	return true
}

func (s *Servidor) prepararTransacao(w http.ResponseWriter, r *http.Request) {
	if s.transacoes == nil {
		escreverErro(w, http.StatusServiceUnavailable, "operacoes de transacao nao estao disponiveis")
		return
	}
	if !s.autorizarCoordenadora(w, r) {
		return
	}

	var dados protocolo.PrepararTransacaoDados
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&dados); err != nil {
		escreverErro(w, http.StatusBadRequest, "corpo JSON invalido: "+err.Error())
		return
	}
	if strings.TrimSpace(dados.Passageiro) == "" || len(dados.Trechos) == 0 {
		escreverErro(w, http.StatusBadRequest, "passageiro e trechos sao obrigatorios")
		return
	}

	itinerario := dominio.Itinerario{Passos: make([]dominio.Trecho, len(dados.Trechos))}
	for i, trecho := range dados.Trechos {
		itinerario.Passos[i] = dominio.Trecho{
			EmpresaID: trecho.EmpresaID,
			IDCarona:  trecho.IDCarona,
			Indice:    trecho.Indice,
			Origem:    dominio.Cidade(trecho.Origem),
			Destino:   dominio.Cidade(trecho.Destino),
		}
	}

	id := strings.TrimSpace(r.PathValue("id"))
	transacao, err := s.transacoes.PrepararTransacao(id, dados.Passageiro, itinerario)
	if err != nil {
		escreverErro(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.transacoes.MarcarTransacaoCoordenada(id); err != nil {
		escreverErro(w, http.StatusConflict, err.Error())
		return
	}
	escreverJSON(w, http.StatusOK, protocolo.TransacaoParticipanteResposta{
		TransacaoID: id,
		EmpresaID:   s.empresaID,
		Estado:      string(transacao.Estado),
	})
}

func (s *Servidor) confirmarTransacao(w http.ResponseWriter, r *http.Request) {
	if s.transacoes == nil {
		escreverErro(w, http.StatusServiceUnavailable, "operacoes de transacao nao estao disponiveis")
		return
	}
	if !s.autorizarCoordenadora(w, r) {
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	reserva, err := s.transacoes.ConfirmarTransacao(id)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, estado.ErrTransacaoNaoEncontrada) {
			status = http.StatusNotFound
		}
		escreverErro(w, status, err.Error())
		return
	}
	escreverJSON(w, http.StatusOK, protocolo.TransacaoParticipanteResposta{
		TransacaoID: id,
		EmpresaID:   s.empresaID,
		Estado:      string(estado.EstadoConfirmada),
		ReservaID:   reserva.Id,
	})
}

func (s *Servidor) abortarTransacao(w http.ResponseWriter, r *http.Request) {
	if s.transacoes == nil {
		escreverErro(w, http.StatusServiceUnavailable, "operacoes de transacao nao estao disponiveis")
		return
	}
	if !s.autorizarCoordenadora(w, r) {
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.transacoes.AbortarTransacao(id); err != nil {
		escreverErro(w, http.StatusConflict, err.Error())
		return
	}
	escreverJSON(w, http.StatusOK, protocolo.TransacaoParticipanteResposta{
		TransacaoID: id,
		EmpresaID:   s.empresaID,
		Estado:      string(estado.EstadoAbortada),
	})
}

func (s *Servidor) consultarTransacao(w http.ResponseWriter, r *http.Request) {
	if s.transacoes == nil {
		escreverErro(w, http.StatusServiceUnavailable, "operacoes de transacao nao estao disponiveis")
		return
	}
	if !s.autorizarCoordenadora(w, r) {
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	transacao, err := s.transacoes.ConsultarTransacao(id)
	if err != nil {
		if errors.Is(err, estado.ErrTransacaoNaoEncontrada) {
			escreverErro(w, http.StatusNotFound, err.Error())
		} else {
			escreverErro(w, http.StatusInternalServerError, "falha ao consultar transacao")
		}
		return
	}
	escreverJSON(w, http.StatusOK, protocolo.TransacaoEstadoResposta{
		TransacaoID: id,
		EmpresaID:   s.empresaID,
		Estado:      string(transacao.Estado),
		ReservaID:   transacao.ReservaID,
	})
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
