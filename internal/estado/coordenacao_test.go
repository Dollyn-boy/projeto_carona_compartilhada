package estado

import (
	"path/filepath"
	"testing"

	"vaijunto/internal/dominio"
)

func TestIDTransacaoUnicoEPersistido(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "empresa-a.json")
	repo, err := NovoRepositorioDeEmpresaComArquivo("empresa-a", caminho)
	if err != nil {
		t.Fatal(err)
	}

	id1, err := repo.GerarIDTransacao()
	if err != nil {
		t.Fatal(err)
	}
	id2, err := repo.GerarIDTransacao()
	if err != nil {
		t.Fatal(err)
	}
	if id1 != "empresa-a-1" || id2 != "empresa-a-2" {
		t.Fatalf("IDs inesperados: %s, %s", id1, id2)
	}

	reaberto, err := NovoRepositorioDeEmpresaComArquivo("empresa-a", caminho)
	if err != nil {
		t.Fatal(err)
	}
	id3, err := reaberto.GerarIDTransacao()
	if err != nil {
		t.Fatal(err)
	}
	if id3 != "empresa-a-3" {
		t.Fatalf("contador não sobreviveu à reinicialização: %s", id3)
	}
}

func TestLogCoordenacaoSobreviveReinicializacao(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "empresa-a.json")
	repo, err := NovoRepositorioDeEmpresaComArquivo("empresa-a", caminho)
	if err != nil {
		t.Fatal(err)
	}

	itA := dominio.Itinerario{Passos: []dominio.Trecho{{EmpresaID: "empresa-a", IDCarona: "1", Indice: 0, Origem: "A", Destino: "B"}}}
	itB := dominio.Itinerario{Passos: []dominio.Trecho{{EmpresaID: "empresa-b", IDCarona: "8", Indice: 0, Origem: "B", Destino: "C"}}}
	tx := TransacaoCoordenada{
		ID: "empresa-a-1", Passageiro: "ana",
		Itinerario: dominio.Itinerario{Passos: append(append([]dominio.Trecho{}, itA.Passos...), itB.Passos...)},
		Participantes: []ParticipanteCoordenacao{
			{EmpresaID: "empresa-a", Itinerario: itA},
			{EmpresaID: "empresa-b", Itinerario: itB},
		},
		Estado: CoordenacaoPreparando,
	}
	if err := repo.CriarTransacaoCoordenada(tx); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarcarPreparacaoTentada(tx.ID, "empresa-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RegistrarParticipantePreparado(tx.ID, "empresa-a"); err != nil {
		t.Fatal(err)
	}

	reaberto, err := NovoRepositorioDeEmpresaComArquivo("empresa-a", caminho)
	if err != nil {
		t.Fatal(err)
	}
	carregado, err := reaberto.ConsultarTransacaoCoordenada(tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if carregado.Estado != CoordenacaoPreparando || !carregado.Participantes[0].Preparado {
		t.Fatalf("log de preparação não foi recuperado: %+v", carregado)
	}

	// O commit somente pode ser registrado após ambos participantes estarem preparados.
	if err := reaberto.MarcarPreparacaoTentada(tx.ID, "empresa-b"); err != nil {
		t.Fatal(err)
	}
	if err := reaberto.RegistrarParticipantePreparado(tx.ID, "empresa-b"); err != nil {
		t.Fatal(err)
	}
	if err := reaberto.DefinirDecisaoCoordenada(tx.ID, DecisaoConfirmar); err != nil {
		t.Fatal(err)
	}
	if err := reaberto.MarcarConfirmacaoTentada(tx.ID, "empresa-a"); err != nil {
		t.Fatal(err)
	}
	if err := reaberto.RegistrarParticipanteConfirmado(tx.ID, "empresa-a", 1); err != nil {
		t.Fatal(err)
	}

	reabertoNovamente, err := NovoRepositorioDeEmpresaComArquivo("empresa-a", caminho)
	if err != nil {
		t.Fatal(err)
	}
	recuperado, err := reabertoNovamente.ConsultarTransacaoCoordenada(tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recuperado.Decisao != DecisaoConfirmar || recuperado.Estado != CoordenacaoPendente || !recuperado.Participantes[0].Confirmado {
		t.Fatalf("decisão/ACK não persistidos corretamente: %+v", recuperado)
	}
}

func TestDecisaoCoordenadaEhImutavel(t *testing.T) {
	repo, _ := NovoRepositorioDeEmpresa("empresa-a")
	it := dominio.Itinerario{Passos: []dominio.Trecho{{EmpresaID: "empresa-a", IDCarona: "1", Indice: 0, Origem: "A", Destino: "B"}}}
	tx := TransacaoCoordenada{ID: "tx-imutavel", Passageiro: "ana", Itinerario: it, Estado: CoordenacaoPreparando, Participantes: []ParticipanteCoordenacao{{EmpresaID: "empresa-a", Itinerario: it}}}
	if err := repo.CriarTransacaoCoordenada(tx); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarcarPreparacaoTentada(tx.ID, "empresa-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RegistrarParticipantePreparado(tx.ID, "empresa-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DefinirDecisaoCoordenada(tx.ID, DecisaoConfirmar); err != nil {
		t.Fatal(err)
	}
	if err := repo.DefinirDecisaoCoordenada(tx.ID, DecisaoAbortar); err == nil {
		t.Fatal("não deveria permitir trocar CONFIRMAR por ABORTAR")
	}
}
