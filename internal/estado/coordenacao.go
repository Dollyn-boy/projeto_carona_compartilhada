package estado

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"vaijunto/internal/dominio"
)

// DecisaoCoordenacao e a decisão durável tomada pela empresa coordenadora.
type DecisaoCoordenacao string

const (
	DecisaoNenhuma   DecisaoCoordenacao = ""
	DecisaoConfirmar DecisaoCoordenacao = "CONFIRMAR"
	DecisaoAbortar   DecisaoCoordenacao = "ABORTAR"
)

// EstadoCoordenacao distingue a fase de preparação, a entrega da decisão
// e a conclusão após todos os participantes reconhecerem a decisão.
type EstadoCoordenacao string

const (
	CoordenacaoPreparando EstadoCoordenacao = "PREPARANDO"
	CoordenacaoPendente   EstadoCoordenacao = "DECISAO_PENDENTE"
	CoordenacaoConcluida  EstadoCoordenacao = "CONCLUIDA"
)

// ParticipanteCoordenacao guarda a parte local do itinerário e os ACKs.
type ParticipanteCoordenacao struct {
	EmpresaID          string             `json:"empresa_id"`
	Itinerario         dominio.Itinerario `json:"itinerario"`
	PreparacaoTentada  bool               `json:"preparacao_tentada"`
	Preparado          bool               `json:"preparado"`
	ConfirmacaoTentada bool               `json:"confirmacao_tentada"`
	Confirmado         bool               `json:"confirmado"`
	AbortoTentado      bool               `json:"aborto_tentado"`
	Abortado           bool               `json:"abortado"`
	ReservaID          int                `json:"reserva_id,omitempty"`
	UltimoErro         string             `json:"ultimo_erro,omitempty"`
}

// TransacaoCoordenada é o log durável da empresa que coordena UMA reserva.
// A empresa coordenadora pode mudar de uma transação para a próxima.
type TransacaoCoordenada struct {
	ID                string                    `json:"id"`
	ChaveIdempotencia string                    `json:"chave_idempotencia,omitempty"`
	Passageiro        string                    `json:"passageiro"`
	Itinerario        dominio.Itinerario        `json:"itinerario"`
	Participantes     []ParticipanteCoordenacao `json:"participantes"`
	Estado            EstadoCoordenacao         `json:"estado"`
	Decisao           DecisaoCoordenacao        `json:"decisao"`
	Motivo            string                    `json:"motivo,omitempty"`
	CriadaEm          time.Time                 `json:"criada_em"`
	AtualizadaEm      time.Time                 `json:"atualizada_em"`
}

var ErrIdempotenciaConflitante = errors.New("chave de idempotencia reutilizada com outros dados")

// CriarOuObterTransacaoCoordenada cria atomicamente o registro inicial associado
// a uma chave de idempotencia. Repetir a mesma chave com os mesmos dados devolve
// o registro existente; reutiliza-la para outra solicitacao e rejeitado.
func (r *Repositorio) CriarOuObterTransacaoCoordenada(
	chave, passageiro string,
	itinerario dominio.Itinerario,
	participantes []ParticipanteCoordenacao,
) (TransacaoCoordenada, bool, error) {
	chave = strings.TrimSpace(chave)
	passageiro = strings.TrimSpace(passageiro)
	if chave == "" || passageiro == "" {
		return TransacaoCoordenada{}, false, fmt.Errorf("chave de idempotencia e passageiro sao obrigatorios")
	}
	tx := TransacaoCoordenada{
		ChaveIdempotencia: chave,
		Passageiro:        passageiro,
		Itinerario:        clonarItinerario(itinerario),
		Participantes:     append([]ParticipanteCoordenacao(nil), participantes...),
		Estado:            CoordenacaoPreparando,
		Decisao:           DecisaoNenhuma,
		CriadaEm:          time.Now().UTC(),
	}
	tx = clonarTransacaoCoordenada(tx)
	if len(tx.Itinerario.Passos) == 0 || len(tx.Participantes) == 0 {
		return TransacaoCoordenada{}, false, fmt.Errorf("transacao coordenada precisa de itinerario e participantes")
	}
	vistos := make(map[string]bool, len(tx.Participantes))
	for _, participante := range tx.Participantes {
		if strings.TrimSpace(participante.EmpresaID) == "" || len(participante.Itinerario.Passos) == 0 {
			return TransacaoCoordenada{}, false, fmt.Errorf("participante sem empresa ou itinerario")
		}
		if vistos[participante.EmpresaID] {
			return TransacaoCoordenada{}, false, fmt.Errorf("participante duplicado: %s", participante.EmpresaID)
		}
		vistos[participante.EmpresaID] = true
		for _, trecho := range participante.Itinerario.Passos {
			if trecho.EmpresaID != participante.EmpresaID {
				return TransacaoCoordenada{}, false, fmt.Errorf("participante %s contem trecho da empresa %s", participante.EmpresaID, trecho.EmpresaID)
			}
		}
	}

	var resultado TransacaoCoordenada
	criada := false
	err := r.alterar(func() error {
		// A busca da chave e a criação ficam sob o mesmo mutex e snapshot.
		for _, existente := range r.transacoesCoordenadas {
			if existente.ChaveIdempotencia != chave {
				continue
			}
			if existente.Passageiro != passageiro || !reflect.DeepEqual(existente.Itinerario, tx.Itinerario) {
				return fmt.Errorf("%w: %s", ErrIdempotenciaConflitante, chave)
			}
			resultado = clonarTransacaoCoordenada(*existente)
			return nil
		}

		r.proximoIDTransacao++
		tx.ID = fmt.Sprintf("%s-%d", normalizarPrefixoID(r.empresaID), r.proximoIDTransacao)
		tx.AtualizadaEm = tx.CriadaEm
		copia := clonarTransacaoCoordenada(tx)
		r.transacoesCoordenadas[tx.ID] = &copia
		resultado = clonarTransacaoCoordenada(copia)
		criada = true
		return nil
	})
	return resultado, criada, err
}

