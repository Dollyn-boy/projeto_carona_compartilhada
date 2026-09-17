// Package estado é a camada "Estado em memória" da arquitetura: guarda
// de fato os dados do servidor (caronas, trechos, reservas) e é o único
// pacote que combina os tipos de internal/dominio com as travas de
// internal/concorrencia para expor operações seguras (já protegidas
// contra corrida) para o pacote casosdeuso chamar.
package estado

import (
	"fmt"
	"sort"
	"strconv"
	"time"
	"vaijunto/internal/concorrencia"
	"vaijunto/internal/dominio"
)

// Repositorio guarda o estado real do servidor em memória. As chaves
// dos três mapas são a mesma string: strconv.Itoa(carona.Id) (ou
// reserva.Id). Usar essa string tambem como dominio.Trecho.IDCarona é o
// que evita converter int<->string espalhado pelo código — a conversão
// acontece só nas bordas deste arquivo (PublicarCarona na entrada,
// ConfirmarReserva na saída).
type Repositorio struct {
	trava    *concorrencia.Trava
	caronas  map[string]*dominio.Carona  // chave: strconv.Itoa(carona.Id)
	assentos map[string][]int            // mesma chave; indexado por Trecho.Indice
	reservas map[string]*dominio.Reserva // chave: strconv.Itoa(reserva.Id)
	usuarios map[string]*dominio.Usuario // chave: usuario.Usuario

	// Geradores simples de ID sequencial. So sao alterados dentro de
	// ComEscrita, entao nao precisam de sync/atomic separado.
	proximoIDCarona  int
	proximoIDReserva int
}

// NovoRepositorio cria um Repositorio vazio, pronto para uso.
func NovoRepositorio() *Repositorio {
	return &Repositorio{
		trava:    concorrencia.NovaTrava(),
		caronas:  make(map[string]*dominio.Carona),
		assentos: make(map[string][]int),
		reservas: make(map[string]*dominio.Reserva),
		usuarios: make(map[string]*dominio.Usuario),
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
// cancela em cascata qualquer reserva que a referencie. Para os itens
// dessas reservas que apontam para OUTRAS caronas (itinerario
// combinado), o assento e devolvido antes de apagar a reserva — senao
// ele ficaria preso para sempre, ja que a reserva inteira esta sendo
// removida.

func (r *Repositorio) CancelarCarona(idCarona int, motorista string) error {
	chave := strconv.Itoa(idCarona)

	return r.trava.ComEscrita(func() error {
		// Agora precisamos pegar o ponteiro da carona para checar quem é o dono
		carona, existe := r.caronas[chave]
		if !existe {
			return fmt.Errorf("carona %d nao encontrada", idCarona)
		}

		// ==========================================
		// BARREIRA DE AUTORIZAÇÃO
		// ==========================================
		if carona.Motorista != motorista {
			return fmt.Errorf("acesso negado: voce nao tem permissao para cancelar a carona %d", idCarona)
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

// BuscarItinerarios monta o grafo de trechos disponiveis a partir do
// estado atual, devolve TODOS os itinerarios possiveis entre origem e
// destino (dominio.BuscarTodosItinerarios, DFS), ignora caronas cuja
// data ja passou, e ordena o resultado: menos trechos primeiro, e em
// caso de empate, menor preco total primeiro.

func (r *Repositorio) BuscarItinerarios(origem, destino dominio.Cidade) ([]dominio.Itinerario, error) {
	var itinerarios []dominio.Itinerario

	err := r.trava.ComLeitura(func() error {
		var disponiveis []dominio.ArestaDisponivel

		for chave, carona := range r.caronas {
			// Ignora caronas cuja data ja passou.
			if carona.Data.Before(time.Now()) {
				continue
			}

			trechos := dominio.TrechosDaRota(chave, carona.Rota)
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

// ConfirmarReserva e o metodo mais importante do arquivo: recebe um
// itinerario JA ESCOLHIDO (um ou mais trechos, possivelmente de caronas
// diferentes) e confirma TODOS os trechos ou NENHUM.
//
func (r *Repositorio) ConfirmarReserva(passageiro string, itinerario dominio.Itinerario) (dominio.Reserva, error) {
	var reserva dominio.Reserva

	if len(itinerario.Passos) == 0 {
		return reserva, fmt.Errorf("itinerario vazio")
	}

	err := r.trava.ComEscrita(func() error {
		// Passo 1: confere se TODOS os trechos ainda tem assento —
		// sem alterar nada ainda. Se qualquer um falhar, retorna erro
		// aqui e a funcao inteira sai sem ter mudado nenhum contador.
		for _, trecho := range itinerario.Passos {
			contadores, existe := r.assentos[trecho.IDCarona]
			if !existe || trecho.Indice >= len(contadores) {
				return fmt.Errorf("trecho invalido: carona %s indice %d", trecho.IDCarona, trecho.Indice)
			}
			if contadores[trecho.Indice] <= 0 {
				return fmt.Errorf("sem assento disponivel no trecho %s/%d", trecho.IDCarona, trecho.Indice)
			}
		}

		// Passo 2: so chega aqui se TODOS os trechos passaram na
		// checagem acima — agora decrementa todos.
		for _, trecho := range itinerario.Passos {
			r.assentos[trecho.IDCarona][trecho.Indice]--
		}

		r.proximoIDReserva++
		chave := strconv.Itoa(r.proximoIDReserva)

		itens := make([]dominio.ItemReserva, len(itinerario.Passos))
		for i, trecho := range itinerario.Passos {
			idCaronaInt, convErr := strconv.Atoi(trecho.IDCarona)
			if convErr != nil {
				return fmt.Errorf("id de carona invalido no trecho: %q", trecho.IDCarona)
			}
			itens[i] = dominio.ItemReserva{CaronaID: idCaronaInt, Trecho: trecho}
		}

		reserva = dominio.Reserva{
			Id:         r.proximoIDReserva,
			Passageiro: passageiro,
			Itens:      itens,
		}
		r.reservas[chave] = &reserva

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
// ocupava para os contadores correspondentes. Esquecer de devolver o
// assento seria um jeito facil de o servidor "vazar" capacidade real ao
// longo de uma execucao longa.
// CancelarReserva remove uma reserva E devolve os assentos que ela
// ocupava para os contadores correspondentes.
//
func (r *Repositorio) CancelarReserva(idReserva int, passageiro string) error {
	chave := strconv.Itoa(idReserva)

	return r.trava.ComEscrita(func() error {
		reserva, existe := r.reservas[chave]
		if !existe {
			return fmt.Errorf("reserva %d nao encontrada", idReserva)
		}

		// ==========================================
		// BARREIRA DE AUTORIZAÇÃO
		// ==========================================
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
