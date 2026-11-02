package consorcio_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vaijunto/internal/api"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
)

func criarTrechoParaCoordenacao(t *testing.T, repo *estado.Repositorio, origem, destino dominio.Cidade, capacidade int) dominio.Trecho {
	t.Helper()
	carona, err := repo.PublicarCarona("motorista", []dominio.Cidade{origem, destino}, capacidade, 25, time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return dominio.TrechosDaRota(repo.EmpresaID(), strconv.Itoa(carona.Id), carona.Rota)[0]
}

func TestCoordenadorReservaEntreDuasEmpresas(t *testing.T) {
	repoA, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	repoB, _ := estado.NovoRepositorioDeEmpresa("empresa-b")
	trechoA := criarTrechoParaCoordenacao(t, repoA, "Feira", "Alagoinhas", 1)
	trechoB := criarTrechoParaCoordenacao(t, repoB, "Alagoinhas", "Salvador", 1)

	handlerB := api.NovoServidorComPeers("empresa-b", repoB, []consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}}).Handler()
	servidorB := httptest.NewServer(handlerB)
	defer servidorB.Close()

	coordenadorA := consorcio.NovoCoordenador(repoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})
	resultado, err := coordenadorA.Reservar(context.Background(), "ana", dominio.Itinerario{Passos: []dominio.Trecho{trechoA, trechoB}})
	if err != nil {
		t.Fatalf("reservar: %v", err)
	}
	if resultado.Estado != "CONFIRMADA" {
		t.Fatalf("esperava CONFIRMADA; obtido %+v", resultado)
	}
	if len(resultado.Reservas) != 2 {
		t.Fatalf("esperava duas reservas locais, obtidas %d", len(resultado.Reservas))
	}

	reservasA, _ := repoA.ConsultarReservas("ana")
	reservasB, _ := repoB.ConsultarReservas("ana")
	if len(reservasA) != 1 || len(reservasB) != 1 {
		t.Fatalf("reservas por empresa incorretas: A=%d B=%d", len(reservasA), len(reservasB))
	}
	// O cancelamento local de apenas uma parte do itinerario distribuido
	// deve ser recusado pelos dois participantes, evitando uma viagem parcial.
	if err := repoA.CancelarReserva(reservasA[0].Id, "ana"); err == nil {
		t.Fatal("empresa A nao deveria cancelar isoladamente uma reserva coordenada")
	}
	if err := repoB.CancelarReserva(reservasB[0].Id, "ana"); err == nil {
		t.Fatal("empresa B nao deveria cancelar isoladamente uma reserva coordenada")
	}

	log, err := repoA.ConsultarTransacaoCoordenada(resultado.TransacaoID)
	if err != nil {
		t.Fatal(err)
	}
	if log.Decisao != estado.DecisaoConfirmar || log.Estado != estado.CoordenacaoConcluida {
		t.Fatalf("log da coordenadora não registra commit concluído: %+v", log)
	}
}

func TestCoordenadorAbortaSeUmParticipanteRecusarPreparacao(t *testing.T) {
	repoA, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	repoB, _ := estado.NovoRepositorioDeEmpresa("empresa-b")
	trechoA := criarTrechoParaCoordenacao(t, repoA, "Feira", "Alagoinhas", 1)
	trechoB := criarTrechoParaCoordenacao(t, repoB, "Alagoinhas", "Salvador", 1)
	if _, err := repoB.ConfirmarReserva("outro-passageiro", dominio.Itinerario{Passos: []dominio.Trecho{trechoB}}); err != nil {
		t.Fatal(err)
	}

	handlerB := api.NovoServidorComPeers("empresa-b", repoB, []consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}}).Handler()
	servidorB := httptest.NewServer(handlerB)
	defer servidorB.Close()
	coordenadorA := consorcio.NovoCoordenador(repoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})

	resultado, err := coordenadorA.Reservar(context.Background(), "ana", dominio.Itinerario{Passos: []dominio.Trecho{trechoA, trechoB}})
	if err != nil {
		t.Fatalf("a decisão de aborto deveria ser distribuída: %v", err)
	}
	if resultado.Estado != "ABORTADA" {
		t.Fatalf("esperava ABORTADA; obtido %+v", resultado)
	}

	local, err := repoA.ConsultarTransacao(resultado.TransacaoID)
	if err != nil {
		t.Fatal(err)
	}
	if local.Estado != estado.EstadoAbortada {
		t.Fatalf("trecho da empresa A não foi liberado: %+v", local)
	}
	remoto, err := repoB.ConsultarTransacao(resultado.TransacaoID)
	if err != nil {
		t.Fatal(err)
	}
	if remoto.Estado != estado.EstadoAbortada {
		t.Fatalf("empresa B não registrou tombstone de aborto: %+v", remoto)
	}
}

