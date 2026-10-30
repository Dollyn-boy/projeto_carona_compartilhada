package estado

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"vaijunto/internal/dominio"
)

func criarItinerarioTransacao(t *testing.T, repo *Repositorio, rota []dominio.Cidade, capacidade int) (dominio.Carona, dominio.Itinerario) {
	t.Helper()

	carona, err := repo.PublicarCarona(
		"motorista",
		rota,
		capacidade,
		25,
		dataFutura(),
	)
	if err != nil {
		t.Fatalf("publicar carona: %v", err)
	}

	itinerario := dominio.Itinerario{
		Passos: dominio.TrechosDaRota(repo.empresaID, strconv.Itoa(carona.Id), carona.Rota),
	}
	return carona, itinerario
}

func TestPrepararTransacaoDescontaAssentosSemCriarReserva(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B", "C"}, 1)

	transacao, err := repo.PrepararTransacao("TX-001", "ana", itinerario)
	if err != nil {
		t.Fatalf("preparar transacao: %v", err)
	}
	if transacao.Estado != EstadoPreparada {
		t.Fatalf("estado esperado %s; obtido %s", EstadoPreparada, transacao.Estado)
	}
	if got := repo.assentos["1"]; len(got) != 2 || got[0] != 0 || got[1] != 0 {
		t.Fatalf("assentos deveriam estar temporariamente bloqueados: %v", got)
	}
	if len(repo.reservas) != 0 {
		t.Fatalf("preparação não deveria criar reserva definitiva; reservas=%d", len(repo.reservas))
	}
}

func TestPrepararTransacaoIdempotente(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)

	primeira, err := repo.PrepararTransacao("TX-IDEMPOTENTE", "ana", itinerario)
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := repo.PrepararTransacao("TX-IDEMPOTENTE", "ana", itinerario)
	if err != nil {
		t.Fatalf("repetir preparação deveria ser idempotente: %v", err)
	}
	if primeira.ID != segunda.ID || segunda.Estado != EstadoPreparada {
		t.Fatalf("a repetição não retornou a mesma transação: primeira=%+v segunda=%+v", primeira, segunda)
	}
	if got := repo.assentos["1"][0]; got != 0 {
		t.Fatalf("a repetição descontou assento novamente; disponíveis=%d", got)
	}
	if len(repo.transacoes) != 1 {
		t.Fatalf("esperava uma transação, encontrei %d", len(repo.transacoes))
	}
}

func TestPrepararTransacaoMesmoIDComDadosDiferentes(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerarioA := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 2)
	_, itinerarioB := criarItinerarioTransacao(t, repo, []dominio.Cidade{"B", "C"}, 2)

	if _, err := repo.PrepararTransacao("TX-CONFLITO", "ana", itinerarioA); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PrepararTransacao("TX-CONFLITO", "bruno", itinerarioB); !errors.Is(err, ErrTransacaoConflitante) {
		t.Fatalf("esperava conflito de identificador; obtive %v", err)
	}
	if repo.assentos["1"][0] != 1 || repo.assentos["2"][0] != 2 {
		t.Fatalf("a solicitação conflitante modificou assentos: %v / %v", repo.assentos["1"], repo.assentos["2"])
	}
}

func TestPrepararTransacaoFalhaSemAlteracaoParcial(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B", "C"}, 1)

	// Ocupa previamente o segundo trecho. A preparação do itinerário inteiro
	// deve falhar sem bloquear o primeiro trecho parcialmente.
	segundoTrecho := dominio.Itinerario{Passos: []dominio.Trecho{itinerario.Passos[1]}}
	if _, err := repo.ConfirmarReserva("outro-passageiro", segundoTrecho); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.PrepararTransacao("TX-SEM-ASSENTO", "ana", itinerario); err == nil {
		t.Fatal("esperava falha porque o segundo trecho está sem assento")
	}
	if repo.assentos["1"][0] != 1 || repo.assentos["1"][1] != 0 {
		t.Fatalf("a preparação falha alterou parcialmente os assentos: %v", repo.assentos["1"])
	}
	if _, existe := repo.transacoes["TX-SEM-ASSENTO"]; existe {
		t.Fatal("uma preparação inválida não deveria ser registrada")
	}
}

