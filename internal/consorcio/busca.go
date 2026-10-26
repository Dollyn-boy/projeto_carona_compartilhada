package consorcio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

type Peer struct {
	EmpresaID string
	URL       string
}

type ResultadoBusca struct {
	Itinerarios           []dominio.Itinerario
	BuscaCompleta         bool
	EmpresasIndisponiveis []string
}

type BuscaDistribuida struct {
	Repositorio *estado.Repositorio
	Peers       []Peer
	HTTP        *http.Client
}

func NovoBuscaDistribuida(
	repo *estado.Repositorio,
	peers []Peer,
) *BuscaDistribuida {
	return &BuscaDistribuida{
		Repositorio: repo,
		Peers:       append([]Peer(nil), peers...),
		HTTP: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// Buscar consulta a disponibilidade local e dos peers, reúne as arestas
// e executa o algoritmo de busca que já existe no pacote dominio.
func (b *BuscaDistribuida) Buscar(
	ctx context.Context,
	origem, destino dominio.Cidade,
	data time.Time,
	passageiros int,
) (ResultadoBusca, error) {
	var resultado ResultadoBusca

	locais, err := b.Repositorio.ListarTrechosDisponiveis(data, passageiros)
	if err != nil {
		return resultado, err
	}

	arestas := append([]dominio.ArestaDisponivel(nil), locais...)
	resultado.BuscaCompleta = true

	for _, peer := range b.Peers {
		remotas, err := b.buscarTrechosPeer(ctx, peer, data, passageiros)
		if err != nil {
			resultado.BuscaCompleta = false
			resultado.EmpresasIndisponiveis = append(
				resultado.EmpresasIndisponiveis,
				peer.EmpresaID,
			)
			continue
		}

		arestas = append(arestas, remotas...)
	}

	grafo := dominio.ConstruirGrafo(arestas)
	resultado.Itinerarios = dominio.BuscarTodosItinerarios(
		grafo,
		origem,
		destino,
	)

	// Primeiro os itinerários com menos trechos. O desempate usa
	// uma assinatura global, não o preço do repositório local:
	// o ID de carona pode repetir em outras empresas.
	sort.Slice(resultado.Itinerarios, func(i, j int) bool {
		a := resultado.Itinerarios[i]
		c := resultado.Itinerarios[j]

		if len(a.Passos) != len(c.Passos) {
			return len(a.Passos) < len(c.Passos)
		}
		return assinatura(a) < assinatura(c)
	})

	sort.Strings(resultado.EmpresasIndisponiveis)
	return resultado, nil
}

func assinatura(it dominio.Itinerario) string {
	var partes []string
	for _, t := range it.Passos {
		partes = append(
			partes,
			fmt.Sprintf("%s/%s/%d", t.EmpresaID, t.IDCarona, t.Indice),
		)
	}
	return strings.Join(partes, "|")
}

func (b *BuscaDistribuida) buscarTrechosPeer(
	ctx context.Context,
	peer Peer,
	data time.Time,
	passageiros int,
) ([]dominio.ArestaDisponivel, error) {
	base, err := url.Parse(strings.TrimRight(peer.URL, "/"))
	if err != nil {
		return nil, fmt.Errorf("URL do peer %s invalida: %w", peer.EmpresaID, err)
	}

	base.Path += "/v1/trechos"
	query := base.Query()
	query.Set("data", data.Format("2006-01-02"))
	query.Set("passageiros", fmt.Sprint(passageiros))
	base.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		base.String(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	cliente := b.HTTP
	if cliente == nil {
		cliente = &http.Client{Timeout: 3 * time.Second}
	}

	resp, err := cliente.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"peer %s respondeu HTTP %d",
			peer.EmpresaID,
			resp.StatusCode,
		)
	}

	var resposta protocolo.TrechosDisponiveisResposta
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&resposta); err != nil {
		return nil, fmt.Errorf("resposta invalida do peer %s: %w", peer.EmpresaID, err)
	}

	// Não aceite uma resposta que se identifica como outra empresa.
	if resposta.EmpresaID != peer.EmpresaID {
		return nil, fmt.Errorf(
			"identidade incorreta: esperado %s, recebido %s",
			peer.EmpresaID,
			resposta.EmpresaID,
		)
	}

	arestas := make([]dominio.ArestaDisponivel, 0, len(resposta.Trechos))
	for _, item := range resposta.Trechos {
		t := item.Trecho

		// Os trechos retornados por um peer devem pertencer a ele.
		if t.EmpresaID != peer.EmpresaID {
			return nil, fmt.Errorf(
				"peer %s devolveu trecho da empresa %s",
				peer.EmpresaID,
				t.EmpresaID,
			)
		}

		arestas = append(arestas, dominio.ArestaDisponivel{
			Trecho: dominio.Trecho{
				EmpresaID: t.EmpresaID,
				IDCarona:  t.IDCarona,
				Indice:    t.Indice,
				Origem:    dominio.Cidade(t.Origem),
				Destino:   dominio.Cidade(t.Destino),
			},
			AssentosDisponiveis: item.AssentosDisponiveis,
			Preco:               float64(item.Preco),
		})
	}

	return arestas, nil
}