// ListarTransacoesCoordenadasPendentes devolve copias de todas as coordenacoes
// ainda nao concluidas. A ordenacao torna a recuperacao deterministica.
func (r *Repositorio) ListarTransacoesCoordenadasPendentes() []TransacaoCoordenada {
	r.mu.RLock()
	defer r.mu.RUnlock()
	resultado := make([]TransacaoCoordenada, 0)
	for _, tx := range r.transacoesCoordenadas {
		if tx.Estado != CoordenacaoConcluida {
			resultado = append(resultado, clonarTransacaoCoordenada(*tx))
		}
	}
	sort.Slice(resultado, func(i, j int) bool {
		if resultado[i].CriadaEm.Equal(resultado[j].CriadaEm) {
			return resultado[i].ID < resultado[j].ID
		}
		return resultado[i].CriadaEm.Before(resultado[j].CriadaEm)
	})
	return resultado
}

// ListarTransacoesCoordenadasDoPassageiro devolve logs de coordenacao iniciados
// por este passageiro neste servidor. Eles permitem consultar as partes remotas
// de reservas coordenadas originalmente por esta empresa.
func (r *Repositorio) ListarTransacoesCoordenadasDoPassageiro(passageiro string) []TransacaoCoordenada {
	r.mu.RLock()
	defer r.mu.RUnlock()
	resultado := make([]TransacaoCoordenada, 0)
	for _, tx := range r.transacoesCoordenadas {
		if tx.Passageiro == passageiro && tx.Decisao == DecisaoConfirmar && tx.Estado == CoordenacaoConcluida {
			resultado = append(resultado, clonarTransacaoCoordenada(*tx))
		}
	}
	sort.Slice(resultado, func(i, j int) bool { return resultado[i].CriadaEm.Before(resultado[j].CriadaEm) })
	return resultado
}

// GerarIDTransacao produz um identificador local monotônico e o persiste
// antes de devolvê-lo. O prefixo identifica a empresa que coordenou a operação.
func (r *Repositorio) GerarIDTransacao() (string, error) {
	var id string
	err := r.alterar(func() error {
		r.proximoIDTransacao++
		id = fmt.Sprintf("%s-%d", normalizarPrefixoID(r.empresaID), r.proximoIDTransacao)
		return nil
	})
	return id, err
}

func normalizarPrefixoID(valor string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(valor) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	prefixo := strings.Trim(b.String(), "-")
	if prefixo == "" {
		return "empresa"
	}
	return prefixo
}