func TestConfirmarTransacaoIdempotente(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B", "C"}, 1)
	if _, err := repo.PrepararTransacao("TX-CONFIRMAR", "ana", itinerario); err != nil {
		t.Fatal(err)
	}

	primeira, err := repo.ConfirmarTransacao("TX-CONFIRMAR")
	if err != nil {
		t.Fatalf("confirmar transação: %v", err)
	}
	segunda, err := repo.ConfirmarTransacao("TX-CONFIRMAR")
	if err != nil {
		t.Fatalf("repetir confirmação: %v", err)
	}
	if primeira.Id != segunda.Id {
		t.Fatalf("confirmação repetida criou outra reserva: IDs %d e %d", primeira.Id, segunda.Id)
	}
	if len(repo.reservas) != 1 {
		t.Fatalf("esperava uma reserva definitiva; encontrei %d", len(repo.reservas))
	}
	if repo.assentos["1"][0] != 0 || repo.assentos["1"][1] != 0 {
		t.Fatalf("confirmação alterou os assentos novamente: %v", repo.assentos["1"])
	}

	transacao, err := repo.ConsultarTransacao("TX-CONFIRMAR")
	if err != nil {
		t.Fatal(err)
	}
	if transacao.Estado != EstadoConfirmada || transacao.ReservaID != primeira.Id {
		t.Fatalf("estado final incorreto: %+v", transacao)
	}
}

func TestAbortarTransacaoIdempotente(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B", "C"}, 1)
	if _, err := repo.PrepararTransacao("TX-ABORTAR", "ana", itinerario); err != nil {
		t.Fatal(err)
	}

	if err := repo.AbortarTransacao("TX-ABORTAR"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AbortarTransacao("TX-ABORTAR"); err != nil {
		t.Fatalf("repetir aborto deveria ser idempotente: %v", err)
	}
	if repo.assentos["1"][0] != 1 || repo.assentos["1"][1] != 1 {
		t.Fatalf("assentos não foram devolvidos exatamente uma vez: %v", repo.assentos["1"])
	}
	if len(repo.reservas) != 0 {
		t.Fatalf("aborto não deveria criar reserva definitiva; reservas=%d", len(repo.reservas))
	}
	transacao, err := repo.ConsultarTransacao("TX-ABORTAR")
	if err != nil {
		t.Fatal(err)
	}
	if transacao.Estado != EstadoAbortada {
		t.Fatalf("estado esperado ABORTADA; obtido %s", transacao.Estado)
	}
}

func TestAbortarTransacaoDesconhecidaCriaTombstone(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)

	if err := repo.AbortarTransacao("TX-ATRASADA"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AbortarTransacao("TX-ATRASADA"); err != nil {
		t.Fatalf("repetir aborto do tombstone: %v", err)
	}
	if _, err := repo.PrepararTransacao("TX-ATRASADA", "ana", itinerario); err == nil {
		t.Fatal("uma preparação atrasada não deveria reabrir uma transação abortada")
	}
	if repo.assentos["1"][0] != 1 {
		t.Fatalf("o tombstone não deveria alterar os assentos: %v", repo.assentos["1"])
	}
}

func TestTransicaoDeEstadoInvalida(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)

	if _, err := repo.ConfirmarTransacao("TX-INEXISTENTE"); !errors.Is(err, ErrTransacaoNaoEncontrada) {
		t.Fatalf("esperava ErrTransacaoNaoEncontrada; obtive %v", err)
	}
	if _, err := repo.PrepararTransacao("TX-FINAL", "ana", itinerario); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConfirmarTransacao("TX-FINAL"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AbortarTransacao("TX-FINAL"); !errors.Is(err, ErrEstadoTransacao) {
		t.Fatalf("não deveria abortar transação confirmada; obtive %v", err)
	}
}

func TestPrepararTransacaoRejeitaTrechoDeOutraEmpresa(t *testing.T) {
	repo, err := NovoRepositorioDeEmpresa("empresa-a")
	if err != nil {
		t.Fatal(err)
	}
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)
	itinerario.Passos[0].EmpresaID = "empresa-b"

	if _, err := repo.PrepararTransacao("TX-OUTRA-EMPRESA", "ana", itinerario); err == nil {
		t.Fatal("a empresa A não deveria preparar trecho da empresa B")
	}
	if repo.assentos["1"][0] != 1 {
		t.Fatal("a tentativa inválida alterou os assentos")
	}
}

