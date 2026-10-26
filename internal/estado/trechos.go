package estado

import (
	"fmt"
	"sort"
	"time"

	"vaijunto/internal/dominio"
)

// ListarTrechosDisponiveis devolve um snapshot dos trechos locais
// que possuem assentos suficientes para a data e o grupo solicitados.
func (r *Repositorio) ListarTrechosDisponiveis(
	data time.Time,
	passageiros int,
) ([]dominio.ArestaDisponivel, error) {
	if passageiros < 1 {
		return nil, fmt.Errorf("numero de passageiros deve ser maior que zero")
	}

	layoutData := "2006-01-02"
	diaSolicitado := data.Format(layoutData)
	hoje := time.Now().In(time.Local).Format(layoutData)

	if diaSolicitado < hoje {
		return nil, fmt.Errorf("a data solicitada nao pode estar no passado")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	resultado := make([]dominio.ArestaDisponivel, 0)

	for chave, carona := range r.caronas {
		// Todas as arestas oferecidas devem corresponder ao dia pedido.
		if carona.Data.In(time.Local).Format(layoutData) != diaSolicitado {
			continue
		}

		assentosLivres, existe := r.assentos[chave]
		if !existe {
			continue
		}

		trechos := dominio.TrechosDaRota(
			r.empresaID,
			chave,
			carona.Rota,
		)

		for _, trecho := range trechos {
			indice := trecho.Indice

			if indice < 0 || indice >= len(assentosLivres) {
				continue
			}

			livres := assentosLivres[indice]
			if livres < passageiros {
				continue
			}

			resultado = append(resultado, dominio.ArestaDisponivel{
				Trecho:              trecho,
				AssentosDisponiveis: livres,
				Preco:               float64(carona.Preco),
			})
		}
	}

	// A ordem dos mapas em Go não é garantida. Ordenar os resultados
	// facilita os testes e torna as respostas mais previsíveis.
	sort.Slice(resultado, func(i, j int) bool {
		a := resultado[i].Trecho
		b := resultado[j].Trecho

		if a.Origem != b.Origem {
			return a.Origem < b.Origem
		}
		if a.Destino != b.Destino {
			return a.Destino < b.Destino
		}
		if a.EmpresaID != b.EmpresaID {
			return a.EmpresaID < b.EmpresaID
		}
		if a.IDCarona != b.IDCarona {
			return a.IDCarona < b.IDCarona
		}
		return a.Indice < b.Indice
	})

	return resultado, nil
}