// CriarTransacaoCoordenada persiste o plano completo antes de qualquer
// chamada de rede. A repetição do mesmo registro é idempotente.
func (r *Repositorio) CriarTransacaoCoordenada(tx TransacaoCoordenada) error {
	tx = clonarTransacaoCoordenada(tx)
	if strings.TrimSpace(tx.ID) == "" || strings.TrimSpace(tx.Passageiro) == "" {
		return fmt.Errorf("id e passageiro da transacao coordenada sao obrigatorios")
	}
	if len(tx.Itinerario.Passos) == 0 || len(tx.Participantes) == 0 {
		return fmt.Errorf("transacao coordenada precisa de itinerario e participantes")
	}
	if tx.Estado == "" {
		tx.Estado = CoordenacaoPreparando
	}
	if tx.Estado != CoordenacaoPreparando || tx.Decisao != DecisaoNenhuma {
		return fmt.Errorf("nova transacao coordenada deve iniciar em PREPARANDO")
	}
	vistos := make(map[string]bool, len(tx.Participantes))
	for _, p := range tx.Participantes {
		if strings.TrimSpace(p.EmpresaID) == "" || len(p.Itinerario.Passos) == 0 {
			return fmt.Errorf("participante sem empresa ou itinerario")
		}
		if vistos[p.EmpresaID] {
			return fmt.Errorf("participante duplicado: %s", p.EmpresaID)
		}
		vistos[p.EmpresaID] = true
		for _, trecho := range p.Itinerario.Passos {
			if trecho.EmpresaID != p.EmpresaID {
				return fmt.Errorf("participante %s contem trecho da empresa %s", p.EmpresaID, trecho.EmpresaID)
			}
		}
	}
	if tx.CriadaEm.IsZero() {
		tx.CriadaEm = time.Now().UTC()
	}
	tx.AtualizadaEm = tx.CriadaEm

	return r.alterar(func() error {
		if existente, ok := r.transacoesCoordenadas[tx.ID]; ok {
			if existente.Passageiro == tx.Passageiro && reflect.DeepEqual(existente.Itinerario, tx.Itinerario) && reflect.DeepEqual(existente.Participantes, tx.Participantes) {
				return nil
			}
			return fmt.Errorf("id de coordenacao ja utilizado: %s", tx.ID)
		}
		copia := clonarTransacaoCoordenada(tx)
		r.transacoesCoordenadas[tx.ID] = &copia
		return nil
	})
}

func (r *Repositorio) ConsultarTransacaoCoordenada(id string) (TransacaoCoordenada, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tx, ok := r.transacoesCoordenadas[id]
	if !ok {
		return TransacaoCoordenada{}, fmt.Errorf("coordenacao %s nao encontrada", id)
	}
	return clonarTransacaoCoordenada(*tx), nil
}

func (r *Repositorio) MarcarPreparacaoTentada(id, empresaID string) error {
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		if tx.Decisao != DecisaoNenhuma {
			return fmt.Errorf("preparacao nao pode iniciar depois da decisao %s", tx.Decisao)
		}
		p, err := participanteLocked(tx, empresaID)
		if err != nil {
			return err
		}
		p.PreparacaoTentada = true
		tx.AtualizadaEm = time.Now().UTC()
		return nil
	})
}

func (r *Repositorio) RegistrarParticipantePreparado(id, empresaID string) error {
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		if tx.Decisao != DecisaoNenhuma {
			return fmt.Errorf("preparacao recebida depois da decisao")
		}
		p, err := participanteLocked(tx, empresaID)
		if err != nil {
			return err
		}
		if !p.PreparacaoTentada {
			return fmt.Errorf("participante %s nao teve preparacao tentada", empresaID)
		}
		p.Preparado = true
		p.UltimoErro = ""
		tx.AtualizadaEm = time.Now().UTC()
		return nil
	})
}

// DefinirDecisaoCoordenada grava a decisão antes de qualquer envio aos peers.
// Uma decisão já registrada é imutável.
func (r *Repositorio) DefinirDecisaoCoordenada(id string, decisao DecisaoCoordenacao) error {
	if decisao != DecisaoConfirmar && decisao != DecisaoAbortar {
		return fmt.Errorf("decisao coordenada invalida: %q", decisao)
	}
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		if tx.Decisao != DecisaoNenhuma {
			if tx.Decisao == decisao {
				return nil
			}
			return fmt.Errorf("decisao ja registrada como %s", tx.Decisao)
		}
		if decisao == DecisaoConfirmar {
			for _, p := range tx.Participantes {
				if !p.Preparado {
					return fmt.Errorf("participante %s ainda nao confirmou preparacao", p.EmpresaID)
				}
			}
		}
		tx.Decisao = decisao
		tx.Estado = CoordenacaoPendente
		tx.AtualizadaEm = time.Now().UTC()
		return nil
	})
}

