package consorcio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// Coordenador executa a coordenação de uma reserva. Cada instância do
// servidor pode coordenar as reservas dos passageiros conectados a ela;
// não existe um coordenador global ou permanente.
type Coordenador struct {
	Repositorio *estado.Repositorio
	Peers       []Peer
	HTTP        *http.Client
	porEmpresa  map[string]Peer

	lockMu sync.Mutex
	locks  map[string]*lockTransacao

	// O cursor evita que uma transacao antiga e permanentemente indisponivel
	// monopolize todos os ciclos quando o prazo global termina antes de visitar
	// os demais registros pendentes.
	recoveryMu     sync.Mutex
	recoveryCursor int
}

type lockTransacao struct {
	mu   sync.Mutex
	refs int
}

func NovoCoordenador(repo *estado.Repositorio, peers []Peer) *Coordenador {
	porEmpresa := make(map[string]Peer, len(peers))
	copia := append([]Peer(nil), peers...)
	for _, peer := range copia {
		porEmpresa[peer.EmpresaID] = peer
	}
	return &Coordenador{
		Repositorio: repo,
		Peers:       copia,
		HTTP:        &http.Client{Timeout: 4 * time.Second},
		porEmpresa:  porEmpresa,
		locks:       make(map[string]*lockTransacao),
	}
}

// Reservar agrupa os trechos por empresa e executa o protocolo preparar/decidir.
// A decisão é persistida antes de qualquer mensagem de confirmação ou aborto.
func (c *Coordenador) Reservar(
	ctx context.Context,
	passageiro string,
	itinerario dominio.Itinerario,
) (protocolo.CoordenarReservaResposta, error) {
	return c.reservar(ctx, passageiro, itinerario, "")
}

// ReservarComChave torna idempotente a solicitação iniciada pelo passageiro.
// O mesmo ID de requisição TCP deve ser reutilizado quando o cliente repetir
// uma chamada cujo resultado ficou desconhecido por falha de comunicação.
func (c *Coordenador) ReservarComChave(
	ctx context.Context,
	passageiro string,
	itinerario dominio.Itinerario,
	chaveIdempotencia string,
) (protocolo.CoordenarReservaResposta, error) {
	if strings.TrimSpace(chaveIdempotencia) == "" {
		return protocolo.CoordenarReservaResposta{}, fmt.Errorf("chave de idempotencia obrigatoria")
	}
	return c.reservar(ctx, passageiro, itinerario, chaveIdempotencia)
}

