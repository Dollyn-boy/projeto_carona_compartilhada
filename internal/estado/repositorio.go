// Package estado guarda os dados do VaiJunto e coordena as operacoes
// que os leem ou modificam. As regras de negocio ficam aqui proximas dos
// dados, para que seja facil acompanhar o que acontece com cada reserva.
package estado

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"vaijunto/internal/concorrencia"
	"vaijunto/internal/dominio"
)

// Repositorio contem o estado em memoria. mu protege os mapas e os IDs;
// trava impede que duas reservas disputem o mesmo trecho ao mesmo tempo.
// caminhoJSON vazio significa que o repositorio e apenas em memoria (util
// nos testes que nao precisam persistir dados).
type Repositorio struct {
	empresaID string
	// Protege os mapas e os identificadores do repositório contra acessos conflitantes.
	mu sync.RWMutex

	// Gerencia travas específicas para os trechos das caronas.
	trava *concorrencia.Trava

	caminhoJSON string

	caronas  map[string]*dominio.Carona
	assentos map[string][]int // Guarda quantos assentos ainda estão livres em cada trecho de cada carona.
	reservas map[string]*dominio.Reserva
	usuarios map[string]*dominio.Usuario

	proximoIDCarona  int
	proximoIDReserva int
}

func NovoRepositorio() *Repositorio {
	return novoRepositorio("local", "")
}

// Mantem compatibilidade com o codigo existente e com os testes.
// O identificador "local" e usado quando nenhuma empresa e especificada.
func NovoRepositorioComArquivo(caminho string) (*Repositorio, error) {
	return NovoRepositorioDeEmpresaComArquivo("local", caminho)
}

// NovoRepositorioDeEmpresa cria um repositorio em memoria
// vinculado a uma empresa especifica.
func NovoRepositorioDeEmpresa(empresaID string) (*Repositorio, error) {
	empresaID = strings.TrimSpace(empresaID)
	if empresaID == "" {
		return nil, fmt.Errorf("identificador da empresa nao pode ser vazio")
	}

	return novoRepositorio(empresaID, ""), nil
}

// NovoRepositorioDeEmpresaComArquivo cria o repositorio e carrega
// os dados persistidos daquela empresa, caso o arquivo exista.
func NovoRepositorioDeEmpresaComArquivo(
	empresaID string,
	caminho string,
) (*Repositorio, error) {
	empresaID = strings.TrimSpace(empresaID)
	caminho = strings.TrimSpace(caminho)

	if empresaID == "" {
		return nil, fmt.Errorf("identificador da empresa nao pode ser vazio")
	}
	if caminho == "" {
		return nil, fmt.Errorf("caminho do arquivo JSON nao pode ser vazio")
	}

	r := novoRepositorio(empresaID, caminho)

	conteudo, err := os.ReadFile(caminho)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ler arquivo de dados %q: %w", caminho, err)
	}

	if err := r.aplicarJSON(conteudo); err != nil {
		return nil, fmt.Errorf("carregar arquivo de dados %q: %w", caminho, err)
	}

	return r, nil
}

func novoRepositorio(empresaID, caminho string) *Repositorio {
	return &Repositorio{
		empresaID:   empresaID,
		trava:       concorrencia.NovaTrava(),
		caminhoJSON: caminho,
		caronas:     make(map[string]*dominio.Carona),
		assentos:    make(map[string][]int),
		reservas:    make(map[string]*dominio.Reserva),
		usuarios:    make(map[string]*dominio.Usuario),
	}
}

// estadoPersistido e o formato completo do arquivo JSON. Guardar um snapshot
// inteiro torna a persistencia facil de entender: cada salvamento descreve
// todo o estado atual, em vez de manter uma sequencia de eventos para repetir.
type estadoPersistido struct {
	EmpresaID        string            `json:"empresa_id"`
	Versao           int               `json:"versao"`
	ProximoIDCarona  int               `json:"proximo_id_carona"`
	ProximoIDReserva int               `json:"proximo_id_reserva"`
	Caronas          []dominio.Carona  `json:"caronas"`
	Assentos         map[string][]int  `json:"assentos_livres_por_trecho"`
	Reservas         []dominio.Reserva `json:"reservas"`
	Usuarios         []dominio.Usuario `json:"usuarios"`
}

