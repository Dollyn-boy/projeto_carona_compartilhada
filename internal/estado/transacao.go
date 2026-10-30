package estado

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"vaijunto/internal/concorrencia"
	"vaijunto/internal/dominio"
)

// Estados possíveis de uma transação local.
type EstadoTransacao string

const (
	EstadoPreparada  EstadoTransacao = "PREPARADA"
	EstadoConfirmada EstadoTransacao = "CONFIRMADA"
	EstadoAbortada   EstadoTransacao = "ABORTADA"
)

var (
	ErrTransacaoNaoEncontrada = errors.New("transacao nao encontrada")
	ErrTransacaoConflitante   = errors.New("identificador de transacao ja utilizado com outros dados")
	ErrEstadoTransacao        = errors.New("operacao invalida para o estado atual da transacao")
)

// TransacaoLocal registra a participação de uma empresa em uma transação.
// O itinerário contém somente os trechos pertencentes a esta empresa.
type TransacaoLocal struct {
	ID           string             `json:"id"`
	Passageiro   string             `json:"passageiro,omitempty"`
	Itinerario   dominio.Itinerario `json:"itinerario,omitempty"`
	Estado       EstadoTransacao    `json:"estado"`
	ReservaID    int                `json:"reserva_id,omitempty"`
	CriadaEm     time.Time          `json:"criada_em"`
	AtualizadaEm time.Time          `json:"atualizada_em"`
}

// PrepararTransacao valida e bloqueia temporariamente um assento em cada
// trecho local do itinerário. A preparação não cria uma Reserva definitiva.
// Repetir a mesma solicitação com o mesmo ID não desconta os assentos novamente.
func (r *Repositorio) PrepararTransacao(
	idTransacao string,
	passageiro string,
	itinerario dominio.Itinerario,
) (TransacaoLocal, error) {
	idTransacao = strings.TrimSpace(idTransacao)
	passageiro = strings.TrimSpace(passageiro)
	if idTransacao == "" {
		return TransacaoLocal{}, fmt.Errorf("id da transacao nao pode ser vazio")
	}
	if passageiro == "" {
		return TransacaoLocal{}, fmt.Errorf("passageiro nao pode ser vazio")
	}

	itinerario = clonarItinerario(itinerario)
	chaves, _, err := validarItinerarioBasico(r.empresaID, itinerario)
	if err != nil {
		return TransacaoLocal{}, err
	}

	var resultado TransacaoLocal
	err = r.trava.ComTrechos(chaves, func() error {
		return r.alterar(func() error {
			// A checagem acontece dentro do mutex do repositório. Assim, duas
			// mensagens simultâneas com o mesmo ID não criam duas preparações.
			if existente, ok := r.transacoes[idTransacao]; ok {
				if existente.Passageiro != passageiro || !reflect.DeepEqual(existente.Itinerario, itinerario) {
					return fmt.Errorf("%w: %s", ErrTransacaoConflitante, idTransacao)
				}
				if existente.Estado == EstadoAbortada {
					return fmt.Errorf("%w: transacao %s ja foi abortada", ErrEstadoTransacao, idTransacao)
				}
				resultado = clonarTransacao(*existente)
				return nil
			}

			_, _, err := validarItinerarioLocal(r.empresaID, itinerario, r.caronas)
			if err != nil {
				return err
			}

			// Valida todos os contadores antes de descontar qualquer assento.
			for _, trecho := range itinerario.Passos {
				livres, existe := r.assentos[trecho.IDCarona]
				if !existe || trecho.Indice < 0 || trecho.Indice >= len(livres) {
					return fmt.Errorf("contador de assentos inexistente para o trecho %s/%d", trecho.IDCarona, trecho.Indice)
				}
				if livres[trecho.Indice] < 1 {
					return fmt.Errorf("sem assento disponivel no trecho %s/%d", trecho.IDCarona, trecho.Indice)
				}
			}

			// Depois de todas as verificações, aplica a preparação.
			for _, trecho := range itinerario.Passos {
				r.assentos[trecho.IDCarona][trecho.Indice]--
			}

			agora := time.Now().UTC()
			transacao := &TransacaoLocal{
				ID:           idTransacao,
				Passageiro:   passageiro,
				Itinerario:   itinerario,
				Estado:       EstadoPreparada,
				CriadaEm:     agora,
				AtualizadaEm: agora,
			}
			r.transacoes[idTransacao] = transacao
			resultado = clonarTransacao(*transacao)
			return nil
		})
	})
	if err != nil {
		return TransacaoLocal{}, err
	}
	return resultado, nil
}

