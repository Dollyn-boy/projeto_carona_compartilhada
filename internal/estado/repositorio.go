// Package estado é a camada "Estado em memória" da arquitetura: guarda
// de fato os dados do servidor (caronas, trechos, reservas) e é o único
// pacote que combina os tipos de internal/dominio com as travas de
// internal/concorrencia para expor operações seguras (já protegidas
// contra corrida) para o pacote casosdeuso chamar.
package estado

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
	"vaijunto/internal/concorrencia"
	"vaijunto/internal/dominio"
)

type EstadoTx int

const (
	TxPreparada EstadoTx = iota + 1
	TxConfirmada
	TxAbortada
)

type Hold struct {
	TxID    string
	Trechos []dominio.Trecho // todos os trechos DESTA empresa na transação
	Estado  EstadoTx
}

var (
	ErrIndisponivel   = errors.New("trecho indisponivel")
	ErrTxDesconhecida = errors.New("transacao desconhecida")
	ErrTxAbortada     = errors.New("transacao abortada")
	ErrEstadoInvalido = errors.New("estado invalido")
)

// Repositorio guarda o estado real do servidor em memória. As chaves
// dos três mapas são a mesma string: strconv.Itoa(carona.Id) (ou
// reserva.Id).
type Repositorio struct {
	trava      *concorrencia.Trava
	empresa    string
	caronas    map[string]*dominio.Carona  // chave: strconv.Itoa(carona.Id)
	assentos   map[string][]int            // mesma chave; indexado por Trecho.Indice
	reservas   map[string]*dominio.Reserva // chave: strconv.Itoa(reserva.Id)
	usuarios   map[string]*dominio.Usuario // chave: usuario.Usuario
	transacoes map[string]*Hold            // chave: TxID

	// Geradores simples de ID sequencial. So sao alterados dentro de
	// ComEscrita, entao nao precisam de sync/atomic separado.
	proximoIDCarona  int
	proximoIDReserva int
}

// Preparar é o primeiro passo do protocolo de duas fases: verifica se os
// trechos estão disponíveis e, se sim, reserva-os (diminuindo os
// contadores de assento). Se não, grava o voto "não" (TxAbortada) e
// devolve erro. Se sim, grava o voto "sim" (TxPreparada) e devolve nil.
func (r *Repositorio) Preparar(txID string, ts []dominio.Trecho) error {
	return r.trava.ComEscrita(func() error {
		// Se a transacao ja existe, nao precisa verificar disponibilidade de novo.
		if h, ok := r.transacoes[txID]; ok {
			// Se a transacao ja foi abortada, devolve erro. Se ja foi preparada ou confirmada, devolve nil (idempotente).
			if h.Estado == TxAbortada {
				return ErrTxAbortada
			}
			return nil
		}
		// Se a transacao nao existe, verifica disponibilidade e ajusta assentos.
		// Se falhar, grava TxAbortada e devolve erro. Se passar, grava TxPreparada e devolve nil.
		if err := r.verificarDisponibilidade(ts); err != nil {
			r.transacoes[txID] = &Hold{TxID: txID, Estado: TxAbortada} // grava o voto "não"
			return err
		}
		r.ajustarAssentos(ts, -1)
		r.transacoes[txID] = &Hold{TxID: txID, Trechos: ts, Estado: TxPreparada}
		return nil
	})
}

// Confirmar é o segundo passo do protocolo de duas fases: se a transação
// estiver em estado TxPreparada, muda para TxConfirmada. Se já estiver
// em TxConfirmada, devolve nil (idempotente). Se estiver em TxAbortada,
// devolve erro.
func (r *Repositorio) Confirmar(txID string) error {
	return r.trava.ComEscrita(func() error {
		h, ok := r.transacoes[txID]
		if !ok {
			return ErrTxDesconhecida
		}
		switch h.Estado {
		case TxAbortada:
			return ErrTxAbortada
		case TxPreparada:
			h.Estado = TxConfirmada
		}
		return nil // já confirmada: idempotente
	})
}

func (r *Repositorio) Abortar(txID string) error {
	return r.trava.ComEscrita(func() error {
		h, ok := r.transacoes[txID]
		if !ok {
			r.transacoes[txID] = &Hold{TxID: txID, Estado: TxAbortada} // "túmulo"
			return nil
		}
		switch h.Estado {
		case TxConfirmada:
			return ErrEstadoInvalido
		case TxPreparada:
			r.ajustarAssentos(h.Trechos, +1)
			h.Estado = TxAbortada
		}
		return nil // já abortada: idempotente
	})
}