func TestCaronaNaoPodeSerCanceladaComTransacaoPreparada(t *testing.T) {
	repo := NovoRepositorio()
	carona, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)
	if _, err := repo.PrepararTransacao("TX-CARONA-PENDENTE", "ana", itinerario); err != nil {
		t.Fatal(err)
	}

	if err := repo.CancelarCarona(carona.Id, "motorista"); err == nil {
		t.Fatal("não deveria cancelar carona usada por transação PREPARADA")
	}
	if _, err := repo.ConsultarTransacao("TX-CARONA-PENDENTE"); err != nil {
		t.Fatal(err)
	}
}

func TestTransacaoPreparadaSobreviveReinicio(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	repo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B", "C"}, 1)
	if _, err := repo.PrepararTransacao("TX-REINICIO", "ana", itinerario); err != nil {
		t.Fatal(err)
	}

	reaberto, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	transacao, err := reaberto.ConsultarTransacao("TX-REINICIO")
	if err != nil {
		t.Fatal(err)
	}
	if transacao.Estado != EstadoPreparada {
		t.Fatalf("estado PREPARADA não foi recuperado: %s", transacao.Estado)
	}
	if reaberto.assentos["1"][0] != 0 || reaberto.assentos["1"][1] != 0 {
		t.Fatalf("assentos temporários não foram recuperados: %v", reaberto.assentos["1"])
	}

	reserva, err := reaberto.ConfirmarTransacao("TX-REINICIO")
	if err != nil {
		t.Fatalf("confirmar após reinício: %v", err)
	}
	if reserva.Id != 1 {
		t.Fatalf("ID de reserva inesperado: %d", reserva.Id)
	}
}

func TestTransacaoConfirmadaSobreviveReinicioERepeticao(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	repo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)
	if _, err := repo.PrepararTransacao("TX-CONFIRMADA-RESTART", "ana", itinerario); err != nil {
		t.Fatal(err)
	}
	primeira, err := repo.ConfirmarTransacao("TX-CONFIRMADA-RESTART")
	if err != nil {
		t.Fatal(err)
	}

	reaberto, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := reaberto.ConfirmarTransacao("TX-CONFIRMADA-RESTART")
	if err != nil {
		t.Fatal(err)
	}
	if segunda.Id != primeira.Id {
		t.Fatalf("repetição após reinício criou outra reserva: %d != %d", segunda.Id, primeira.Id)
	}
	if len(reaberto.reservas) != 1 {
		t.Fatalf("esperava uma reserva após reinício; encontrei %d", len(reaberto.reservas))
	}
}

func TestTransacaoAbortadaSobreviveReinicio(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	repo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)
	if _, err := repo.PrepararTransacao("TX-ABORTADA-RESTART", "ana", itinerario); err != nil {
		t.Fatal(err)
	}
	if err := repo.AbortarTransacao("TX-ABORTADA-RESTART"); err != nil {
		t.Fatal(err)
	}

	reaberto, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	if err := reaberto.AbortarTransacao("TX-ABORTADA-RESTART"); err != nil {
		t.Fatalf("aborto repetido após reinício: %v", err)
	}
	if reaberto.assentos["1"][0] != 1 {
		t.Fatalf("aborto repetido após reinício devolveu assento outra vez: %v", reaberto.assentos["1"])
	}
}

func TestFalhaPersistenciaPreparoRestauraEstado(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)

	// Usar um diretório como destino faz a operação de rename falhar.
	caminho := filepath.Join(t.TempDir(), "destino")
	if err := os.Mkdir(caminho, 0755); err != nil {
		t.Fatal(err)
	}
	repo.caminhoJSON = caminho

	if _, err := repo.PrepararTransacao("TX-ROLLBACK", "ana", itinerario); err == nil {
		t.Fatal("esperava erro de persistência")
	}
	if repo.assentos["1"][0] != 1 {
		t.Fatalf("assento não foi restaurado após falha de persistência: %v", repo.assentos["1"])
	}
	if _, existe := repo.transacoes["TX-ROLLBACK"]; existe {
		t.Fatal("transação não deveria permanecer em memória após rollback")
	}
}

func TestPreparacoesConcorrentesNaoReservamMaisQueCapacidade(t *testing.T) {
	repo := NovoRepositorio()
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)

	const tentativas = 16
	var sucessos atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < tentativas; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "TX-CONCORRENTE-" + strconv.Itoa(i)
			if _, err := repo.PrepararTransacao(id, "passageiro-"+strconv.Itoa(i), itinerario); err == nil {
				sucessos.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if obtido := sucessos.Load(); obtido != 1 {
		t.Fatalf("esperava uma preparação bem-sucedida; obtive %d", obtido)
	}
	if repo.assentos["1"][0] != 0 {
		t.Fatalf("contador de assentos inconsistente: %v", repo.assentos["1"])
	}
}