// ConfirmarTransacao transforma uma preparação em uma Reserva definitiva.
// A operação não desconta os assentos novamente: eles foram bloqueados
// quando PrepararTransacao foi executada.
func (r *Repositorio) ConfirmarTransacao(idTransacao string) (dominio.Reserva, error) {
	idTransacao = strings.TrimSpace(idTransacao)
	if idTransacao == "" {
		return dominio.Reserva{}, fmt.Errorf("id da transacao nao pode ser vazio")
	}

	var resultado dominio.Reserva
	err := r.alterar(func() error {
		transacao, existe := r.transacoes[idTransacao]
		if !existe {
			return fmt.Errorf("%w: %s", ErrTransacaoNaoEncontrada, idTransacao)
		}

		switch transacao.Estado {
		case EstadoConfirmada:
			reserva, existe := r.reservas[strconv.Itoa(transacao.ReservaID)]
			if !existe {
				return fmt.Errorf("reserva %d da transacao %s nao esta mais ativa", transacao.ReservaID, idTransacao)
			}
			resultado = clonarReserva(*reserva)
			return nil
		case EstadoAbortada:
			return fmt.Errorf("%w: transacao %s esta ABORTADA", ErrEstadoTransacao, idTransacao)
		case EstadoPreparada:
			// Continua abaixo.
		default:
			return fmt.Errorf("estado desconhecido na transacao %s: %q", idTransacao, transacao.Estado)
		}

		if transacao.Passageiro == "" || len(transacao.Itinerario.Passos) == 0 {
			return fmt.Errorf("transacao %s preparada sem passageiro ou itinerario", idTransacao)
		}

		_, idsCarona, err := validarItinerarioLocal(r.empresaID, transacao.Itinerario, r.caronas)
		if err != nil {
			return fmt.Errorf("nao foi possivel confirmar a transacao %s: %w", idTransacao, err)
		}

		itens := make([]dominio.ItemReserva, len(transacao.Itinerario.Passos))
		for i, trecho := range transacao.Itinerario.Passos {
			itens[i] = dominio.ItemReserva{
				CaronaID: idsCarona[i],
				Trecho:   trecho,
			}
		}

		idReserva := r.proximoIDReserva + 1
		reserva := dominio.Reserva{
			Id:         idReserva,
			Passageiro: transacao.Passageiro,
			Itens:      itens,
		}

		// Não altera r.assentos aqui. A preparação já descontou esses assentos.
		r.proximoIDReserva = idReserva
		copiaReserva := clonarReserva(reserva)
		r.reservas[strconv.Itoa(idReserva)] = &copiaReserva

		transacao.Estado = EstadoConfirmada
		transacao.ReservaID = idReserva
		transacao.AtualizadaEm = time.Now().UTC()
		resultado = clonarReserva(reserva)
		return nil
	})
	if err != nil {
		return dominio.Reserva{}, err
	}
	return resultado, nil
}