func TestCoordenadorPersisteConfirmacaoEPermiteRetomar(t *testing.T) {
	repoA, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	repoB, _ := estado.NovoRepositorioDeEmpresa("empresa-b")
	trechoA := criarTrechoParaCoordenacao(t, repoA, "A", "B", 1)
	trechoB := criarTrechoParaCoordenacao(t, repoB, "B", "C", 1)

	falharConfirmacao := atomic.Bool{}
	falharConfirmacao.Store(true)
	baseHandler := api.NovoServidorComPeers("empresa-b", repoB, []consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}}).Handler()
	servidorB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transacoes/empresa-a-1/confirmar" && falharConfirmacao.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"erro":"falha temporaria simulada"}`))
			return
		}
		baseHandler.ServeHTTP(w, r)
	}))
	defer servidorB.Close()

	coordenador := consorcio.NovoCoordenador(repoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})
	itinerario := dominio.Itinerario{Passos: []dominio.Trecho{trechoA, trechoB}}
	pendente, err := coordenador.Reservar(context.Background(), "ana", itinerario)
	if err != nil {
		t.Fatalf("a confirmação pendente deve ser representada no resultado: %v", err)
	}
	if pendente.Estado != "PENDENTE_CONFIRMAR" {
		t.Fatalf("esperava PENDENTE_CONFIRMAR; obtido %+v", pendente)
	}

	log, err := repoA.ConsultarTransacaoCoordenada(pendente.TransacaoID)
	if err != nil {
		t.Fatal(err)
	}
	if log.Decisao != estado.DecisaoConfirmar {
		t.Fatalf("decisão CONFIRMAR não foi persistida: %+v", log)
	}
	if _, err := repoA.ConsultarTransacao(pendente.TransacaoID); err != nil {
		t.Fatalf("empresa A deveria já ter confirmado: %v", err)
	}

	falharConfirmacao.Store(false)
	concluida, err := coordenador.RetomarDecisao(context.Background(), pendente.TransacaoID)
	if err != nil {
		t.Fatal(err)
	}
	if concluida.Estado != "CONFIRMADA" {
		t.Fatalf("retomada não concluiu a transação: %+v", concluida)
	}
	reservasB, _ := repoB.ConsultarReservas("ana")
	if len(reservasB) != 1 {
		t.Fatalf("esperava uma reserva na empresa B, obtidas %d", len(reservasB))
	}
}

