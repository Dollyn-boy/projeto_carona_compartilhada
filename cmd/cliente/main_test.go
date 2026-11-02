package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChaveReservaPendenteSobreviveReinicio(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "cliente_estado.json")
	t.Setenv("VAIJUNTO_CLIENTE_ESTADO", caminho)

	estadoCliente = estadoPersistidoCliente{ChavesPendentes: make(map[string]string), ServidoresPendentes: make(map[string]string)}
	assinatura := "assinatura-de-teste"

	primeira, err := obterChaveReservaPendente(assinatura, "servidor-a:8080")
	if err != nil {
		t.Fatalf("obter primeira chave: %v", err)
	}
	if primeira == "" {
		t.Fatal("a chave de idempotencia nao pode ser vazia")
	}

	// Simula o encerramento do processo e a inicializacao de outro cliente.
	estadoCliente = estadoPersistidoCliente{ChavesPendentes: make(map[string]string), ServidoresPendentes: make(map[string]string)}
	if err := carregarEstadoCliente(); err != nil {
		t.Fatalf("carregar estado persistido: %v", err)
	}

	segunda, err := obterChaveReservaPendente(assinatura, "servidor-a:8080")
	if err != nil {
		t.Fatalf("obter chave apos reinicio: %v", err)
	}
	if segunda != primeira {
		t.Fatalf("chave mudou depois do reinicio: primeira=%q segunda=%q", primeira, segunda)
	}
}

func TestChaveReservaPendenteSoEhRemovidaAposConclusao(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "cliente_estado.json")
	t.Setenv("VAIJUNTO_CLIENTE_ESTADO", caminho)
	estadoCliente = estadoPersistidoCliente{ChavesPendentes: make(map[string]string), ServidoresPendentes: make(map[string]string)}

	assinatura := "reserva-concluida"
	chave, err := obterChaveReservaPendente(assinatura, "servidor-a:8080")
	if err != nil {
		t.Fatal(err)
	}
	removerChaveReservaPendente(assinatura)

	outra, err := obterChaveReservaPendente(assinatura, "servidor-a:8080")
	if err != nil {
		t.Fatal(err)
	}
	if outra == chave {
		t.Fatal("uma reserva concluida nao deve reter a chave de tentativa anterior")
	}

	if _, err := os.Stat(caminho); err != nil {
		t.Fatalf("arquivo de estado do cliente deveria existir: %v", err)
	}
}

func TestAssinaturaReservaDistinguePassageiroEItinerario(t *testing.T) {
	dados := []byte(`{"trechos":[{"empresa_id":"empresa-a","id_carona":"1","indice":0}]}`)
	assinaturaAna := assinaturaReservaCliente("ana", dados)
	assinaturaBia := assinaturaReservaCliente("bia", dados)
	if assinaturaAna == assinaturaBia {
		t.Fatal("passageiros diferentes nao devem compartilhar a mesma assinatura")
	}
	assinaturaOutroItinerario := assinaturaReservaCliente("ana", []byte(`{"trechos":[{"empresa_id":"empresa-b","id_carona":"2","indice":0}]}`))
	if assinaturaAna == assinaturaOutroItinerario {
		t.Fatal("itinerarios diferentes nao devem compartilhar a mesma assinatura")
	}
}

func TestChavePendenteNaoPodeSerReenviadaAOutroCoordenador(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "cliente_estado.json")
	t.Setenv("VAIJUNTO_CLIENTE_ESTADO", caminho)
	estadoCliente = estadoPersistidoCliente{
		ChavesPendentes:     make(map[string]string),
		ServidoresPendentes: make(map[string]string),
	}
	assinatura := "mesma-viagem"
	if _, err := obterChaveReservaPendente(assinatura, "servidor-a:8080"); err != nil {
		t.Fatal(err)
	}
	if _, err := obterChaveReservaPendente(assinatura, "servidor-b:8080"); err == nil {
		t.Fatal("uma tentativa pendente nao deve ser enviada a outro coordenador")
	}
}