func (c *Coordenador) reservar(
	ctx context.Context,
	passageiro string,
	itinerario dominio.Itinerario,
	chaveIdempotencia string,
) (protocolo.CoordenarReservaResposta, error) {
	if c == nil || c.Repositorio == nil {
		return protocolo.CoordenarReservaResposta{}, fmt.Errorf("coordenador nao configurado")
	}
	passageiro = strings.TrimSpace(passageiro)
	if passageiro == "" {
		return protocolo.CoordenarReservaResposta{}, fmt.Errorf("passageiro nao pode ser vazio")
	}

	participantes, err := particionarItinerario(itinerario)
	if err != nil {
		return protocolo.CoordenarReservaResposta{}, err
	}
	empresaLocal := c.Repositorio.EmpresaID()
	for _, participante := range participantes {
		if participante.EmpresaID != empresaLocal {
			if _, existe := c.porEmpresa[participante.EmpresaID]; !existe {
				return protocolo.CoordenarReservaResposta{}, fmt.Errorf(
					"empresa %q nao esta configurada como peer",
					participante.EmpresaID,
				)
			}
		}
	}

	var tx estado.TransacaoCoordenada
	criada := false
	lockKey := ""
	if chaveIdempotencia != "" {
		lockKey = "chave:" + chaveIdempotencia
		unlock := c.lockTransacao(lockKey)
		defer unlock()
		tx, criada, err = c.Repositorio.CriarOuObterTransacaoCoordenada(
			chaveIdempotencia,
			passageiro,
			itinerario,
			participantes,
		)
	} else {
		id, gerarErr := c.Repositorio.GerarIDTransacao()
		if gerarErr != nil {
			return protocolo.CoordenarReservaResposta{}, fmt.Errorf("gerar ID da transacao: %w", gerarErr)
		}
		lockKey = "tx:" + id
		// O lock é adquirido antes de tornar o registro visível à rotina de
		// recuperação. Assim ela não pode abortar uma transação que ainda está
		// começando a preparar seus participantes.
		unlock := c.lockTransacao(lockKey)
		defer unlock()
		tx = estado.TransacaoCoordenada{
			ID:            id,
			Passageiro:    passageiro,
			Itinerario:    clonarItinerario(itinerario),
			Participantes: participantes,
			Estado:        estado.CoordenacaoPreparando,
		}
		if err = c.Repositorio.CriarTransacaoCoordenada(tx); err == nil {
			criada = true
		}
	}
	if err != nil {
		return protocolo.CoordenarReservaResposta{}, fmt.Errorf("registrar coordenacao: %w", err)
	}

	if !criada {
		// A chave ja existe. Nunca reiniciamos a preparacao: devolvemos o
		// resultado conhecido ou recuperamos a transacao interrompida.
		if err := c.recuperarUmaSemLock(ctx, tx.ID); err != nil {
			resposta := c.montarResposta(tx.ID)
			// Falha de entrega depois de uma decisão durável é um resultado
			// pendente, não uma nova operação nem uma troca de decisão.
			if resposta.Estado == "PENDENTE_CONFIRMAR" || resposta.Estado == "PENDENTE_ABORTAR" {
				return resposta, nil
			}
			return resposta, err
		}
		return c.montarResposta(tx.ID), nil
	}

	for _, participante := range participantes {
		if err := c.Repositorio.MarcarPreparacaoTentada(tx.ID, participante.EmpresaID); err != nil {
			return c.montarResposta(tx.ID), fmt.Errorf("registrar tentativa de preparacao: %w", err)
		}
		if err := c.prepararParticipante(ctx, tx.ID, passageiro, participante); err != nil {
			_ = c.Repositorio.RegistrarErroParticipante(tx.ID, participante.EmpresaID, err.Error())
			_ = c.Repositorio.RegistrarMotivoCoordenacao(tx.ID, err.Error())
			if decisaoErr := c.Repositorio.DefinirDecisaoCoordenada(tx.ID, estado.DecisaoAbortar); decisaoErr != nil {
				return c.montarResposta(tx.ID), fmt.Errorf("persistir decisao de aborto: %w", decisaoErr)
			}
			_ = c.distribuirDecisao(ctx, tx.ID)
			return c.montarResposta(tx.ID), nil
		}
		if err := c.Repositorio.RegistrarParticipantePreparado(tx.ID, participante.EmpresaID); err != nil {
			return c.montarResposta(tx.ID), fmt.Errorf("registrar preparacao de %s: %w", participante.EmpresaID, err)
		}
	}

	// A decisão é persistida antes de enviar qualquer confirmação.
	if err := c.Repositorio.DefinirDecisaoCoordenada(tx.ID, estado.DecisaoConfirmar); err != nil {
		return c.montarResposta(tx.ID), fmt.Errorf("persistir decisao de confirmacao: %w", err)
	}
	_ = c.distribuirDecisao(ctx, tx.ID)
	return c.montarResposta(tx.ID), nil
}

// lockTransacao serializa operações concorrentes sobre a mesma coordenação,
// sem impedir que esta empresa coordene reservas diferentes em paralelo.
func (c *Coordenador) lockTransacao(chave string) func() {
	c.lockMu.Lock()
	l := c.locks[chave]
	if l == nil {
		l = &lockTransacao{}
		c.locks[chave] = l
	}
	l.refs++
	c.lockMu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		c.lockMu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(c.locks, chave)
		}
		c.lockMu.Unlock()
	}
}