func TestDoisCoordenadoresDisputamUltimoAssentoSemVendaDuplicada(t *testing.T) {
	repoA, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	repoC, _ := estado.NovoRepositorioDeEmpresa("empresa-c")
	repoB, _ := estado.NovoRepositorioDeEmpresa("empresa-b")
	trechoB := criarTrechoParaCoordenacao(t, repoB, "B", "C", 1)

	peersB := []consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}, {EmpresaID: "empresa-c", URL: "http://empresa-c"}}
	servidorB := httptest.NewServer(api.NovoServidorComPeers("empresa-b", repoB, peersB).Handler())
	defer servidorB.Close()
	coordA := consorcio.NovoCoordenador(repoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})
	coordC := consorcio.NovoCoordenador(repoC, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})

	inicio := make(chan struct{})
	resultados := make(chan protocoloResultadoTeste, 2)
	var wg sync.WaitGroup
	for _, item := range []struct {
		nome  string
		coord *consorcio.Coordenador
	}{{"ana", coordA}, {"bia", coordC}} {
		wg.Add(1)
		go func(nome string, coord *consorcio.Coordenador) {
			defer wg.Done()
			<-inicio
			resultado, err := coord.Reservar(context.Background(), nome, dominio.Itinerario{Passos: []dominio.Trecho{trechoB}})
			resultados <- protocoloResultadoTeste{resultado: resultado.Estado, err: err}
		}(item.nome, item.coord)
	}
	close(inicio)
	wg.Wait()
	close(resultados)

	confirmadas, abortadas := 0, 0
	for resultado := range resultados {
		if resultado.err != nil {
			t.Errorf("erro inesperado de coordenação: %v", resultado.err)
			continue
		}
		switch resultado.resultado {
		case "CONFIRMADA":
			confirmadas++
		case "ABORTADA":
			abortadas++
		default:
			t.Errorf("estado inesperado: %s", resultado.resultado)
		}
	}
	if confirmadas != 1 || abortadas != 1 {
		t.Fatalf("esperava uma confirmação e um aborto; confirmadas=%d abortadas=%d", confirmadas, abortadas)
	}
	reservasA, _ := repoB.ConsultarReservas("ana")
	reservasB, _ := repoB.ConsultarReservas("bia")
	if len(reservasA)+len(reservasB) != 1 {
		t.Fatalf("assento foi vendido mais de uma vez: ana=%d bia=%d", len(reservasA), len(reservasB))
	}
}

type protocoloResultadoTeste struct {
	resultado string
	err       error
}

func TestCoordenadorAgrupaPartesNaoContiguasDaMesmaEmpresa(t *testing.T) {
	repoA, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	repoB, _ := estado.NovoRepositorioDeEmpresa("empresa-b")
	trechoA1 := criarTrechoParaCoordenacao(t, repoA, "Feira", "Alagoinhas", 1)
	trechoB := criarTrechoParaCoordenacao(t, repoB, "Alagoinhas", "Salvador", 1)
	trechoA2 := criarTrechoParaCoordenacao(t, repoA, "Salvador", "Cachoeira", 1)

	servidorB := httptest.NewServer(api.NovoServidorComPeers("empresa-b", repoB, []consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}}).Handler())
	defer servidorB.Close()
	coordenador := consorcio.NovoCoordenador(repoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})

	itinerario := dominio.Itinerario{Passos: []dominio.Trecho{trechoA1, trechoB, trechoA2}}
	resultado, err := coordenador.Reservar(context.Background(), "ana", itinerario)
	if err != nil {
		t.Fatal(err)
	}
	if resultado.Estado != "CONFIRMADA" {
		t.Fatalf("esperava CONFIRMADA; obtido %+v", resultado)
	}
	reservasA, _ := repoA.ConsultarReservas("ana")
	reservasB, _ := repoB.ConsultarReservas("ana")
	if len(reservasA) != 1 || len(reservasA[0].Itens) != 2 || len(reservasB) != 1 {
		t.Fatalf("a empresa A deveria criar uma reserva com dois trechos e B uma reserva: A=%+v B=%+v", reservasA, reservasB)
	}
}