// NovoRepositorio cria um Repositorio vazio, pronto para uso.
func NovoRepositorio(empresa string) *Repositorio {
	return &Repositorio{
		trava:      concorrencia.NovaTrava(),
		empresa:    empresa,
		caronas:    make(map[string]*dominio.Carona),
		assentos:   make(map[string][]int),
		reservas:   make(map[string]*dominio.Reserva),
		usuarios:   make(map[string]*dominio.Usuario),
		transacoes: make(map[string]*Hold),
	}
}

// PublicarCarona cria uma nova carona e os contadores de assento (um
// por trecho elementar da rota, todos comecando em "capacidade").
func (r *Repositorio) PublicarCarona(motorista string, rota []dominio.Cidade, capacidade int, preco int, data time.Time) (dominio.Carona, error) {
	if len(rota) < 2 {
		return dominio.Carona{}, fmt.Errorf("rota precisa de pelo menos 2 cidades, veio %d", len(rota))
	}
	if capacidade <= 0 {
		return dominio.Carona{}, fmt.Errorf("capacidade precisa ser maior que zero")
	}

	if data.Before(time.Now()) {
		return dominio.Carona{}, fmt.Errorf("data da carona não pode estar no passado")
	}

	var carona dominio.Carona

	err := r.trava.ComEscrita(func() error {
		r.proximoIDCarona++
		// chave é a string que usamos como chave nos mapas e tambem como
		// dominio.Trecho.IDCarona — evita converter int<->string em varios
		// lugares do código.
		chave := strconv.Itoa(r.proximoIDCarona)

		carona = dominio.Carona{
			Id:         r.proximoIDCarona,
			Motorista:  motorista,
			Rota:       rota,
			Preco:      preco,
			Capacidade: capacidade,
			Data:       data,
		}

		// Um contador por trecho elementar — dominio.TrechosDaRota so e
		// usada aqui pra saber QUANTOS trechos existem (len(rota)-1);
		// os proprios Trecho retornados nao sao guardados, so recriados
		// sob demanda em BuscarItinerarios a partir de r.caronas.

		// rota de [A, B, C] tem 2 trechos: A->B e B->C
		numTrechos := len(rota) - 1

		// assentosPorTrecho[i] é o contador de assentos livres no trecho i da carona
		assentosPorTrecho := make([]int, numTrechos)

		for i := range assentosPorTrecho {
			assentosPorTrecho[i] = capacidade
		}

		r.caronas[chave] = &carona
		r.assentos[chave] = assentosPorTrecho
		return nil
	})

	return carona, err
}

type CaronaComOcupacao struct {
	Carona               dominio.Carona
	PassageirosPorTrecho [][]string // indice = Trecho.Indice; valor = nomes dos passageiros confirmados naquele trecho
}

// ConsultarCaronas devolve as caronas de um motorista, cada uma com a
// ocupacao (quem confirmou o que) em cada trecho — cruzando r.caronas
// com r.reservas.
func (r *Repositorio) ConsultarCaronas(motorista string) ([]CaronaComOcupacao, error) {
	var resultado []CaronaComOcupacao

	err := r.trava.ComLeitura(func() error {
		for chave, carona := range r.caronas {
			if carona.Motorista != motorista {
				continue
			}

			numTrechos := len(carona.Rota) - 1
			passageirosPorTrecho := make([][]string, numTrechos)

			for _, reserva := range r.reservas {
				for _, item := range reserva.Itens {
					if strconv.Itoa(item.CaronaID) != chave {
						continue
					}
					if item.Trecho.Indice < 0 || item.Trecho.Indice >= numTrechos {
						continue
					}
					passageirosPorTrecho[item.Trecho.Indice] = append(
						passageirosPorTrecho[item.Trecho.Indice],
						reserva.Passageiro,
					)
				}
			}

			resultado = append(resultado, CaronaComOcupacao{
				Carona:               *carona,
				PassageirosPorTrecho: passageirosPorTrecho,
			})
		}
		return nil
	})

	return resultado, err
}

// CancelarCarona remove uma carona e seus contadores de assento, e
// cancela em cascata qualquer reserva que a referencie.

