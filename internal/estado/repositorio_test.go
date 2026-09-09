package estado

import (
	"testing"
	"vaijunto/internal/dominio"
)

// TODO — testes de integracao desta camada (dominio + concorrencia ja
// combinados através do Repositorio):
//   - publicar carona e confirmar reservas concorrentes disputando os
//     mesmos trechos (varias goroutines chamando ConfirmarReserva ao
//     mesmo tempo);
//   - validar que a soma de assentos confirmados nunca excede a
//     capacidade e que nenhuma reserva fica parcial;
//   - rodar sempre com: go test -race ./internal/estado/...
//
// Isso complementa (nao substitui) os testes mais focados de
// internal/dominio e internal/concorrencia — aqui voce testa as tres
// camadas trabalhando juntas.
func TestPublicarCarona(t *testing.T) {
	repo := NovoRepositorio()
	rota := []dominio.Cidade{"A", "B", "C"}

	carona, err := repo.PublicarCarona("motorista1", rota, 3, 10)

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

	if _, err := r.PublicarCarona(
		"Joao",
		[]dominio.Cidade{
			"Feira de Santana",
			"Santo Amaro",
		},
		3,
		20,
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
