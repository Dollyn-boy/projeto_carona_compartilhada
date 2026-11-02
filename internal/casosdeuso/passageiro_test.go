package casosdeuso

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"vaijunto/internal/api"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

func publicarTrechoCasoUso(t *testing.T, repo *estado.Repositorio, origem, destino dominio.Cidade) dominio.Trecho {
	t.Helper()
	carona, err := repo.PublicarCarona("motorista", []dominio.Cidade{origem, destino}, 1, 25, time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatalf("publicar carona: %v", err)
	}
	return dominio.TrechosDaRota(repo.EmpresaID(), strconv.Itoa(carona.Id), carona.Rota)[0]
}

func TestConfirmarReservaTCPIdempotenteEConsultaPartesDistribuidas(t *testing.T) {
	repoA, err := estado.NovoRepositorioDeEmpresa("empresa-a")
	if err != nil {
		t.Fatal(err)
	}
	repoB, err := estado.NovoRepositorioDeEmpresa("empresa-b")
	if err != nil {
		t.Fatal(err)
	}
	trechoA := publicarTrechoCasoUso(t, repoA, "Feira", "Alagoinhas")
	trechoB := publicarTrechoCasoUso(t, repoB, "Alagoinhas", "Salvador")

	servidorB := httptest.NewServer(api.NovoServidorComPeers(
		"empresa-b", repoB,
		[]consorcio.Peer{{EmpresaID: "empresa-a", URL: "http://empresa-a"}},
	).Handler())
	defer servidorB.Close()

	busca := consorcio.NovoBuscaDistribuida(repoA, []consorcio.Peer{{EmpresaID: "empresa-b", URL: servidorB.URL}})
	dados, err := json.Marshal(protocolo.ConfirmarReservaDados{Trechos: []protocolo.TrechoDados{
		{EmpresaID: trechoA.EmpresaID, IDCarona: trechoA.IDCarona, Indice: trechoA.Indice, Origem: string(trechoA.Origem), Destino: string(trechoA.Destino)},
		{EmpresaID: trechoB.EmpresaID, IDCarona: trechoB.IDCarona, Indice: trechoB.Indice, Origem: string(trechoB.Origem), Destino: string(trechoB.Destino)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	sessao := &Sessao{Autenticado: true, Usuario: "ana", Role: "p", UltimaAtividade: time.Now()}
	req := protocolo.Requisicao{Tipo: "confirmar_reserva", IDRequisicao: "solicitacao-estavel-001", Dados: dados}

	primeira := DespacharComBusca(repoA, sessao, req, busca)
	if primeira.Status != "ok" {
		t.Fatalf("primeira reserva falhou: %s", primeira.Motivo)
	}
	var resultado1 protocolo.CoordenarReservaResposta
	if err := json.Unmarshal(primeira.Dados, &resultado1); err != nil {
		t.Fatal(err)
	}
	if resultado1.Estado != "CONFIRMADA" || len(resultado1.Reservas) != 2 {
		t.Fatalf("reserva distribuida inesperada: %+v", resultado1)
	}

	// Simula repetição TCP da mesma solicitação porque o cliente perdeu a
	// resposta original: o mesmo ID deve recuperar a mesma transação.
	segunda := DespacharComBusca(repoA, sessao, req, busca)
	if segunda.Status != "ok" {
		t.Fatalf("repeticao da reserva falhou: %s", segunda.Motivo)
	}
	var resultado2 protocolo.CoordenarReservaResposta
	if err := json.Unmarshal(segunda.Dados, &resultado2); err != nil {
		t.Fatal(err)
	}
	if resultado2.TransacaoID != resultado1.TransacaoID {
		t.Fatalf("repeticao criou outra transacao: primeira=%s segunda=%s", resultado1.TransacaoID, resultado2.TransacaoID)
	}
	reservasA, _ := repoA.ConsultarReservas("ana")
	reservasB, _ := repoB.ConsultarReservas("ana")
	if len(reservasA) != 1 || len(reservasB) != 1 {
		t.Fatalf("repeticao duplicou reserva: A=%d B=%d", len(reservasA), len(reservasB))
	}

	consulta := DespacharComBusca(repoA, sessao, protocolo.Requisicao{
		Tipo: "consultar_reservas", IDRequisicao: "consulta-001", Dados: json.RawMessage(`{}`),
	}, busca)
	if consulta.Status != "ok" {
		t.Fatalf("consulta de reservas falhou: %s", consulta.Motivo)
	}
	var reservas protocolo.ConsultarReservasResposta
	if err := json.Unmarshal(consulta.Dados, &reservas); err != nil {
		t.Fatal(err)
	}
	if len(reservas.Reservas) != 2 {
		t.Fatalf("consulta deveria agregar as duas empresas; obtidas %d", len(reservas.Reservas))
	}
	if reservas.Reservas[0].TransacaoID == "" || reservas.Reservas[1].TransacaoID == "" {
		t.Fatal("consulta deveria informar o ID global da transacao")
	}

	// O cancelamento parcial é recusado: não pode liberar somente o trecho A.
	cancelarDados, _ := json.Marshal(protocolo.CancelarReservaDados{
		IDReserva:   resultado1.Reservas[0].ID,
		EmpresaID:   resultado1.Reservas[0].EmpresaID,
		TransacaoID: resultado1.TransacaoID,
	})
	cancelamento := DespacharComBusca(repoA, sessao, protocolo.Requisicao{
		Tipo: "cancelar_reserva", IDRequisicao: "cancelar-001", Dados: cancelarDados,
	}, busca)
	if cancelamento.Status != "erro" {
		t.Fatal("cancelamento parcial de itinerario distribuido deveria ser bloqueado")
	}
	reservasA, _ = repoA.ConsultarReservas("ana")
	reservasB, _ = repoB.ConsultarReservas("ana")
	if len(reservasA) != 1 || len(reservasB) != 1 {
		t.Fatalf("tentativa de cancelamento parcial alterou dados: A=%d B=%d", len(reservasA), len(reservasB))
	}

}