// alterar executa uma modificacao protegida pelo mutex do repositorio.
// Quando ha arquivo configurado, cria uma copia do estado anterior. Se o
// salvamento falhar, restaura essa copia para memoria e disco nao divergirem.

// Atenção: As funções passadas a alterar precisam evitar
//
//	modificações parciais antes de retornar um erro,
//	ou a própria implementação de alterar precisa ser
//	fortalecida para cobrir esse cenário.
func (r *Repositorio) alterar(fn func() error) error {
	// Adquirir mutex global para evitar alterações simultâneas no estado protegido.
	r.mu.Lock()
	defer r.mu.Unlock()

	// Serializar o estado anterior para bytes JSON
	var backup []byte
	if r.caminhoJSON != "" {
		var err error
		// Salvar o estado do repositório antes de qualquer modificação.
		backup, err = r.serializarLocked()
		if err != nil {
			return fmt.Errorf("preparar estado anterior: %w", err)
		}
	}

	// Executa a funcao de alteracao. Se der erro, devolve o erro sem alterar nada.
	// Não se deve incluir chamadas para outros métodos em fn() para evitar deadlocks
	if err := fn(); err != nil {
		return err
	}

	// Se não existe caminho configurado, a alteração já foi feita em memória,
	//  então alterar retorna sem gravar um arquivo.
	if r.caminhoJSON == "" {
		return nil
	}

	// Se houver caminhoJSON, tenta salvar o estado atual. Se falhar, restaura o backup.
	if err := r.salvarLocked(); err != nil {
		// Essa é a função de recuperação/rollback
		if erroRestaurar := r.aplicarJSONLocked(backup); erroRestaurar != nil {
			return fmt.Errorf("erro ao salvar (%v) e erro ao restaurar estado anterior: %w", err, erroRestaurar)
		}
		return fmt.Errorf("nao foi possivel persistir a alteracao: %w", err)
	}
	return nil
}

// PublicarCarona cria uma carona e inicializa um contador de assentos para
// cada trecho adjacente da rota. Ex.: A-B-C cria dois contadores.
func (r *Repositorio) PublicarCarona(motorista string, rota []dominio.Cidade, capacidade int, preco int, data time.Time) (dominio.Carona, error) {
	if len(rota) < 2 {
		return dominio.Carona{}, fmt.Errorf("rota precisa de pelo menos 2 cidades, veio %d", len(rota))
	}
	if capacidade <= 0 {
		return dominio.Carona{}, fmt.Errorf("capacidade precisa ser maior que zero")
	}
	if data.Before(time.Now()) {
		return dominio.Carona{}, fmt.Errorf("data da carona nao pode estar no passado")
	}

	// A carona e criada dentro de r.alterar para garantir que o estado seja consistente e persistido corretamente.
	var criada dominio.Carona

	// r.alterar garante que a operacao seja atomica e que o estado seja salvo corretamente.
	// se o salvamento falhar, a carona nao sera criada e o estado sera restaurado.
	// A copia da carona e armazenada no mapa para evitar que referencias externas modifiquem o estado interno do repositorio.
	err := r.alterar(func() error {
		// Closure que cria a carona e inicializa os contadores de assentos livres para cada trecho da rota.
		id := r.proximoIDCarona + 1
		criada = dominio.Carona{
			Id:         id,
			Motorista:  motorista,
			Rota:       append([]dominio.Cidade(nil), rota...),
			Preco:      preco,
			Capacidade: capacidade,
			Data:       data,
		}
		chave := strconv.Itoa(id)
		livres := make([]int, len(rota)-1)
		for i := range livres {
			livres[i] = capacidade
		}

		r.proximoIDCarona = id
		// Copia da carona e armazenada no mapa para evitar que referencias externas modifiquem o estado interno do repositorio.
		copiaCarona := clonarCarona(criada)
		r.caronas[chave] = &copiaCarona
		r.assentos[chave] = livres
		return nil
	})
	return clonarCarona(criada), err
}

type CaronaComOcupacao struct {
	Carona               dominio.Carona
	PassageirosPorTrecho [][]string
}

