package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"vaijunto/internal/consorcio"
	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

func TestEndpointsParticipanteTransacao(t *testing.T) {
	repo, err := estado.NovoRepositorioDeEmpresa("empresa-b")
	if err != nil {
		t.Fatal(err)
	}
	carona, err := repo.PublicarCarona("motorista-b", []dominio.Cidade{"B", "C"}, 1, 20, time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	trecho := dominio.TrechosDaRota("empresa-b", strconv.Itoa(carona.Id), carona.Rota)[0]

	handler := NovoServidorComPeers("empresa-b", repo, []consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}}).Handler()
	id := "empresa-a-1"
	dados, _ := json.Marshal(protocolo.PrepararTransacaoDados{
		Passageiro: "ana",
		Trechos: []protocolo.TrechoDados{{
			EmpresaID: trecho.EmpresaID, IDCarona: trecho.IDCarona,
			Indice: trecho.Indice, Origem: string(trecho.Origem), Destino: string(trecho.Destino),
		}},
	})

	preparar := httptest.NewRequest(http.MethodPost, "/v1/transacoes/"+id+"/preparar", bytes.NewReader(dados))
	preparar.Header.Set("X-VaiJunto-Empresa", "empresa-a")
	prepararRec := httptest.NewRecorder()
	handler.ServeHTTP(prepararRec, preparar)
	if prepararRec.Code != http.StatusOK {
		t.Fatalf("preparar: HTTP %d: %s", prepararRec.Code, prepararRec.Body.String())
	}

	// A repetição não desconta o assento novamente.
	preparar2 := httptest.NewRequest(http.MethodPost, "/v1/transacoes/"+id+"/preparar", bytes.NewReader(dados))
	preparar2.Header.Set("X-VaiJunto-Empresa", "empresa-a")
	prepararRec2 := httptest.NewRecorder()
	handler.ServeHTTP(prepararRec2, preparar2)
	if prepararRec2.Code != http.StatusOK {
		t.Fatalf("preparar repetido: HTTP %d: %s", prepararRec2.Code, prepararRec2.Body.String())
	}

	confirmar := httptest.NewRequest(http.MethodPost, "/v1/transacoes/"+id+"/confirmar", nil)
	confirmar.Header.Set("X-VaiJunto-Empresa", "empresa-a")
	confirmarRec := httptest.NewRecorder()
	handler.ServeHTTP(confirmarRec, confirmar)
	if confirmarRec.Code != http.StatusOK {
		t.Fatalf("confirmar: HTTP %d: %s", confirmarRec.Code, confirmarRec.Body.String())
	}

	abortar := httptest.NewRequest(http.MethodPost, "/v1/transacoes/"+id+"/abortar", nil)
	abortar.Header.Set("X-VaiJunto-Empresa", "empresa-a")
	abortarRec := httptest.NewRecorder()
	handler.ServeHTTP(abortarRec, abortar)
	if abortarRec.Code != http.StatusConflict {
		t.Fatalf("abortar transacao confirmada: esperado 409, obtido %d", abortarRec.Code)
	}

	consultar := httptest.NewRequest(http.MethodGet, "/v1/transacoes/"+id, nil)
	consultar.Header.Set("X-VaiJunto-Empresa", "empresa-a")
	consultarRec := httptest.NewRecorder()
	handler.ServeHTTP(consultarRec, consultar)
	if consultarRec.Code != http.StatusOK {
		t.Fatalf("consultar: HTTP %d: %s", consultarRec.Code, consultarRec.Body.String())
	}

	disponiveis, err := repo.ListarTrechosDisponiveis(time.Now().Add(48*time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range disponiveis {
		if item.Trecho.IDCarona == strconv.Itoa(carona.Id) {
			t.Fatal("o trecho confirmado não deveria continuar disponível")
		}
	}
	reservas, err := repo.ConsultarReservas("ana")
	if err != nil {
		t.Fatal(err)
	}
	if len(reservas) != 1 {
		t.Fatalf("esperava uma reserva definitiva; obtidas %d", len(reservas))
	}
}

func TestEndpointTransacaoRejeitaCoordenadoraNaoAutorizada(t *testing.T) {
	repo, err := estado.NovoRepositorioDeEmpresa("empresa-b")
	if err != nil {
		t.Fatal(err)
	}
	handler := NovoServidorComPeers("empresa-b", repo, []consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/transacoes/tx-1/abortar", nil)
	req.Header.Set("X-VaiJunto-Empresa", "empresa-c")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperado HTTP 403; obtido %d", rec.Code)
	}
}