func chaveLockTransacao(tx estado.TransacaoCoordenada) string {
	if tx.ChaveIdempotencia != "" {
		return "chave:" + tx.ChaveIdempotencia
	}
	return "tx:" + tx.ID
}

// RetomarDecisao reenvia a decisão já persistida aos participantes sem ACK.
// É a entrada que a etapa 7 poderá executar na inicialização e periodicamente.
// Não inventa decisão quando o coordenador caiu ainda em PREPARANDO.
func (c *Coordenador) RetomarDecisao(
	ctx context.Context,
	id string,
) (protocolo.CoordenarReservaResposta, error) {
	tx, err := c.Repositorio.ConsultarTransacaoCoordenada(id)
	if err != nil {
		return protocolo.CoordenarReservaResposta{}, err
	}
	unlock := c.lockTransacao(chaveLockTransacao(tx))
	defer unlock()
	return c.retomarDecisaoSemLock(ctx, id)
}

func (c *Coordenador) retomarDecisaoSemLock(
	ctx context.Context,
	id string,
) (protocolo.CoordenarReservaResposta, error) {
	tx, err := c.Repositorio.ConsultarTransacaoCoordenada(id)
	if err != nil {
		return protocolo.CoordenarReservaResposta{}, err
	}
	if tx.Decisao == estado.DecisaoNenhuma {
		return c.montarResposta(id), fmt.Errorf("transacao %s ainda nao possui decisao duravel", id)
	}
	if err := c.distribuirDecisao(ctx, id); err != nil {
		// Propagar o erro permite que a rotina periódica de recuperação
		// registre a falha. A decisão já persistida permanece imutável.
		return c.montarResposta(id), err
	}
	return c.montarResposta(id), nil
}

// RecuperarPendentes retoma todas as coordenacoes incompletas persistidas.
// Uma transacao encontrada em PREPARANDO é abortada: pela regra do protocolo,
// nenhuma confirmação pode ter sido enviada antes de persistir CONFIRMAR.
func (c *Coordenador) RecuperarPendentes(ctx context.Context) error {
	if c == nil || c.Repositorio == nil {
		return fmt.Errorf("coordenador nao configurado")
	}
	c.recoveryMu.Lock()
	defer c.recoveryMu.Unlock()

	var falhas []error
	pendentes := c.Repositorio.ListarTransacoesCoordenadasPendentes()
	if len(pendentes) == 0 {
		c.recoveryCursor = 0
		return nil
	}
	inicio := c.recoveryCursor % len(pendentes)
	processadas := 0
	for deslocamento := 0; deslocamento < len(pendentes); deslocamento++ {
		if err := ctx.Err(); err != nil {
			falhas = append(falhas, err)
			break
		}
		indice := (inicio + deslocamento) % len(pendentes)
		pendente := pendentes[indice]
		unlock := c.lockTransacao(chaveLockTransacao(pendente))
		err := c.recuperarUmaSemLock(ctx, pendente.ID)
		unlock()
		processadas++
		if err != nil {
			falhas = append(falhas, fmt.Errorf("recuperar %s: %w", pendente.ID, err))
		}
	}
	// Na proxima rodada, continuar do proximo registro, inclusive quando um
	// peer indisponivel consumiu o prazo e impediu visitar toda a lista.
	c.recoveryCursor = (inicio + processadas) % len(pendentes)
	return errors.Join(falhas...)
}

func (c *Coordenador) recuperarUmaSemLock(ctx context.Context, id string) error {
	tx, err := c.Repositorio.ConsultarTransacaoCoordenada(id)
	if err != nil {
		return err
	}
	if tx.Estado == estado.CoordenacaoConcluida {
		return nil
	}
	if tx.Decisao == estado.DecisaoNenhuma {
		if tx.Estado != estado.CoordenacaoPreparando {
			return fmt.Errorf("estado %s sem decisao para transacao %s", tx.Estado, id)
		}
		motivo := "recuperacao: coordenacao reiniciada antes de persistir uma decisao; aborto escolhido com seguranca"
		if err := c.Repositorio.RegistrarMotivoCoordenacao(id, motivo); err != nil {
			return err
		}
		if err := c.Repositorio.DefinirDecisaoCoordenada(id, estado.DecisaoAbortar); err != nil {
			return err
		}
	}
	_, err = c.retomarDecisaoSemLock(ctx, id)
	return err
}