// AbortarTransacao desfaz uma preparação, devolvendo exatamente os assentos
// que ela bloqueou. Se o ID ainda não existir, grava um tombstone ABORTADA
// para impedir que uma mensagem de preparação atrasada reabra a operação.
func (r *Repositorio) AbortarTransacao(idTransacao string) error {
	idTransacao = strings.TrimSpace(idTransacao)
	if idTransacao == "" {
		return fmt.Errorf("id da transacao nao pode ser vazio")
	}

	return r.alterar(func() error {
		transacao, existe := r.transacoes[idTransacao]
		if !existe {
			agora := time.Now().UTC()
			r.transacoes[idTransacao] = &TransacaoLocal{
				ID:           idTransacao,
				Estado:       EstadoAbortada,
				CriadaEm:     agora,
				AtualizadaEm: agora,
			}
			return nil
		}

		switch transacao.Estado {
		case EstadoAbortada:
			// Repetir um aborto não devolve os assentos uma segunda vez.
			return nil
		case EstadoConfirmada:
			return fmt.Errorf("%w: transacao %s ja esta CONFIRMADA", ErrEstadoTransacao, idTransacao)
		case EstadoPreparada:
			// Continua abaixo.
		default:
			return fmt.Errorf("estado desconhecido na transacao %s: %q", idTransacao, transacao.Estado)
		}

		// Valida todos os trechos e contadores antes de devolver qualquer assento.
		_, _, err := validarItinerarioLocal(r.empresaID, transacao.Itinerario, r.caronas)
		if err != nil {
			return fmt.Errorf("nao foi possivel abortar a transacao %s: %w", idTransacao, err)
		}
		for _, trecho := range transacao.Itinerario.Passos {
			carona := r.caronas[trecho.IDCarona]
			livres := r.assentos[trecho.IDCarona]
			if trecho.Indice >= len(livres) {
				return fmt.Errorf("contador de assentos invalido para o trecho %s/%d", trecho.IDCarona, trecho.Indice)
			}
			if livres[trecho.Indice] >= carona.Capacidade {
				return fmt.Errorf("devolver o assento da transacao %s excederia a capacidade da carona %s", idTransacao, trecho.IDCarona)
			}
		}

		for _, trecho := range transacao.Itinerario.Passos {
			r.assentos[trecho.IDCarona][trecho.Indice]++
		}
		transacao.Estado = EstadoAbortada
		transacao.AtualizadaEm = time.Now().UTC()
		return nil
	})
}