// ConsultarCaronas lista as caronas de um motorista e os passageiros
// confirmados em cada trecho elementar.
func (r *Repositorio) ConsultarCaronas(motorista string) ([]CaronaComOcupacao, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resultado := make([]CaronaComOcupacao, 0)
	for chave, carona := range r.caronas {
		if carona.Motorista != motorista {
			continue
		}
		porTrecho := make([][]string, len(carona.Rota)-1)
		for _, reserva := range r.reservas {
			for _, item := range reserva.Itens {
				if strconv.Itoa(item.CaronaID) == chave && item.Trecho.Indice >= 0 && item.Trecho.Indice < len(porTrecho) {
					porTrecho[item.Trecho.Indice] = append(porTrecho[item.Trecho.Indice], reserva.Passageiro)
				}
			}
		}
		for i := range porTrecho {
			sort.Strings(porTrecho[i])
		}
		resultado = append(resultado, CaronaComOcupacao{Carona: clonarCarona(*carona), PassageirosPorTrecho: porTrecho})
	}
	sort.Slice(resultado, func(i, j int) bool { return resultado[i].Carona.Id < resultado[j].Carona.Id })
	return resultado, nil
}

// CancelarCarona cancela a carona e remove as reservas que a utilizavam.
// Se uma reserva tambem ocupava trechos de outras caronas, esses assentos
// sao devolvidos. Cancelamento de carona e raro, por isso adquire as travas
// dos trechos existentes antes de atualizar os dados relacionados.
func (r *Repositorio) CancelarCarona(idCarona int, motorista string) error {
	chave := strconv.Itoa(idCarona)
	chaves := r.chavesTodosTrechos()
	return r.trava.ComTrechos(chaves, func() error {
		return r.alterar(func() error {
			carona, existe := r.caronas[chave]
			if !existe {
				return fmt.Errorf("carona %d nao encontrada", idCarona)
			}
			if carona.Motorista != motorista {
				return fmt.Errorf("acesso negado: voce nao tem permissao para cancelar a carona %d", idCarona)
			}

			for chaveReserva, reserva := range r.reservas {
				usaCarona := false
				for _, item := range reserva.Itens {
					if item.CaronaID == idCarona {
						usaCarona = true
						break
					}
				}
				if !usaCarona {
					continue
				}

				for _, item := range reserva.Itens {
					if item.CaronaID == idCarona {
						continue // os contadores desta carona serao removidos abaixo
					}
					chaveOutra := strconv.Itoa(item.CaronaID)
					if livres, ok := r.assentos[chaveOutra]; ok && item.Trecho.Indice >= 0 && item.Trecho.Indice < len(livres) {
						livres[item.Trecho.Indice]++
					}
				}
				delete(r.reservas, chaveReserva)
			}

			delete(r.caronas, chave)
			delete(r.assentos, chave)
			return nil
		})
	})
}

// VerificarReserva verifica se todos os trechos de uma reserva ainda existem.
func (r *Repositorio) VerificarReserva(idReserva int) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reserva, existe := r.reservas[strconv.Itoa(idReserva)]
	if !existe {
		return false, fmt.Errorf("reserva %d nao encontrada", idReserva)
	}
	for _, item := range reserva.Itens {
		livres, ok := r.assentos[strconv.Itoa(item.CaronaID)]
		if !ok || item.Trecho.Indice < 0 || item.Trecho.Indice >= len(livres) {
			return false, nil
		}
	}
	return true, nil
}