func TestReservaComChaveDeIdempotenciaNaoDuplicaTransacao(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "empresa-a.json")
	repo, err := estado.NovoRepositorioDeEmpresaComArquivo("empresa-a", caminho)
	if err != nil {
		t.Fatal(err)
	}
	trecho := criarTrechoParaCoordenacao(t, repo, "A", "B", 2)
	itinerario := dominio.Itinerario{Passos: []dominio.Trecho{trecho}}
	coordenador := consorcio.NovoCoordenador(repo, nil)

	primeira, err := coordenador.ReservarComChave(context.Background(), "ana", itinerario, "req-estavel-001")
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := coordenador.ReservarComChave(context.Background(), "ana", itinerario, "req-estavel-001")
	if err != nil {
		t.Fatal(err)
	}
	if primeira.TransacaoID != segunda.TransacaoID {
		t.Fatalf("a repeticao criou outra transacao: primeira=%s segunda=%s", primeira.TransacaoID, segunda.TransacaoID)
	}
	reservas, err := repo.ConsultarReservas("ana")
	if err != nil {
		t.Fatal(err)
	}
	if len(reservas) != 1 {
		t.Fatalf("a repeticao criou reservas duplicadas: %d", len(reservas))
	}

	// A chave continua reconhecida depois de reiniciar o repositório.
	reaberto, err := estado.NovoRepositorioDeEmpresaComArquivo("empresa-a", caminho)
	if err != nil {
		t.Fatal(err)
	}
	terceira, err := consorcio.NovoCoordenador(reaberto, nil).ReservarComChave(
		context.Background(), "ana", itinerario, "req-estavel-001",
	)
	if err != nil {
		t.Fatal(err)
	}
	if terceira.TransacaoID != primeira.TransacaoID || terceira.Estado != "CONFIRMADA" {
		t.Fatalf("a repeticao apos reinicio nao devolveu a transacao original: %+v", terceira)
	}
	reservas, _ = reaberto.ConsultarReservas("ana")
	if len(reservas) != 1 {
		t.Fatalf("repeticao apos reinicio duplicou reserva: %d", len(reservas))
	}
}

func TestChaveIdempotenciaNaoPodeSerReutilizadaComOutroItinerario(t *testing.T) {
	repo, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	trecho1 := criarTrechoParaCoordenacao(t, repo, "A", "B", 1)
	trecho2 := criarTrechoParaCoordenacao(t, repo, "B", "C", 1)
	coordenador := consorcio.NovoCoordenador(repo, nil)
	if _, err := coordenador.ReservarComChave(context.Background(), "ana", dominio.Itinerario{Passos: []dominio.Trecho{trecho1}}, "mesma-chave"); err != nil {
		t.Fatal(err)
	}
	if _, err := coordenador.ReservarComChave(context.Background(), "ana", dominio.Itinerario{Passos: []dominio.Trecho{trecho2}}, "mesma-chave"); err == nil {
		t.Fatal("a mesma chave nao deveria identificar itinerarios diferentes")
	}
}

func TestRecuperacaoAbortaPreparacaoInterrompidaAntesDaDecisao(t *testing.T) {
	caminhoA := filepath.Join(t.TempDir(), "empresa-a.json")
	repoA, err := estado.NovoRepositorioDeEmpresaComArquivo("empresa-a", caminhoA)
	if err != nil {
		t.Fatal(err)
	}
	repoB, _ := estado.NovoRepositorioDeEmpresa("empresa-b")
	trechoA := criarTrechoParaCoordenacao(t, repoA, "A", "B", 1)
	trechoB := criarTrechoParaCoordenacao(t, repoB, "B", "C", 1)
	itinerario := dominio.Itinerario{Passos: []dominio.Trecho{trechoA, trechoB}}
	participantes := []estado.ParticipanteCoordenacao{
		{EmpresaID: "empresa-a", Itinerario: dominio.Itinerario{Passos: []dominio.Trecho{trechoA}}},
		{EmpresaID: "empresa-b", Itinerario: dominio.Itinerario{Passos: []dominio.Trecho{trechoB}}},
	}
	tx := estado.TransacaoCoordenada{
		ID: "empresa-a-recovery-1", Passageiro: "ana", Itinerario: itinerario,
		Participantes: participantes, Estado: estado.CoordenacaoPreparando,
	}
	if err := repoA.CriarTransacaoCoordenada(tx); err != nil {
		t.Fatal(err)
	}
	// Simula: A preparou localmente; B tambem preparou, mas o ACK de B
	// nao chegou a ser persistido no log da coordenadora.
	if err := repoA.MarcarPreparacaoTentada(tx.ID, "empresa-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := repoA.PrepararTransacao(tx.ID, "ana", participantes[0].Itinerario); err != nil {
		t.Fatal(err)
	}
	if err := repoA.RegistrarParticipantePreparado(tx.ID, "empresa-a"); err != nil {
		t.Fatal(err)
	}
	if err := repoA.MarcarPreparacaoTentada(tx.ID, "empresa-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := repoB.PrepararTransacao(tx.ID, "ana", participantes[1].Itinerario); err != nil {
		t.Fatal(err)
	}

	// Reinicia a coordenadora a partir do JSON, simulando a queda do processo.
	reabertoA, err := estado.NovoRepositorioDeEmpresaComArquivo("empresa-a", caminhoA)
	if err != nil {
		t.Fatal(err)
	}
	servidorB := httptest.NewServer(api.NovoServidorComPeers(
		"empresa-b", repoB,
		[]consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}},
	).Handler())
	defer servidorB.Close()
	coordenador := consorcio.NovoCoordenador(reabertoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})

	if err := coordenador.RecuperarPendentes(context.Background()); err != nil {
		t.Fatalf("recuperacao deveria concluir o aborto: %v", err)
	}
	log, err := reabertoA.ConsultarTransacaoCoordenada(tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if log.Decisao != estado.DecisaoAbortar || log.Estado != estado.CoordenacaoConcluida {
		t.Fatalf("coordenacao nao foi abortada e concluida: %+v", log)
	}
	localA, _ := reabertoA.ConsultarTransacao(tx.ID)
	localB, _ := repoB.ConsultarTransacao(tx.ID)
	if localA.Estado != estado.EstadoAbortada || localB.Estado != estado.EstadoAbortada {
		t.Fatalf("participantes nao foram liberados: A=%s B=%s", localA.Estado, localB.Estado)
	}
}