func (r *Repositorio) CancelarCarona(idCarona int, motorista string) error {
	chave := strconv.Itoa(idCarona)

	return r.trava.ComEscrita(func() error {
		// Agora precisamos pegar o ponteiro da carona para checar quem é o dono
		carona, existe := r.caronas[chave]
		if !existe {
			return fmt.Errorf("carona %d nao encontrada", idCarona)
		}

		if carona.Motorista != motorista {
			return fmt.Errorf("acesso negado: voce nao tem permissao para cancelar a carona %d", idCarona)
		}

		for _, h := range r.transacoes {
			if h.Estado == TxAbortada {
				continue
			}
			for _, t := range h.Trechos {
				if t.IDCarona == chave {
					return fmt.Errorf("carona %d tem transacoes pendentes ou confirmadas", idCarona)
				}
			}
		}

		for chaveReserva, reserva := range r.reservas {
			tocaEssaCarona := false
			for _, item := range reserva.Itens {
				if item.CaronaID == idCarona {
					tocaEssaCarona = true
					continue
				}
				// Devolve o assento nas OUTRAS caronas do itinerario
				if contadores, ok := r.assentos[strconv.Itoa(item.CaronaID)]; ok && item.Trecho.Indice < len(contadores) {
					contadores[item.Trecho.Indice]++
				}
			}
			if tocaEssaCarona {
				delete(r.reservas, chaveReserva)
			}
		}

		delete(r.caronas, chave)
		delete(r.assentos, chave)
		return nil
	})
}

// VerificarReserva confere se uma reserva ja confirmada ainda e valida.
func (r *Repositorio) VerificarReserva(idReserva int) (bool, error) {
	chave := strconv.Itoa(idReserva)
	var valida bool

	err := r.trava.ComLeitura(func() error {
		reserva, existe := r.reservas[chave]
		if !existe {
			return fmt.Errorf("reserva %d nao encontrada", idReserva)
		}

		valida = true
		for _, item := range reserva.Itens {
			contadores, ok := r.assentos[strconv.Itoa(item.CaronaID)]
			if !ok || item.Trecho.Indice >= len(contadores) {
				// a carona (ou o trecho) referenciado nao existe mais
				valida = false
				return nil
			}
		}
		return nil
	})

	return valida, err
}

func (r *Repositorio) BuscarItinerarios(origem, destino dominio.Cidade) ([]dominio.Itinerario, error) {
	var itinerarios []dominio.Itinerario

	err := r.trava.ComLeitura(func() error {
		var disponiveis []dominio.ArestaDisponivel

		for chave, carona := range r.caronas {
			// Ignora caronas cuja data ja passou.
			if carona.Data.Before(time.Now()) {
				continue
			}

			trechos := dominio.TrechosDaRota(r.empresa, chave, carona.Rota)
			contadores := r.assentos[chave]

			for _, trecho := range trechos {
				assentosLivres := contadores[trecho.Indice]
				if assentosLivres <= 0 {
					continue // sem assento neste trecho — nao entra no grafo de busca
				}

				disponiveis = append(disponiveis, dominio.ArestaDisponivel{
					Trecho:              trecho,
					AssentosDisponiveis: assentosLivres,
					Preco:               float64(carona.Preco),
				})
			}
		}

		grafo := dominio.ConstruirGrafo(disponiveis)
		itinerarios = dominio.BuscarTodosItinerarios(grafo, origem, destino)

		// Criterio de ordenacao: menos trechos primeiro; empatando,
		// menor preco total primeiro. Continua dentro da trava porque
		// precoTotalItinerario le r.caronas.
		sort.Slice(itinerarios, func(i, j int) bool {
			if len(itinerarios[i].Passos) != len(itinerarios[j].Passos) {
				return len(itinerarios[i].Passos) < len(itinerarios[j].Passos)
			}
			return r.precoTotalItinerario(itinerarios[i]) < r.precoTotalItinerario(itinerarios[j])
		})

		return nil
	})

	return itinerarios, err
}

// precoTotalItinerario soma o preco das caronas envolvidas num
// itinerario. So deve ser chamada de dentro de um ComLeitura/ComEscrita
// (le r.caronas diretamente, sem trava propria).
func (r *Repositorio) precoTotalItinerario(it dominio.Itinerario) int {
	total := 0
	for _, passo := range it.Passos {
		if carona, ok := r.caronas[passo.IDCarona]; ok {
			total += carona.Preco
		}
	}
	return total
}

type chaveTrecho struct {
	carona string
	indice int
}

func (r *Repositorio) verificarDisponibilidade(it []dominio.Trecho) error {
	precisa := map[chaveTrecho]int{}
	for _, passo := range it {
		if passo.Empresa != r.empresa {
			return fmt.Errorf("trecho de carona de outra empresa: %s", passo.Empresa)
		}
		precisa[chaveTrecho{passo.IDCarona, passo.Indice}]++
	}

	for k, n := range precisa {
		c, ok := r.assentos[k.carona]
		if !ok || k.indice < 0 || k.indice >= len(c) {
			return fmt.Errorf("trecho invalido")
		}
		if c[k.indice] < n {
			return fmt.Errorf("%w: carona %s trecho %d", ErrIndisponivel, k.carona, k.indice)
		}
	}

	return nil
}