// BuscarItinerarios monta um grafo apenas com os trechos que ainda possuem
// assento. O mutex de leitura protege o snapshot enquanto a busca e ordenacao
// acontecem; a busca em si nao modifica o estado.
func (r *Repositorio) BuscarItinerarios(origem, destino dominio.Cidade) ([]dominio.Itinerario, error) {
	// O mutex de leitura protege o snapshot enquanto a busca e ordenacao acontecem; a busca em si nao modifica o estado.
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Construir uma lista de arestas disponiveis (trechos com assentos livres) para montar o grafo.
	disponiveis := make([]dominio.ArestaDisponivel, 0)

	// Iterar sobre todas as caronas e seus trechos para identificar quais ainda possuem assentos livres.
	// Para construir o grafo, precisamos de uma lista de trechos disponíveis. Para obter essa lista,
	// iteramos sobre todas as caronas do repositorio e verificamos os assentos livres em cada trecho.
	for chave, carona := range r.caronas {
		if carona.Data.Before(time.Now()) {
			continue
		}
		for _, trecho := range dominio.TrechosDaRota(r.empresaID, chave, carona.Rota) {
			livres := r.assentos[chave][trecho.Indice]
			if livres > 0 {
				// Se houver assentos livres, adiciona a aresta ao grafo de itinerários disponíveis.
				disponiveis = append(disponiveis, dominio.ArestaDisponivel{
					Trecho: trecho, AssentosDisponiveis: livres, Preco: float64(carona.Preco),
				})
			}
		}
	}

	// Montar o grafo e buscar todos os itinerários possíveis entre a origem e o destino.
	itinerarios := dominio.BuscarTodosItinerarios(dominio.ConstruirGrafo(disponiveis), origem, destino)

	// Ordenar os itinerários encontrados: primeiro pelo número de trechos, depois pelo preço total.
	// Isso garante que o cliente receba os itinerários mais curtos e baratos primeiro.
	sort.Slice(itinerarios, func(i, j int) bool {
		if len(itinerarios[i].Passos) != len(itinerarios[j].Passos) {
			return len(itinerarios[i].Passos) < len(itinerarios[j].Passos)
		}
		return r.precoTotalItinerarioLocked(itinerarios[i]) < r.precoTotalItinerarioLocked(itinerarios[j])
	})
	return itinerarios, nil
}

func (r *Repositorio) precoTotalItinerarioLocked(it dominio.Itinerario) int {
	total := 0
	for _, passo := range it.Passos {
		if carona, ok := r.caronas[passo.IDCarona]; ok {
			total += carona.Preco
		}
	}
	return total
}

// ConfirmarReserva reserva todos os trechos do itinerario ou nenhum deles.
// Primeiro adquire travas por trecho em ordem fixa; depois valida toda a rota
// e aplica a mudanca como uma unica operacao do repositorio.
func (r *Repositorio) ConfirmarReserva(passageiro string, itinerario dominio.Itinerario) (dominio.Reserva, error) {
	// 1. Valida se o itinerario nao esta vazio
	var criada dominio.Reserva
	if len(itinerario.Passos) == 0 {
		return criada, fmt.Errorf("itinerario vazio")
	}

	// 2. Cria uma copia dos trechos do itinerario para evitar modificacoes externas durante a operacao.
	passos := append([]dominio.Trecho(nil), itinerario.Passos...)
	chaves := make([]string, len(passos)) // armazenará as chaves das travas.
	idsCarona := make([]int, len(passos)) //armazenará os IDs numéricos das caronas.
	vistos := make(map[string]bool)       // para verificar se o itinerário contém trechos duplicados.

	// 3. Valida cada trecho: índice válido, ID de carona válido e sem duplicatas.
	for i, trecho := range passos {
		if trecho.EmpresaID != r.empresaID {
			return criada, fmt.Errorf(
				"trecho da empresa %q nao pode ser reservado pelo servidor da empresa %q",
				trecho.EmpresaID,
				r.empresaID,
			)
		}
		if trecho.Indice < 0 {
			return criada, fmt.Errorf("indice de trecho invalido: %d", trecho.Indice)
		}
		id, err := strconv.Atoi(trecho.IDCarona)
		if err != nil || id <= 0 {
			return criada, fmt.Errorf("id de carona invalido no trecho: %q", trecho.IDCarona)
		}

		// 4. Prepara as chaves para travas e verifica se o mesmo trecho aparece mais de uma vez no itinerário.
		idsCarona[i] = id
		chaves[i] = concorrencia.ChaveTrecho(trecho.IDCarona, trecho.Indice)
		if vistos[chaves[i]] {
			return criada, fmt.Errorf("itinerario contem o mesmo trecho mais de uma vez")
		}
		vistos[chaves[i]] = true
	}

	// 5. Adquire travas para todos os trechos do itinerário em ordem fixa e executa a operação de reserva.
	// Se a operação falhar, nenhuma alteração será feita e o estado do repositório permanecerá consistente.
	err := r.trava.ComTrechos(chaves, func() error {
		return r.alterar(func() error {
			// Validacao completa antes de alterar qualquer contador.
			for _, trecho := range passos {
				chave := trecho.IDCarona
				if _, existe := r.caronas[chave]; !existe {
					return fmt.Errorf("carona %s nao encontrada", chave)
				}
				livres, existe := r.assentos[chave]
				if !existe || trecho.Indice >= len(livres) {
					return fmt.Errorf("trecho invalido: carona %s indice %d", chave, trecho.Indice)
				}
				if livres[trecho.Indice] <= 0 {
					return fmt.Errorf("sem assento disponivel no trecho %s/%d", chave, trecho.Indice)
				}
			}

			// Se todas as validacoes passaram, cria a reserva e decrementa os contadores de assentos livres.
			itens := make([]dominio.ItemReserva, len(passos))
			for i, trecho := range passos {
				itens[i] = dominio.ItemReserva{CaronaID: idsCarona[i], Trecho: trecho}
			}
			id := r.proximoIDReserva + 1
			criada = dominio.Reserva{Id: id, Passageiro: passageiro, Itens: itens}

			for _, trecho := range passos {
				r.assentos[trecho.IDCarona][trecho.Indice]--
			}
			r.proximoIDReserva = id
			copiaReserva := clonarReserva(criada)
			r.reservas[strconv.Itoa(id)] = &copiaReserva
			return nil
		})
	})
	return clonarReserva(criada), err
}

