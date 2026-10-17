package estado

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"vaijunto/internal/dominio"
)

func TestPubclicarCaronaInicializaTodosTrechos(t *testing.T) {
	repo := NovoRepositorio()
	rota := []dominio.Cidade{"A", "B", "C", "D"}
	data := dataFutura()

	carona, err := repo.PublicarCarona("motorista1", rota, 3, 10, data)

	if err != nil {
		t.Fatalf("erro inesperado ao publicar carona: %v", err)
	}

	id := carona.Id
	assentos_por_trecho := repo.assentos[strconv.Itoa(id)]

	qtd_trechos := len(assentos_por_trecho)
	if qtd_trechos != 3 {
		t.Fatalf("erro na quantidade de trechos criados: %d , quando deveria ser 3", qtd_trechos)
	}

	for i, assentos := range assentos_por_trecho {
		if assentos != 3 {
			t.Fatalf(
				"trecho %d deveria ter 3 assentos, mas tem %d",
				i, assentos,
			)
		}
	}
}

func TestRepositorioPersisteERecarregaJSONRepo(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	repo, err := NovoRepositorioComArquivo(caminho)

	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CadastrarUsuario("ana", "senha-de-teste", "p"); err != nil {
		t.Fatal(err)
	}
	carona, err := repo.PublicarCarona("motorista", []dominio.Cidade{"A", "B", "C"}, 1, 20, dataFutura())
	if err != nil {
		t.Fatal(err)
	}
	itinerario := dominio.Itinerario{Passos: dominio.TrechosDaRota(strconv.Itoa(carona.Id), carona.Rota)}
	if _, err := repo.ConfirmarReserva("ana", itinerario); err != nil {
		t.Fatal(err)
	}

	segundoRepo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}

	autenticado, _, err := segundoRepo.AutenticarUsuario("ana", "senha-de-teste")
	if err != nil {
		t.Fatal(err)
	}
	if !autenticado {
		t.Fatalf("Autenticação do usuário falhou")
	}

	reservas, err := segundoRepo.ConsultarReservas("ana")
	if err != nil {
		t.Fatal(err)
	}
	if len(reservas) == 0 {
		t.Fatalf("Reserva não pode ser confirmada")
	}

	assentos_por_trecho := segundoRepo.assentos[strconv.Itoa(carona.Id)]
	for i, assentos := range assentos_por_trecho {
		if assentos != 0 {
			t.Fatalf(
				"trecho %d deveria ter 0 assentos disponíveis, mas tem %d",
				i, assentos,
			)
		}
	}

}

func TestPublicarCarona(t *testing.T) {
	repo := NovoRepositorio()
	rota := []dominio.Cidade{"A", "B", "C"}
	data := dataFutura()

	carona, err := repo.PublicarCarona("motorista1", rota, 3, 10, data)

	if err != nil {
		t.Fatalf("erro inesperado ao publicar carona: %v", err)
	}

	if carona.Id != 1 {
		t.Errorf("esperado carona.Id = 1, mas veio %d", carona.Id)
	}

	if carona.Motorista != "motorista1" {
		t.Errorf("esperado motorista = 'motorista1', mas veio '%s'", carona.Motorista)
	}

	if len(carona.Rota) != 3 || carona.Rota[0] != "A" || carona.Rota[1] != "B" || carona.Rota[2] != "C" {
		t.Errorf("esperado rota = [A B C], mas veio %v", carona.Rota)
	}

	if carona.Capacidade != 3 {
		t.Errorf("esperado capacidade = 3, mas veio %d", carona.Capacidade)
	}

	if carona.Preco != 10 {
		t.Errorf("esperado preco = 10, mas veio %d", carona.Preco)
	}
}