func (c *Coordenador) prepararParticipante(
	ctx context.Context,
	id, passageiro string,
	participante estado.ParticipanteCoordenacao,
) error {
	if participante.EmpresaID == c.Repositorio.EmpresaID() {
		_, err := c.Repositorio.PrepararTransacao(id, passageiro, participante.Itinerario)
		if err != nil {
			return err
		}
		return c.Repositorio.MarcarTransacaoCoordenada(id)
	}

	dados := protocolo.PrepararTransacaoDados{
		Passageiro: passageiro,
		Trechos:    trechosParaDados(participante.Itinerario),
	}
	var resposta protocolo.TransacaoParticipanteResposta
	if err := c.chamarPeer(ctx, participante.EmpresaID, id, "preparar", dados, &resposta); err != nil {
		return err
	}
	if resposta.TransacaoID != id || resposta.EmpresaID != participante.EmpresaID || resposta.Estado != string(estado.EstadoPreparada) {
		return fmt.Errorf("resposta de preparacao inconsistente da empresa %s", participante.EmpresaID)
	}
	return nil
}

// distribuirDecisao envia a decisão persistida e registra cada ACK recebido.
// Uma falha de rede não altera a decisão; a transação permanece pendente.
func (c *Coordenador) distribuirDecisao(ctx context.Context, id string) error {
	tx, err := c.Repositorio.ConsultarTransacaoCoordenada(id)
	if err != nil {
		return err
	}
	var falhas []error

	if tx.Decisao == estado.DecisaoConfirmar {
		for _, participante := range tx.Participantes {
			if participante.Confirmado {
				continue
			}
			if err := c.Repositorio.MarcarConfirmacaoTentada(id, participante.EmpresaID); err != nil {
				falhas = append(falhas, fmt.Errorf("marcar tentativa de confirmacao em %s: %w", participante.EmpresaID, err))
				continue
			}
			reservaID, err := c.confirmarParticipante(ctx, id, participante)
			if err != nil {
				_ = c.Repositorio.RegistrarErroParticipante(id, participante.EmpresaID, err.Error())
				falhas = append(falhas, fmt.Errorf("confirmacao pendente em %s: %w", participante.EmpresaID, err))
				continue
			}
			if err := c.Repositorio.RegistrarParticipanteConfirmado(id, participante.EmpresaID, reservaID); err != nil {
				falhas = append(falhas, fmt.Errorf("registrar ACK de confirmacao de %s: %w", participante.EmpresaID, err))
			}
		}
		return errors.Join(falhas...)
	}

	if tx.Decisao == estado.DecisaoAbortar {
		for _, participante := range tx.Participantes {
			if !participante.PreparacaoTentada || participante.Abortado {
				continue
			}
			if err := c.Repositorio.MarcarAbortoTentado(id, participante.EmpresaID); err != nil {
				falhas = append(falhas, fmt.Errorf("marcar tentativa de aborto em %s: %w", participante.EmpresaID, err))
				continue
			}
			if err := c.abortarParticipante(ctx, id, participante.EmpresaID); err != nil {
				_ = c.Repositorio.RegistrarErroParticipante(id, participante.EmpresaID, err.Error())
				falhas = append(falhas, fmt.Errorf("aborto pendente em %s: %w", participante.EmpresaID, err))
				continue
			}
			if err := c.Repositorio.RegistrarParticipanteAbortado(id, participante.EmpresaID); err != nil {
				falhas = append(falhas, fmt.Errorf("registrar ACK de aborto de %s: %w", participante.EmpresaID, err))
			}
		}
		return errors.Join(falhas...)
	}
	return fmt.Errorf("coordenacao %s ainda nao possui decisao", id)
}