// ConsultarReservas devolve as reservas ativas de um passageiro.
func (r *Repositorio) ConsultarReservas(passageiro string) ([]dominio.Reserva, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resultado := make([]dominio.Reserva, 0)
	for _, reserva := range r.reservas {
		if reserva.Passageiro == passageiro {
			resultado = append(resultado, clonarReserva(*reserva))
		}
	}
	sort.Slice(resultado, func(i, j int) bool { return resultado[i].Id < resultado[j].Id })
	return resultado, nil
}

// CancelarReserva remove a reserva e devolve os assentos de seus trechos.
func (r *Repositorio) CancelarReserva(idReserva int, passageiro string) error {
	chaveReserva := strconv.Itoa(idReserva)

	// Descobre os trechos sem manter o mutex geral enquanto espera pelas travas.
	r.mu.RLock()
	reservaInicial, existe := r.reservas[chaveReserva]
	if !existe {
		r.mu.RUnlock()
		return fmt.Errorf("reserva %d nao encontrada", idReserva)
	}
	if reservaInicial.Passageiro != passageiro {
		r.mu.RUnlock()
		return fmt.Errorf("acesso negado: voce nao tem permissao para cancelar a reserva %d", idReserva)
	}
	itens := append([]dominio.ItemReserva(nil), reservaInicial.Itens...)
	r.mu.RUnlock()

	chaves := make([]string, len(itens))
	for i, item := range itens {
		chaves[i] = concorrencia.ChaveTrecho(strconv.Itoa(item.CaronaID), item.Trecho.Indice)
	}
	return r.trava.ComTrechos(chaves, func() error {
		return r.alterar(func() error {
			reserva, existe := r.reservas[chaveReserva]
			if !existe {
				return fmt.Errorf("reserva %d nao encontrada", idReserva)
			}
			if reserva.Passageiro != passageiro {
				return fmt.Errorf("acesso negado: voce nao tem permissao para cancelar a reserva %d", idReserva)
			}
			for _, item := range reserva.Itens {
				chave := strconv.Itoa(item.CaronaID)
				if livres, ok := r.assentos[chave]; ok && item.Trecho.Indice >= 0 && item.Trecho.Indice < len(livres) {
					livres[item.Trecho.Indice]++
				}
			}
			delete(r.reservas, chaveReserva)
			return nil
		})
	})
}

// CadastrarUsuario persiste somente o hash da senha, nunca a senha original.
func (r *Repositorio) CadastrarUsuario(usuario string, senha string, role string) error {
	return r.alterar(func() error {
		if _, existe := r.usuarios[usuario]; existe {
			return fmt.Errorf("usuario %s ja cadastrado", usuario)
		}
		r.usuarios[usuario] = &dominio.Usuario{Usuario: usuario, SenhaHash: dominio.HashSenha(senha), Role: role}
		return nil
	})
}

