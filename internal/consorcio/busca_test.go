package consorcio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// trechoRemoto monta um item no formato que a API REST devolve.
func trechoRemoto(empresa, idCarona string, indice int, origem, destino string) protocolo.TrechoDisponivelDados {
	return protocolo.TrechoDisponivelDados{
		Trecho: protocolo.TrechoDados{
			EmpresaID: empresa,
			IDCarona:  idCarona,
			Indice:    indice,
			Origem:    origem,
			Destino:   destino,
		},
		AssentosDisponiveis: 3,
		Preco:               20,
	}
}

// peerFalso sobe um servidor HTTP que se comporta como a API REST de outra
// empresa. "identidade" e o empresa_id que ele declara na resposta (pode ser
// diferente do esperado, para testar a validacao).
func peerFalso(t *testing.T, identidade string, trechos ...protocolo.TrechoDisponivelDados) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/trechos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(protocolo.TrechosDisponiveisResposta{
			EmpresaID: identidade,
			Trechos:   trechos,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// repoLocal cria a "empresa-a" com uma carona Riachao -> Feira de Santana.
func repoLocal(t *testing.T, data time.Time) *estado.Repositorio {
	t.Helper()

	repo, err := estado.NovoRepositorioDeEmpresa("empresa-a")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.PublicarCarona(
		"motorista-a",
		[]dominio.Cidade{"Riachao", "Feira de Santana"},
		3, 10, data,
	)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func dataFutura() time.Time { return time.Now().Add(48 * time.Hour) }

// empresasDoItinerario devolve a sequencia de empresas que atendem cada passo.
func empresasDoItinerario(it dominio.Itinerario) []string {
	var out []string
	for _, p := range it.Passos {
		out = append(out, p.EmpresaID)
	}
	return out
}

// ---------------------------------------------------------------------------
// Testes
// ---------------------------------------------------------------------------

// Caso feliz: A (local) + B + C formam um unico itinerario de 3 trechos.
func TestBuscarCombinaTrechosDeTresEmpresas(t *testing.T) {
	data := dataFutura()
	repo := repoLocal(t, data)

	b := peerFalso(t, "empresa-b", trechoRemoto("empresa-b", "7", 0, "Feira de Santana", "Salvador"))
	c := peerFalso(t, "empresa-c", trechoRemoto("empresa-c", "9", 0, "Salvador", "Ilheus"))

	busca := NovoBuscaDistribuida(repo, []Peer{
		{EmpresaID: "empresa-b", URL: b.URL},
		{EmpresaID: "empresa-c", URL: c.URL},
	})

	res, err := busca.Buscar(context.Background(), "Riachao", "Ilheus", data, 1)
	if err != nil {
		t.Fatal(err)
	}

	if !res.BuscaCompleta {
		t.Errorf("busca deveria ser completa; indisponiveis=%v", res.EmpresasIndisponiveis)
	}
	if len(res.Itinerarios) != 1 {
		t.Fatalf("esperado 1 itinerario, obtido %d: %+v", len(res.Itinerarios), res.Itinerarios)
	}

	got := empresasDoItinerario(res.Itinerarios[0])
	want := []string{"empresa-a", "empresa-b", "empresa-c"}
	if len(got) != len(want) {
		t.Fatalf("empresas do itinerario = %v; esperado %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("empresas do itinerario = %v; esperado %v", got, want)
		}
	}
}

// Verifica o que realmente sai na rede: caminho, metodo e query string.
func TestBuscarEnviaRequisicaoCorretaAoPeer(t *testing.T) {
	data := dataFutura()
	repo := repoLocal(t, data)

	var (
		mu       sync.Mutex
		metodo   string
		caminho  string
		dataQS   string
		passQS   string
		chamadas int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		metodo, caminho = r.Method, r.URL.Path
		dataQS, passQS = r.URL.Query().Get("data"), r.URL.Query().Get("passageiros")
		chamadas++
		mu.Unlock()

		_ = json.NewEncoder(w).Encode(protocolo.TrechosDisponiveisResposta{EmpresaID: "empresa-b"})
	}))
	defer srv.Close()

	// A barra final na URL nao pode gerar "//v1/trechos".
	busca := NovoBuscaDistribuida(repo, []Peer{{EmpresaID: "empresa-b", URL: srv.URL + "/"}})

	if _, err := busca.Buscar(context.Background(), "Riachao", "Salvador", data, 2); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if chamadas != 1 {
		t.Fatalf("esperada 1 chamada ao peer; obtidas %d", chamadas)
	}
	if metodo != http.MethodGet || caminho != "/v1/trechos" {
		t.Errorf("requisicao inesperada: %s %s", metodo, caminho)
	}
	if dataQS != data.Format("2006-01-02") {
		t.Errorf("data=%q; esperado %q", dataQS, data.Format("2006-01-02"))
	}
	if passQS != "2" {
		t.Errorf("passageiros=%q; esperado 2", passQS)
	}
}

// Peer fora do ar: a busca continua, mas avisa que esta incompleta.
func TestBuscaToleraPeerForaDoAr(t *testing.T) {
	data := dataFutura()
	repo := repoLocal(t, data)

	b := peerFalso(t, "empresa-b", trechoRemoto("empresa-b", "7", 0, "Feira de Santana", "Salvador"))
	c := peerFalso(t, "empresa-c")
	urlC := c.URL
	c.Close() // derruba a empresa C antes da busca

	busca := NovoBuscaDistribuida(repo, []Peer{
		{EmpresaID: "empresa-b", URL: b.URL},
		{EmpresaID: "empresa-c", URL: urlC},
	})

	res, err := busca.Buscar(context.Background(), "Riachao", "Salvador", data, 1)
	if err != nil {
		t.Fatalf("falha de um peer nao deve derrubar a busca: %v", err)
	}
	if res.BuscaCompleta {
		t.Error("BuscaCompleta deveria ser false")
	}
	if len(res.EmpresasIndisponiveis) != 1 || res.EmpresasIndisponiveis[0] != "empresa-c" {
		t.Errorf("EmpresasIndisponiveis = %v; esperado [empresa-c]", res.EmpresasIndisponiveis)
	}
	// Mesmo sem C, o caminho A -> B continua valido.
	if len(res.Itinerarios) != 1 {
		t.Errorf("esperado 1 itinerario (A+B); obtido %d", len(res.Itinerarios))
	}
}

// Peer lento: o timeout do cliente HTTP precisa cortar a espera.
func TestBuscaToleraPeerLento(t *testing.T) {
	data := dataFutura()
	repo := repoLocal(t, data)

	lento := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(protocolo.TrechosDisponiveisResposta{EmpresaID: "empresa-b"})
	}))
	defer lento.Close()

	busca := NovoBuscaDistribuida(repo, []Peer{{EmpresaID: "empresa-b", URL: lento.URL}})
	busca.HTTP = &http.Client{Timeout: 100 * time.Millisecond}

	inicio := time.Now()
	res, err := busca.Buscar(context.Background(), "Riachao", "Feira de Santana", data, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(inicio); d > 350*time.Millisecond {
		t.Errorf("a busca esperou %v; o timeout de 100ms nao foi respeitado", d)
	}
	if res.BuscaCompleta || len(res.EmpresasIndisponiveis) != 1 {
		t.Errorf("peer lento deveria ser marcado como indisponivel: %+v", res)
	}
}