func TestBuscarItinerarios(t *testing.T) {
	r := NovoRepositorio()

	data := dataFutura()

	if _, err := r.PublicarCarona(
		"Joao",
		[]dominio.Cidade{
			"Feira de Santana",
			"Santo Amaro",
		},
		3,
		20,
		data,
	); err != nil {
		t.Fatalf("erro inesperado ao publicar a carona de Joao: %v", err)
	}

	if _, err := r.PublicarCarona(
		"Maria",
		[]dominio.Cidade{
			"Santo Amaro",
			"Salvador",
		},
		3,
		30,
		data,
	); err != nil {
		t.Fatalf("erro inesperado ao publicar a carona de Maria: %v", err)
	}

	itinerarios, err := r.BuscarItinerarios(
		"Feira de Santana",
		"Salvador",
	)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(itinerarios) != 1 {
		t.Fatalf(
			"esperava 1 itinerario, recebeu %d",
			len(itinerarios),
		)
	}

	if len(itinerarios[0].Passos) != 2 {
		t.Fatalf(
			"esperava 2 passos, recebeu %d",
			len(itinerarios[0].Passos),
		)
	}
}

// dataFutura evita que o teste comece a falhar quando o calendario avanca.
func dataFutura() time.Time {
	return time.Now().Add(48 * time.Hour)
}