func TestRecuperacaoRetomaDecisaoConfirmarPersistida(t *testing.T) {
	repoA, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	repoB, _ := estado.NovoRepositorioDeEmpresa("empresa-b")
	trechoA := criarTrechoParaCoordenacao(t, repoA, "A", "B", 1)
	trechoB := criarTrechoParaCoordenacao(t, repoB, "B", "C", 1)
	itA := dominio.Itinerario{Passos: []dominio.Trecho{trechoA}}
	itB := dominio.Itinerario{Passos: []dominio.Trecho{trechoB}}
	it := dominio.Itinerario{Passos: []dominio.Trecho{trechoA, trechoB}}
	tx := estado.TransacaoCoordenada{
		ID: "empresa-a-recovery-commit", Passageiro: "ana", Itinerario: it,
		Participantes: []estado.ParticipanteCoordenacao{
			{EmpresaID: "empresa-a", Itinerario: itA},
			{EmpresaID: "empresa-b", Itinerario: itB},
		},
		Estado: estado.CoordenacaoPreparando,
	}
	if err := repoA.CriarTransacaoCoordenada(tx); err != nil {
		t.Fatal(err)
	}
	for _, empresa := range []string{"empresa-a", "empresa-b"} {
		if err := repoA.MarcarPreparacaoTentada(tx.ID, empresa); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repoA.PrepararTransacao(tx.ID, "ana", itA); err != nil {
		t.Fatal(err)
	}
	if err := repoA.MarcarTransacaoCoordenada(tx.ID); err != nil {
		t.Fatal(err)
	}
	if err := repoA.RegistrarParticipantePreparado(tx.ID, "empresa-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := repoB.PrepararTransacao(tx.ID, "ana", itB); err != nil {
		t.Fatal(err)
	}
	if err := repoB.MarcarTransacaoCoordenada(tx.ID); err != nil {
		t.Fatal(err)
	}
	if err := repoA.RegistrarParticipantePreparado(tx.ID, "empresa-b"); err != nil {
		t.Fatal(err)
	}
	if err := repoA.DefinirDecisaoCoordenada(tx.ID, estado.DecisaoConfirmar); err != nil {
		t.Fatal(err)
	}

	servidorB := httptest.NewServer(api.NovoServidorComPeers(
		"empresa-b", repoB,
		[]consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}},
	).Handler())
	defer servidorB.Close()
	coordenador := consorcio.NovoCoordenador(repoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})
	if err := coordenador.RecuperarPendentes(context.Background()); err != nil {
		t.Fatal(err)
	}
	final, err := repoA.ConsultarTransacaoCoordenada(tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Estado != estado.CoordenacaoConcluida || final.Decisao != estado.DecisaoConfirmar {
		t.Fatalf("decisao confirmada nao foi retomada: %+v", final)
	}
	reservasB, _ := repoB.ConsultarReservas("ana")
	if len(reservasB) != 1 {
		t.Fatalf("esperava exatamente uma reserva remota apos recovery; obtidas %d", len(reservasB))
	}
}