func (c *Coordenador) confirmarParticipante(
	ctx context.Context,
	id string,
	participante estado.ParticipanteCoordenacao,
) (int, error) {
	if participante.EmpresaID == c.Repositorio.EmpresaID() {
		reserva, err := c.Repositorio.ConfirmarTransacao(id)
		if err != nil {
			return 0, err
		}
		return reserva.Id, nil
	}
	var resposta protocolo.TransacaoParticipanteResposta
	if err := c.chamarPeer(ctx, participante.EmpresaID, id, "confirmar", struct{}{}, &resposta); err != nil {
		return 0, err
	}
	if resposta.TransacaoID != id || resposta.EmpresaID != participante.EmpresaID || resposta.Estado != string(estado.EstadoConfirmada) || resposta.ReservaID <= 0 {
		return 0, fmt.Errorf("resposta de confirmacao inconsistente da empresa %s", participante.EmpresaID)
	}
	return resposta.ReservaID, nil
}

func (c *Coordenador) abortarParticipante(ctx context.Context, id, empresaID string) error {
	if empresaID == c.Repositorio.EmpresaID() {
		return c.Repositorio.AbortarTransacao(id)
	}
	var resposta protocolo.TransacaoParticipanteResposta
	if err := c.chamarPeer(ctx, empresaID, id, "abortar", struct{}{}, &resposta); err != nil {
		return err
	}
	if resposta.TransacaoID != id || resposta.EmpresaID != empresaID || resposta.Estado != string(estado.EstadoAbortada) {
		return fmt.Errorf("resposta de aborto inconsistente da empresa %s", empresaID)
	}
	return nil
}