// ConsultarTransacao devolve uma cópia do registro local da transação.
func (r *Repositorio) ConsultarTransacao(idTransacao string) (TransacaoLocal, error) {
	idTransacao = strings.TrimSpace(idTransacao)
	if idTransacao == "" {
		return TransacaoLocal{}, fmt.Errorf("id da transacao nao pode ser vazio")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	transacao, existe := r.transacoes[idTransacao]
	if !existe {
		return TransacaoLocal{}, fmt.Errorf("%w: %s", ErrTransacaoNaoEncontrada, idTransacao)
	}
	return clonarTransacao(*transacao), nil
}

func clonarItinerario(itinerario dominio.Itinerario) dominio.Itinerario {
	return dominio.Itinerario{
		Passos: append([]dominio.Trecho(nil), itinerario.Passos...),
	}
}

func clonarTransacao(transacao TransacaoLocal) TransacaoLocal {
	transacao.Itinerario = clonarItinerario(transacao.Itinerario)
	return transacao
}

// validarItinerarioBasico verifica os dados que não dependem de consultar
// o mapa de caronas. É chamada antes de adquirir as travas por trecho.
func validarItinerarioBasico(
	empresaID string,
	itinerario dominio.Itinerario,
) ([]string, []int, error) {
	if len(itinerario.Passos) == 0 {
		return nil, nil, fmt.Errorf("itinerario vazio")
	}

	chaves := make([]string, len(itinerario.Passos))
	idsCarona := make([]int, len(itinerario.Passos))
	vistos := make(map[string]bool, len(itinerario.Passos))

	for i, trecho := range itinerario.Passos {
		if trecho.EmpresaID != empresaID {
			return nil, nil, fmt.Errorf(
				"trecho da empresa %q nao pode ser preparado pelo servidor da empresa %q",
				trecho.EmpresaID,
				empresaID,
			)
		}
		idCarona, err := strconv.Atoi(trecho.IDCarona)
		if err != nil || idCarona <= 0 || strconv.Itoa(idCarona) != trecho.IDCarona {
			return nil, nil, fmt.Errorf("id de carona invalido no trecho: %q", trecho.IDCarona)
		}
		if trecho.Indice < 0 {
			return nil, nil, fmt.Errorf("indice de trecho invalido: %d", trecho.Indice)
		}
		if strings.TrimSpace(string(trecho.Origem)) == "" || strings.TrimSpace(string(trecho.Destino)) == "" {
			return nil, nil, fmt.Errorf("origem e destino do trecho nao podem ser vazios")
		}

		chave := concorrencia.ChaveTrecho(trecho.IDCarona, trecho.Indice)
		if vistos[chave] {
			return nil, nil, fmt.Errorf("itinerario contem o mesmo trecho mais de uma vez")
		}
		vistos[chave] = true
		chaves[i] = chave
		idsCarona[i] = idCarona

		if i > 0 && itinerario.Passos[i-1].Destino != trecho.Origem {
			return nil, nil, fmt.Errorf("itinerario possui trechos desconectados entre %q e %q", itinerario.Passos[i-1].Destino, trecho.Origem)
		}
	}

	return chaves, idsCarona, nil
}

// validarItinerarioLocal confirma que cada trecho corresponde à rota real
// de uma carona pertencente ao repositório. O chamador protege os mapas.
func validarItinerarioLocal(
	empresaID string,
	itinerario dominio.Itinerario,
	caronas map[string]*dominio.Carona,
) ([]string, []int, error) {
	chaves, idsCarona, err := validarItinerarioBasico(empresaID, itinerario)
	if err != nil {
		return nil, nil, err
	}

	for i, trecho := range itinerario.Passos {
		carona, existe := caronas[trecho.IDCarona]
		if !existe {
			return nil, nil, fmt.Errorf("carona %s nao encontrada", trecho.IDCarona)
		}
		if trecho.Indice >= len(carona.Rota)-1 {
			return nil, nil, fmt.Errorf("indice de trecho invalido: carona %s indice %d", trecho.IDCarona, trecho.Indice)
		}
		if carona.Rota[trecho.Indice] != trecho.Origem || carona.Rota[trecho.Indice+1] != trecho.Destino {
			return nil, nil, fmt.Errorf("trecho %s/%d nao corresponde a rota cadastrada", trecho.IDCarona, trecho.Indice)
		}
		if idsCarona[i] <= 0 {
			return nil, nil, fmt.Errorf("id de carona invalido no trecho %q", trecho.IDCarona)
		}
	}

	return chaves, idsCarona, nil
}

func itensReservaDaTransacao(transacao TransacaoLocal) ([]dominio.ItemReserva, error) {
	itens := make([]dominio.ItemReserva, len(transacao.Itinerario.Passos))
	for i, trecho := range transacao.Itinerario.Passos {
		idCarona, err := strconv.Atoi(trecho.IDCarona)
		if err != nil || idCarona <= 0 {
			return nil, fmt.Errorf("id de carona invalido no trecho: %q", trecho.IDCarona)
		}
		itens[i] = dominio.ItemReserva{CaronaID: idCarona, Trecho: trecho}
	}
	return itens, nil
}

func transacaoIgualReserva(transacao TransacaoLocal, reserva dominio.Reserva) bool {
	itens, err := itensReservaDaTransacao(transacao)
	if err != nil {
		return false
	}
	return reserva.Id == transacao.ReservaID &&
		reserva.Passageiro == transacao.Passageiro &&
		reflect.DeepEqual(reserva.Itens, itens)
}