func (r *Repositorio) ajustarAssentos(ts []dominio.Trecho, delta int) {
	for _, t := range ts {
		if c, ok := r.assentos[t.IDCarona]; ok && t.Indice >= 0 && t.Indice < len(c) {
			c[t.Indice] += delta
		}
	}
}

// ConfirmarReserva e o metodo mais importante do arquivo: recebe um
// itinerario JA ESCOLHIDO (um ou mais trechos, possivelmente de caronas
// diferentes) e confirma TODOS os trechos ou NENHUM.
func (r *Repositorio) ConfirmarReserva(passageiro string, itinerario dominio.Itinerario) (dominio.Reserva, error) {
	var reserva dominio.Reserva

	if len(itinerario.Passos) == 0 {
		return reserva, fmt.Errorf("itinerario vazio")
	}

	err := r.trava.ComEscrita(func() error {

		err := r.verificarDisponibilidade(itinerario.Passos)
		if err != nil {
			return err
		}

		itens := make([]dominio.ItemReserva, len(itinerario.Passos))
		for i, trecho := range itinerario.Passos {
			idCarona, err := strconv.Atoi(trecho.IDCarona)
			if err != nil {
				return fmt.Errorf("id de carona invalido no trecho: %q", trecho.IDCarona)
			}
			itens[i] = dominio.ItemReserva{CaronaID: idCarona, Trecho: trecho}
		}
		// 2. A partir daqui nada falha: aplica.
		r.ajustarAssentos(itinerario.Passos, -1)

		r.proximoIDReserva++
		reserva = dominio.Reserva{Id: r.proximoIDReserva, Passageiro: passageiro, Itens: itens}
		r.reservas[strconv.Itoa(reserva.Id)] = &reserva

		return nil
	})

	return reserva, err
}

// ConsultarReservas devolve as reservas de um passageiro.
func (r *Repositorio) ConsultarReservas(passageiro string) ([]dominio.Reserva, error) {
	var resultado []dominio.Reserva

	err := r.trava.ComLeitura(func() error {
		for _, reserva := range r.reservas {
			if reserva.Passageiro == passageiro {
				resultado = append(resultado, *reserva)
			}
		}
		return nil
	})

	return resultado, err
}

// CancelarReserva remove uma reserva E devolve os assentos que ela
// ocupava para os contadores correspondentes.
func (r *Repositorio) CancelarReserva(idReserva int, passageiro string) error {
	chave := strconv.Itoa(idReserva)

	return r.trava.ComEscrita(func() error {
		reserva, existe := r.reservas[chave]
		if !existe {
			return fmt.Errorf("reserva %d nao encontrada", idReserva)
		}

		if reserva.Passageiro != passageiro {
			return fmt.Errorf("acesso negado: voce nao tem permissao para cancelar a reserva %d", idReserva)
		}

		for _, item := range reserva.Itens {
			trecho := item.Trecho
			if contadores, ok := r.assentos[trecho.IDCarona]; ok && trecho.Indice < len(contadores) {
				contadores[trecho.Indice]++
			}
		}

		delete(r.reservas, chave)
		return nil
	})
}

func (r *Repositorio) CadastrarUsuario(usuario string, senha string, role string) error {
	return r.trava.ComEscrita(func() error {
		if _, existe := r.usuarios[usuario]; existe {
			return fmt.Errorf("usuario %s ja cadastrado", usuario)
		}

		r.usuarios[usuario] = &dominio.Usuario{Usuario: usuario, SenhaHash: dominio.HashSenha(senha), Role: role}
		return nil
	})
}

// AutenticarUsuario confere usuario/senha e devolve o Role cadastrado
// (motorista ou passageiro) para quem chamou poder decidir que menu
// mostrar, sem precisar perguntar de novo a cada login.
func (r *Repositorio) AutenticarUsuario(usuario string, senha string) (bool, string, error) {
	autenticado := false
	var role string
	err := r.trava.ComLeitura(func() error {
		usuarioObj, existe := r.usuarios[usuario]
		if !existe {
			return fmt.Errorf("usuario %s nao encontrado", usuario)
		}
		if usuarioObj.SenhaHash != dominio.HashSenha(senha) {
			return fmt.Errorf("senha incorreta para o usuario %s", usuario)
		}
		autenticado = true
		role = usuarioObj.Role
		return nil
	})
	return autenticado, role, err
}

// VerificarUsuario mantido por compatibilidade com quem so precisa do
// booleano — mas prefira AutenticarUsuario se voce tambem precisa do Role.
func (r *Repositorio) VerificarUsuario(usuario string, senha string) bool {
	autenticado, _, err := r.AutenticarUsuario(usuario, senha)
	if err != nil {
		return false
	}
	return autenticado
}