func TestArquivoVersaoUmContinuaSendoCarregado(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados-v1.json")
	carona := dominio.Carona{
		Id:         1,
		Motorista:  "motorista",
		Rota:       []dominio.Cidade{"A", "B"},
		Preco:      10,
		Capacidade: 2,
		Data:       dataFutura(),
	}
	legado := estadoPersistido{
		EmpresaID:        "local",
		Versao:           1,
		ProximoIDCarona:  1,
		ProximoIDReserva: 0,
		Caronas:          []dominio.Carona{carona},
		Assentos:         map[string][]int{"1": {2}},
		Reservas:         []dominio.Reserva{},
		Usuarios:         []dominio.Usuario{},
	}
	conteudo, err := json.Marshal(legado)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caminho, conteudo, 0644); err != nil {
		t.Fatal(err)
	}

	repo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatalf("arquivo legado versão 1 deveria ser carregado: %v", err)
	}
	if repo.assentos["1"][0] != 2 {
		t.Fatalf("assentos do arquivo legado não foram carregados: %v", repo.assentos["1"])
	}
	if repo.transacoes == nil || len(repo.transacoes) != 0 {
		t.Fatalf("mapa de transações deveria iniciar vazio: %#v", repo.transacoes)
	}

	if err := repo.CadastrarUsuario("ana", "senha", "p"); err != nil {
		t.Fatal(err)
	}
	conteudoAtualizado, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatal(err)
	}
	var atualizado estadoPersistido
	if err := json.Unmarshal(conteudoAtualizado, &atualizado); err != nil {
		t.Fatal(err)
	}
	if atualizado.Versao != 2 {
		t.Fatalf("arquivo deveria ser atualizado para versão 2; obtida %d", atualizado.Versao)
	}
}

func TestTransacaoConfirmadaPermaneceNoHistoricoAposCancelarReserva(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	repo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	_, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)
	if _, err := repo.PrepararTransacao("TX-CANCELADA-DEPOIS", "ana", itinerario); err != nil {
		t.Fatal(err)
	}
	reserva, err := repo.ConfirmarTransacao("TX-CANCELADA-DEPOIS")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CancelarReserva(reserva.Id, "ana"); err != nil {
		t.Fatal(err)
	}

	reaberto, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatalf("transação confirmada deve continuar carregável após cancelamento da reserva: %v", err)
	}
	transacao, err := reaberto.ConsultarTransacao("TX-CANCELADA-DEPOIS")
	if err != nil {
		t.Fatal(err)
	}
	if transacao.Estado != EstadoConfirmada {
		t.Fatalf("estado histórico deveria continuar CONFIRMADA; obtido %s", transacao.Estado)
	}
	if _, err := reaberto.ConfirmarTransacao("TX-CANCELADA-DEPOIS"); err == nil {
		t.Fatal("a confirmação repetida não deve recriar uma reserva que foi cancelada posteriormente")
	}
}

func TestTransacaoConfirmadaSobreviveCancelamentoDaCarona(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	repo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	carona, itinerario := criarItinerarioTransacao(t, repo, []dominio.Cidade{"A", "B"}, 1)
	if _, err := repo.PrepararTransacao("TX-CARONA-CANCELADA", "ana", itinerario); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConfirmarTransacao("TX-CARONA-CANCELADA"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CancelarCarona(carona.Id, "motorista"); err != nil {
		t.Fatal(err)
	}

	if _, err := NovoRepositorioComArquivo(caminho); err != nil {
		t.Fatalf("histórico da transação deve continuar válido após cancelar a carona: %v", err)
	}
}

func TestTombstoneDeAbortoSobreviveReinicio(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	repo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AbortarTransacao("TX-TOMBSTONE"); err != nil {
		t.Fatal(err)
	}

	reaberto, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	transacao, err := reaberto.ConsultarTransacao("TX-TOMBSTONE")
	if err != nil {
		t.Fatal(err)
	}
	if transacao.Estado != EstadoAbortada {
		t.Fatalf("tombstone não foi recuperado: estado=%s", transacao.Estado)
	}
}