// Respostas invalidas de peers devem ser descartadas por inteiro.
func TestBuscaDescartaRespostasInvalidas(t *testing.T) {
	data := dataFutura()

	casos := []struct {
		nome    string
		handler http.HandlerFunc
	}{
		{
			nome: "HTTP 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"erro":"boom"}`, http.StatusInternalServerError)
			},
		},
		{
			nome: "HTTP 400",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"erro":"data invalida"}`, http.StatusBadRequest)
			},
		},
		{
			nome: "JSON quebrado",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"empresa_id": "empresa-b", "trechos": [`))
			},
		},
		{
			nome: "identidade diferente da configurada",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(protocolo.TrechosDisponiveisResposta{
					EmpresaID: "empresa-x",
					Trechos: []protocolo.TrechoDisponivelDados{
						trechoRemoto("empresa-x", "1", 0, "Feira de Santana", "Salvador"),
					},
				})
			},
		},
		{
			nome: "trecho de outra empresa dentro da resposta",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(protocolo.TrechosDisponiveisResposta{
					EmpresaID: "empresa-b",
					Trechos: []protocolo.TrechoDisponivelDados{
						trechoRemoto("empresa-c", "1", 0, "Feira de Santana", "Salvador"),
					},
				})
			},
		},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			repo := repoLocal(t, data)
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			busca := NovoBuscaDistribuida(repo, []Peer{{EmpresaID: "empresa-b", URL: srv.URL}})

			res, err := busca.Buscar(context.Background(), "Riachao", "Salvador", data, 1)
			if err != nil {
				t.Fatal(err)
			}
			if res.BuscaCompleta {
				t.Error("BuscaCompleta deveria ser false")
			}
			if len(res.EmpresasIndisponiveis) != 1 || res.EmpresasIndisponiveis[0] != "empresa-b" {
				t.Errorf("EmpresasIndisponiveis = %v", res.EmpresasIndisponiveis)
			}
			// Nada do peer defeituoso pode entrar no grafo: Salvador e inalcancavel.
			if len(res.Itinerarios) != 0 {
				t.Errorf("nenhum itinerario ate Salvador era esperado; obtido %+v", res.Itinerarios)
			}
		})
	}
}

// Sem peers configurados, a busca e puramente local e completa.
func TestBuscaSemPeersEhLocalECompleta(t *testing.T) {
	data := dataFutura()
	repo := repoLocal(t, data)

	res, err := NovoBuscaDistribuida(repo, nil).Buscar(
		context.Background(), "Riachao", "Feira de Santana", data, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.BuscaCompleta || len(res.Itinerarios) != 1 {
		t.Errorf("resultado inesperado: %+v", res)
	}
}