func (c *Coordenador) chamarPeer(
	ctx context.Context,
	empresaID, id, acao string,
	dados any,
	resposta any,
) error {
	peer, existe := c.porEmpresa[empresaID]
	if !existe {
		return fmt.Errorf("peer %s nao configurado", empresaID)
	}
	base, err := url.Parse(strings.TrimRight(peer.URL, "/"))
	if err != nil {
		return fmt.Errorf("URL do peer %s invalida: %w", empresaID, err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/transacoes/" + url.PathEscape(id) + "/" + acao

	corpo, err := json.Marshal(dados)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(corpo))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VaiJunto-Empresa", c.Repositorio.EmpresaID())

	cliente := c.HTTP
	if cliente == nil {
		cliente = &http.Client{Timeout: 4 * time.Second}
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return fmt.Errorf("chamar empresa %s: %w", empresaID, err)
	}
	defer resp.Body.Close()

	conteudo, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("ler resposta de %s: %w", empresaID, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var erro protocolo.ErroREST
		_ = json.Unmarshal(conteudo, &erro)
		if erro.Erro == "" {
			erro.Erro = strings.TrimSpace(string(conteudo))
		}
		return fmt.Errorf("empresa %s respondeu HTTP %d: %s", empresaID, resp.StatusCode, erro.Erro)
	}
	if err := json.Unmarshal(conteudo, resposta); err != nil {
		return fmt.Errorf("JSON invalido recebido de %s: %w", empresaID, err)
	}
	return nil
}

func (c *Coordenador) montarResposta(id string) protocolo.CoordenarReservaResposta {
	tx, err := c.Repositorio.ConsultarTransacaoCoordenada(id)
	if err != nil {
		return protocolo.CoordenarReservaResposta{TransacaoID: id, Estado: "ERRO", Mensagem: err.Error()}
	}
	resposta := protocolo.CoordenarReservaResposta{
		TransacaoID: id,
		Decisao:     string(tx.Decisao),
		Mensagem:    tx.Motivo,
	}
	switch {
	case tx.Decisao == estado.DecisaoNenhuma:
		resposta.Estado = "PREPARANDO"
	case tx.Decisao == estado.DecisaoConfirmar && tx.Estado == estado.CoordenacaoConcluida:
		resposta.Estado = "CONFIRMADA"
	case tx.Decisao == estado.DecisaoConfirmar:
		resposta.Estado = "PENDENTE_CONFIRMAR"
	case tx.Decisao == estado.DecisaoAbortar && tx.Estado == estado.CoordenacaoConcluida:
		resposta.Estado = "ABORTADA"
	case tx.Decisao == estado.DecisaoAbortar:
		resposta.Estado = "PENDENTE_ABORTAR"
	}

	for _, participante := range tx.Participantes {
		pendente := false
		if tx.Decisao == estado.DecisaoConfirmar {
			pendente = !participante.Confirmado
		} else if tx.Decisao == estado.DecisaoAbortar {
			pendente = participante.PreparacaoTentada && !participante.Abortado
		}
		if pendente {
			resposta.EmpresasPendentes = append(resposta.EmpresasPendentes, participante.EmpresaID)
		}
	}
	sort.Strings(resposta.EmpresasPendentes)

	if resposta.Estado == "CONFIRMADA" {
		for _, participante := range tx.Participantes {
			reserva := protocolo.ReservaResposta{
				ID:          participante.ReservaID,
				Passageiro:  tx.Passageiro,
				EmpresaID:   participante.EmpresaID,
				TransacaoID: tx.ID,
				Trechos:     trechosParaDados(participante.Itinerario),
			}
			resposta.Reservas = append(resposta.Reservas, reserva)
		}
	}
	return resposta
}

func particionarItinerario(it dominio.Itinerario) ([]estado.ParticipanteCoordenacao, error) {
	if len(it.Passos) == 0 {
		return nil, fmt.Errorf("itinerario vazio")
	}
	porEmpresa := make(map[string]int)
	participantes := make([]estado.ParticipanteCoordenacao, 0)
	vistos := make(map[string]bool)
	for i, trecho := range it.Passos {
		if strings.TrimSpace(trecho.EmpresaID) == "" || strings.TrimSpace(trecho.IDCarona) == "" || strings.TrimSpace(string(trecho.Origem)) == "" || strings.TrimSpace(string(trecho.Destino)) == "" || trecho.Indice < 0 {
			return nil, fmt.Errorf("trecho %d do itinerario possui dados invalidos", i)
		}
		if i > 0 && it.Passos[i-1].Destino != trecho.Origem {
			return nil, fmt.Errorf("itinerario desconectado entre %q e %q", it.Passos[i-1].Destino, trecho.Origem)
		}
		chave := fmt.Sprintf("%s/%s/%d", trecho.EmpresaID, trecho.IDCarona, trecho.Indice)
		if vistos[chave] {
			return nil, fmt.Errorf("itinerario contem trecho duplicado: %s", chave)
		}
		vistos[chave] = true
		indice, existe := porEmpresa[trecho.EmpresaID]
		if !existe {
			indice = len(participantes)
			porEmpresa[trecho.EmpresaID] = indice
			participantes = append(participantes, estado.ParticipanteCoordenacao{
				EmpresaID:  trecho.EmpresaID,
				Itinerario: dominio.Itinerario{Passos: make([]dominio.Trecho, 0)},
			})
		}
		participantes[indice].Itinerario.Passos = append(participantes[indice].Itinerario.Passos, trecho)
	}
	return participantes, nil
}

func clonarItinerario(it dominio.Itinerario) dominio.Itinerario {
	return dominio.Itinerario{Passos: append([]dominio.Trecho(nil), it.Passos...)}
}

func trechosParaDados(it dominio.Itinerario) []protocolo.TrechoDados {
	trechos := make([]protocolo.TrechoDados, len(it.Passos))
	for i, trecho := range it.Passos {
		trechos[i] = protocolo.TrechoDados{
			EmpresaID: trecho.EmpresaID,
			IDCarona:  trecho.IDCarona,
			Indice:    trecho.Indice,
			Origem:    string(trecho.Origem),
			Destino:   string(trecho.Destino),
		}
	}
	return trechos
}