func TestDisputaPeloUltimoAssento(t *testing.T) {
	// Criar carona com 1 assento disponível
	repo := NovoRepositorio()
	carona, err := repo.PublicarCarona("motorista", []dominio.Cidade{"A", "B"}, 1, 10, dataFutura())
	if err != nil {
		t.Fatal(err)
	}
	itinerario := dominio.Itinerario{Passos: dominio.TrechosDaRota("1", carona.Rota)}

	// Iniciar 20 Go Routines
	const tentativas = 20

	var wg sync.WaitGroup // permite esperar até que todas as 20 goroutines terminem.
	var mu sync.Mutex     // protege a variável confirmadas, que várias goroutines podem tentar incrementar simultaneamente.

	confirmadas := 0

	// Todas as go routines reservam o mesmo trecho com 1 assento disponível
	for i := 0; i < tentativas; i++ {
		wg.Add(1) // registra uma goroutine que precisa terminar.

		go func(i int) {
			defer wg.Done() // avisa que aquela goroutine terminou, inclusive se a função retornar antes do fim.
			_, err := repo.ConfirmarReserva("passageiro-"+strconv.Itoa(i), itinerario)

			if err == nil {
				mu.Lock()
				confirmadas++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Deve haver apenas 1 reserva confirmada ao fim do processo
	if confirmadas != 1 {
		t.Fatalf(
			"esperava 1 reserva confirmada, mas houve %d",
			confirmadas,
		)
	}
}

func TestReservaConcorrenteNaoVendeDuasVezes(t *testing.T) {
	repo := NovoRepositorio()
	carona, err := repo.PublicarCarona("motorista", []dominio.Cidade{"A", "B"}, 1, 10, dataFutura())
	if err != nil {
		t.Fatal(err)
	}
	itinerario := dominio.Itinerario{Passos: dominio.TrechosDaRota("1", carona.Rota)}

	const passageiros = 20
	var sucessos atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < passageiros; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := repo.ConfirmarReserva("passageiro-"+strconv.Itoa(i), itinerario); err == nil {
				sucessos.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if obtido := sucessos.Load(); obtido != 1 {
		t.Fatalf("esperava exatamente uma reserva confirmada, obtive %d", obtido)
	}
	totalReservas := 0
	for i := 0; i < passageiros; i++ {
		reservas, err := repo.ConsultarReservas("passageiro-" + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		totalReservas += len(reservas)
	}
	if totalReservas != 1 {
		t.Fatalf("esperava uma unica reserva no repositorio, encontrei %d", totalReservas)
	}
}

func TestReservaDeItinerarioEAtomica(t *testing.T) {
	repo := NovoRepositorio()
	carona, err := repo.PublicarCarona("motorista", []dominio.Cidade{"A", "B", "C"}, 1, 10, dataFutura())
	if err != nil {
		t.Fatal(err)
	}
	trechos := dominio.TrechosDaRota("1", carona.Rota)
	if _, err := repo.ConfirmarReserva("primeiro", dominio.Itinerario{Passos: []dominio.Trecho{trechos[0]}}); err != nil {
		t.Fatal(err)
	}

	// A-B ja esta cheio; a tentativa A-B-C deve falhar sem ocupar B-C.
	if _, err := repo.ConfirmarReserva("segundo", dominio.Itinerario{Passos: trechos}); err == nil {
		t.Fatal("a reserva deveria falhar porque A-B nao tem assento")
	}
	caminhoBC, err := repo.BuscarItinerarios("B", "C")
	if err != nil {
		t.Fatal(err)
	}
	if len(caminhoBC) != 1 || len(caminhoBC[0].Passos) != 1 {
		t.Fatalf("a falha parcial ocupou indevidamente o trecho B-C: %+v", caminhoBC)
	}
}

func TestRepositorioPersisteERecarregaJSON(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	repo, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CadastrarUsuario("ana", "senha-de-teste", "p"); err != nil {
		t.Fatal(err)
	}
	carona, err := repo.PublicarCarona("motorista", []dominio.Cidade{"A", "B", "C"}, 1, 20, dataFutura())
	if err != nil {
		t.Fatal(err)
	}
	itinerario := dominio.Itinerario{Passos: dominio.TrechosDaRota("1", carona.Rota)}
	if _, err := repo.ConfirmarReserva("ana", itinerario); err != nil {
		t.Fatal(err)
	}

	conteudo, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatal(err)
	}
	var jsonValido map[string]any
	if err := json.Unmarshal(conteudo, &jsonValido); err != nil {
		t.Fatalf("o arquivo persistido nao e JSON valido: %v", err)
	}
	if strings.Contains(string(conteudo), "senha-de-teste") {
		t.Fatal("o arquivo nao deve guardar senha em texto puro")
	}

	reaberto, err := NovoRepositorioComArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	ok, role, err := reaberto.AutenticarUsuario("ana", "senha-de-teste")
	if err != nil || !ok || role != "p" {
		t.Fatalf("usuario nao foi restaurado corretamente: ok=%v role=%q err=%v", ok, role, err)
	}
	reservas, err := reaberto.ConsultarReservas("ana")
	if err != nil || len(reservas) != 1 {
		t.Fatalf("reserva nao foi restaurada: reservas=%v err=%v", reservas, err)
	}
	if _, err := reaberto.ConfirmarReserva("outra-pessoa", itinerario); err == nil {
		t.Fatal("a disponibilidade deveria continuar ocupada depois do reinicio")
	}
	if err := reaberto.CancelarReserva(reservas[0].Id, "ana"); err != nil {
		t.Fatal(err)
	}
	if _, err := reaberto.ConfirmarReserva("outra-pessoa", itinerario); err != nil {
		t.Fatalf("cancelamento nao foi persistido corretamente: %v", err)
	}
}

func TestFalhaAoSalvarDesfazAlteracaoEmMemoria(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "dados.json")
	if err := os.Mkdir(caminho, 0755); err != nil {
		t.Fatal(err)
	}
	repo := NovoRepositorio()
	// Um diretorio no lugar do arquivo faz a troca atomica falhar de forma controlada.
	repo.caminhoJSON = caminho
	if _, err := repo.PublicarCarona("motorista", []dominio.Cidade{"A", "B"}, 1, 10, dataFutura()); err == nil {
		t.Fatal("a publicacao deveria retornar erro se o snapshot nao puder ser gravado")
	}
	caronas, err := repo.ConsultarCaronas("motorista")
	if err != nil {
		t.Fatal(err)
	}
	if len(caronas) != 0 {
		t.Fatalf("o estado em memoria nao foi revertido apos falha de persistencia: %+v", caronas)
	}
}