func TestConfirmacaoRemotaExecutadaComRespostaPerdidaEhIdempotente(t *testing.T) {
	repoA, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	repoB, _ := estado.NovoRepositorioDeEmpresa("empresa-b")
	trechoB := criarTrechoParaCoordenacao(t, repoB, "B", "C", 1)
	base := api.NovoServidorComPeers("empresa-b", repoB, []consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}}).Handler()
	perderPrimeiroACK := atomic.Bool{}
	perderPrimeiroACK.Store(true)
	servidorB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transacoes/empresa-a-1/confirmar" && perderPrimeiroACK.CompareAndSwap(true, false) {
			// Executa e persiste a confirmacao no participante, mas simula a
			// perda da resposta na rede devolvendo um erro ao coordenador.
			recorder := httptest.NewRecorder()
			base.ServeHTTP(recorder, r)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"erro":"ACK perdido depois da execucao"}`))
			return
		}
		base.ServeHTTP(w, r)
	}))
	defer servidorB.Close()

	coord := consorcio.NovoCoordenador(repoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})
	it := dominio.Itinerario{Passos: []dominio.Trecho{trechoB}}
	pendente, err := coord.ReservarComChave(context.Background(), "ana", it, "req-ack-perdido")
	if err != nil {
		t.Fatal(err)
	}
	if pendente.Estado != "PENDENTE_CONFIRMAR" {
		t.Fatalf("esperava estado pendente após perder ACK; obtido %+v", pendente)
	}

	reservasAntes, _ := repoB.ConsultarReservas("ana")
	if len(reservasAntes) != 1 {
		t.Fatalf("a empresa B deveria ter executado a confirmação: %d", len(reservasAntes))
	}

	final, err := coord.RetomarDecisao(context.Background(), pendente.TransacaoID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Estado != "CONFIRMADA" {
		t.Fatalf("retomada deveria concluir: %+v", final)
	}
	reservasDepois, _ := repoB.ConsultarReservas("ana")
	if len(reservasDepois) != 1 || reservasDepois[0].Id != reservasAntes[0].Id {
		t.Fatalf("repetir confirmacao duplicou reserva: antes=%+v depois=%+v", reservasAntes, reservasDepois)
	}
}

func TestDuasChamadasConcorrentesComMesmaChaveUsamUmaTransacao(t *testing.T) {
	repo, _ := estado.NovoRepositorioDeEmpresa("empresa-a")
	trecho := criarTrechoParaCoordenacao(t, repo, "A", "B", 2)
	coord := consorcio.NovoCoordenador(repo, nil)
	it := dominio.Itinerario{Passos: []dominio.Trecho{trecho}}

	const chamadas = 8
	resultados := make(chan string, chamadas)
	erros := make(chan error, chamadas)
	inicio := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < chamadas; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			resultado, err := coord.ReservarComChave(context.Background(), "ana", it, "chave-concorrente")
			if err != nil {
				erros <- err
				return
			}
			resultados <- resultado.TransacaoID
		}()
	}
	close(inicio)
	wg.Wait()
	close(resultados)
	close(erros)
	for err := range erros {
		t.Errorf("chamada idempotente concorrente falhou: %v", err)
	}
	var id string
	for atual := range resultados {
		if id == "" {
			id = atual
		} else if atual != id {
			t.Errorf("chamadas usaram transacoes diferentes: %s e %s", id, atual)
		}
	}
	reservas, _ := repo.ConsultarReservas("ana")
	if len(reservas) != 1 {
		t.Fatalf("esperava uma reserva para a mesma chave; obtidas %d", len(reservas))
	}
}