// AutenticarUsuario verifica a senha e devolve o papel do usuario.
func (r *Repositorio) AutenticarUsuario(usuario string, senha string) (bool, string, error) {
	// Acesso de leitura, pois nao altera o estado do repositorio.
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Verifica se o usuario existe e se a senha bate com o hash armazenado.
	usuarioObj, existe := r.usuarios[usuario]
	if !existe {
		return false, "", fmt.Errorf("usuario %s nao encontrado", usuario)
	}
	if usuarioObj.SenhaHash != dominio.HashSenha(senha) {
		return false, "", fmt.Errorf("senha incorreta para o usuario %s", usuario)
	}
	return true, usuarioObj.Role, nil
}

// VerificarUsuario e mantido para chamadas que so precisam do booleano.
func (r *Repositorio) VerificarUsuario(usuario string, senha string) bool {
	autenticado, _, err := r.AutenticarUsuario(usuario, senha)
	return err == nil && autenticado
}

func (r *Repositorio) chavesTodosTrechos() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	chaves := make([]string, 0)
	for id, carona := range r.caronas {
		for indice := 0; indice < len(carona.Rota)-1; indice++ {
			chaves = append(chaves, concorrencia.ChaveTrecho(id, indice))
		}
	}
	return chaves
}

func clonarCarona(c dominio.Carona) dominio.Carona {
	c.Rota = append([]dominio.Cidade(nil), c.Rota...)
	return c
}

func clonarReserva(r dominio.Reserva) dominio.Reserva {
	r.Itens = append([]dominio.ItemReserva(nil), r.Itens...)
	return r
}

// estadoLocked cria uma copia do estado atual, pronta para serializar em JSON.
func (r *Repositorio) estadoLocked() estadoPersistido {
	e := estadoPersistido{
		EmpresaID:        r.empresaID,
		Versao:           1,
		ProximoIDCarona:  r.proximoIDCarona,
		ProximoIDReserva: r.proximoIDReserva,
		Assentos:         make(map[string][]int, len(r.assentos)),
		Caronas:          make([]dominio.Carona, 0, len(r.caronas)),
		Reservas:         make([]dominio.Reserva, 0, len(r.reservas)),
		Usuarios:         make([]dominio.Usuario, 0, len(r.usuarios)),
	}
	for chave, carona := range r.caronas {
		e.Caronas = append(e.Caronas, clonarCarona(*carona))
		if livres, ok := r.assentos[chave]; ok {
			e.Assentos[chave] = append([]int(nil), livres...)
		}
	}
	for _, reserva := range r.reservas {
		e.Reservas = append(e.Reservas, clonarReserva(*reserva))
	}
	for _, usuario := range r.usuarios {
		e.Usuarios = append(e.Usuarios, *usuario)
	}

	sort.Slice(e.Caronas, func(i, j int) bool { return e.Caronas[i].Id < e.Caronas[j].Id })
	sort.Slice(e.Reservas, func(i, j int) bool { return e.Reservas[i].Id < e.Reservas[j].Id })
	sort.Slice(e.Usuarios, func(i, j int) bool { return e.Usuarios[i].Usuario < e.Usuarios[j].Usuario })
	return e
}

// serializarLocked converte o estado atual em JSON indentado, pronto para salvar em arquivo.
func (r *Repositorio) serializarLocked() ([]byte, error) {
	return json.MarshalIndent(r.estadoLocked(), "", "  ")
}

// salvarLocked grava o estado atual em arquivo JSON, criando o diretório se necessario e usando um arquivo temporario para evitar corrupcao.
func (r *Repositorio) salvarLocked() error {
	conteudo, err := r.serializarLocked()
	if err != nil {
		return err
	}
	diretorio := filepath.Dir(r.caminhoJSON)
	if err := os.MkdirAll(diretorio, 0755); err != nil {
		return err
	}
	arquivoTemp, err := os.CreateTemp(diretorio, ".dados-*.tmp")
	if err != nil {
		return err
	}
	nomeTemp := arquivoTemp.Name()
	defer os.Remove(nomeTemp)

	if _, err := arquivoTemp.Write(conteudo); err != nil {
		_ = arquivoTemp.Close()
		return err
	}
	if err := arquivoTemp.Sync(); err != nil {
		_ = arquivoTemp.Close()
		return err
	}
	if err := arquivoTemp.Close(); err != nil {
		return err
	}
	// O arquivo temporario fica no mesmo diretorio, entao a troca evita
	// deixar um JSON parcialmente escrito caso o processo seja interrompido.
	if err := os.Rename(nomeTemp, r.caminhoJSON); err != nil {
		return err
	}
	return nil
}

