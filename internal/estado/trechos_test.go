package estado

import (
	"testing"
	"time"
	"vaijunto/internal/dominio"
)

func TestListarTrechosDisponiveisFiltraAssentos(t *testing.T) {
	repo, err := NovoRepositorioDeEmpresa("empresa-a")
	if err != nil {
		t.Fatal(err)
	}

	data := dataFuturaTeste()
	carona, err := repo.PublicarCarona(
		"motorista-a",
		[]dominio.Cidade{"A", "B", "C"},
		2,
		10,
		data,
	)
	if err != nil {
		t.Fatal(err)
	}

	trechos, err := repo.ListarTrechosDisponiveis(data, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(trechos) != 2 {
		t.Fatalf("esperados 2 trechos, recebidos %d", len(trechos))
	}

	for _, trecho := range trechos {
		if trecho.Trecho.EmpresaID != "empresa-a" {
			t.Fatalf("empresa incorreta no trecho: %q", trecho.Trecho.EmpresaID)
		}
		if trecho.AssentosDisponiveis != 2 {
			t.Fatalf(
				"esperados 2 assentos, encontrados %d",
				trecho.AssentosDisponiveis,
			)
		}
	}

	// Um grupo maior que a capacidade não deve encontrar esses trechos.
	trechos, err = repo.ListarTrechosDisponiveis(data, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(trechos) != 0 {
		t.Fatalf("esperava nenhum trecho para 3 passageiros, encontrei %d", len(trechos))
	}

	_ = carona
}

func dataFuturaTeste() time.Time {
	return time.Now().Add(48 * time.Hour)
}