func (r *Repositorio) MarcarConfirmacaoTentada(id, empresaID string) error {
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		if tx.Decisao != DecisaoConfirmar {
			return fmt.Errorf("a coordenacao nao decidiu CONFIRMAR")
		}
		p, err := participanteLocked(tx, empresaID)
		if err != nil {
			return err
		}
		p.ConfirmacaoTentada = true
		tx.AtualizadaEm = time.Now().UTC()
		return nil
	})
}

func (r *Repositorio) RegistrarParticipanteConfirmado(id, empresaID string, reservaID int) error {
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		if tx.Decisao != DecisaoConfirmar {
			return fmt.Errorf("a coordenacao nao decidiu CONFIRMAR")
		}
		p, err := participanteLocked(tx, empresaID)
		if err != nil {
			return err
		}
		if !p.ConfirmacaoTentada {
			return fmt.Errorf("confirmacao de %s nao foi enviada", empresaID)
		}
		if reservaID <= 0 {
			return fmt.Errorf("id de reserva invalido recebido de %s", empresaID)
		}
		p.Confirmado = true
		p.ReservaID = reservaID
		p.UltimoErro = ""
		tx.AtualizadaEm = time.Now().UTC()
		concluirCoordenacaoSeCompleta(tx)
		return nil
	})
}

func (r *Repositorio) MarcarAbortoTentado(id, empresaID string) error {
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		if tx.Decisao != DecisaoAbortar {
			return fmt.Errorf("a coordenacao nao decidiu ABORTAR")
		}
		p, err := participanteLocked(tx, empresaID)
		if err != nil {
			return err
		}
		p.AbortoTentado = true
		tx.AtualizadaEm = time.Now().UTC()
		return nil
	})
}

func (r *Repositorio) RegistrarParticipanteAbortado(id, empresaID string) error {
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		if tx.Decisao != DecisaoAbortar {
			return fmt.Errorf("a coordenacao nao decidiu ABORTAR")
		}
		p, err := participanteLocked(tx, empresaID)
		if err != nil {
			return err
		}
		if !p.AbortoTentado {
			return fmt.Errorf("aborto de %s nao foi enviado", empresaID)
		}
		p.Abortado = true
		p.UltimoErro = ""
		tx.AtualizadaEm = time.Now().UTC()
		concluirCoordenacaoSeCompleta(tx)
		return nil
	})
}

// RegistrarMotivoCoordenacao persiste a causa que levou à decisão de aborto.
func (r *Repositorio) RegistrarMotivoCoordenacao(id, motivo string) error {
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		tx.Motivo = motivo
		tx.AtualizadaEm = time.Now().UTC()
		return nil
	})
}

func (r *Repositorio) RegistrarErroParticipante(id, empresaID, mensagem string) error {
	return r.alterar(func() error {
		tx, err := r.transacaoCoordenadaLocked(id)
		if err != nil {
			return err
		}
		p, err := participanteLocked(tx, empresaID)
		if err != nil {
			return err
		}
		p.UltimoErro = mensagem
		tx.AtualizadaEm = time.Now().UTC()
		return nil
	})
}

func (r *Repositorio) transacaoCoordenadaLocked(id string) (*TransacaoCoordenada, error) {
	tx, ok := r.transacoesCoordenadas[id]
	if !ok {
		return nil, fmt.Errorf("coordenacao %s nao encontrada", id)
	}
	return tx, nil
}

func participanteLocked(tx *TransacaoCoordenada, empresaID string) (*ParticipanteCoordenacao, error) {
	for i := range tx.Participantes {
		if tx.Participantes[i].EmpresaID == empresaID {
			return &tx.Participantes[i], nil
		}
	}
	return nil, fmt.Errorf("empresa %s nao participa da transacao %s", empresaID, tx.ID)
}

func concluirCoordenacaoSeCompleta(tx *TransacaoCoordenada) {
	for _, p := range tx.Participantes {
		if tx.Decisao == DecisaoConfirmar && !p.Confirmado {
			return
		}
		if tx.Decisao == DecisaoAbortar && p.PreparacaoTentada && !p.Abortado {
			return
		}
	}
	tx.Estado = CoordenacaoConcluida
}

func clonarTransacaoCoordenada(tx TransacaoCoordenada) TransacaoCoordenada {
	tx.Itinerario = clonarItinerario(tx.Itinerario)
	tx.Participantes = append([]ParticipanteCoordenacao(nil), tx.Participantes...)
	for i := range tx.Participantes {
		tx.Participantes[i].Itinerario = clonarItinerario(tx.Participantes[i].Itinerario)
	}
	return tx
}