func (r *Repositorio) aplicarJSON(conteudo []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.aplicarJSONLocked(conteudo)
}

func (r *Repositorio) aplicarJSONLocked(conteudo []byte) error {
	var e estadoPersistido
	if err := json.Unmarshal(conteudo, &e); err != nil {
		return err
	}
	if e.Versao != 1 {
		return fmt.Errorf("versao do arquivo JSON nao suportada: %d", e.Versao)
	}

	if e.EmpresaID != "" && e.EmpresaID != r.empresaID {
		return fmt.Errorf(
			"arquivo pertence a empresa %q, mas o servidor foi configurado como %q",
			e.EmpresaID,
			r.empresaID,
		)
	}

	caronas := make(map[string]*dominio.Carona, len(e.Caronas))
	for _, carona := range e.Caronas {
		chave := strconv.Itoa(carona.Id)
		if carona.Id <= 0 || len(carona.Rota) < 2 {
			return fmt.Errorf("carona invalida no arquivo: id %d", carona.Id)
		}
		if _, repetida := caronas[chave]; repetida {
			return fmt.Errorf("id de carona duplicado no arquivo: %d", carona.Id)
		}
		copia := clonarCarona(carona)
		caronas[chave] = &copia
	}
	assentos := make(map[string][]int, len(e.Assentos))
	for chave, livres := range e.Assentos {
		if _, existe := caronas[chave]; !existe {
			return fmt.Errorf("contadores apontam para carona inexistente: %s", chave)
		}
		if len(livres) != len(caronas[chave].Rota)-1 {
			return fmt.Errorf("quantidade de contadores invalida para carona %s", chave)
		}
		assentos[chave] = append([]int(nil), livres...)
	}
	for chave := range caronas {
		if _, existe := assentos[chave]; !existe {
			return fmt.Errorf("faltam contadores para carona %s", chave)
		}
	}

	reservas := make(map[string]*dominio.Reserva, len(e.Reservas))
	for _, reserva := range e.Reservas {
		chave := strconv.Itoa(reserva.Id)
		if reserva.Id <= 0 {
			return fmt.Errorf("id de reserva invalido: %d", reserva.Id)
		}
		if _, repetida := reservas[chave]; repetida {
			return fmt.Errorf("id de reserva duplicado no arquivo: %d", reserva.Id)
		}
		copia := clonarReserva(reserva)
		reservas[chave] = &copia
	}
	usuarios := make(map[string]*dominio.Usuario, len(e.Usuarios))
	for _, usuario := range e.Usuarios {
		if usuario.Usuario == "" {
			return fmt.Errorf("usuario vazio no arquivo")
		}
		if _, repetido := usuarios[usuario.Usuario]; repetido {
			return fmt.Errorf("usuario duplicado no arquivo: %s", usuario.Usuario)
		}
		copia := usuario
		usuarios[usuario.Usuario] = &copia
	}

	r.caronas = caronas
	r.assentos = assentos
	r.reservas = reservas
	r.usuarios = usuarios
	r.proximoIDCarona = e.ProximoIDCarona
	r.proximoIDReserva = e.ProximoIDReserva
	// Tolera um contador antigo menor que os IDs encontrados, evitando
	// reutilizar identificadores caso o arquivo tenha sido editado manualmente.
	for _, carona := range e.Caronas {
		if carona.Id > r.proximoIDCarona {
			r.proximoIDCarona = carona.Id
		}
	}
	for _, reserva := range e.Reservas {
		if reserva.Id > r.proximoIDReserva {
			r.proximoIDReserva = reserva.Id
		}
	}
	return nil
}
