package protocolo

// Estrutura de dados que deve ser seguida no tráfego da  API dos consórcios
type HealthResposta struct {
	Status    string `json:"status"`
	EmpresaID string `json:"empresa_id"`
}

type TrechoDisponivelDados struct {
	Trecho              TrechoDados `json:"trecho"`
	AssentosDisponiveis int         `json:"assentos_disponiveis"`
	Preco               int         `json:"preco"`
}

type TrechosDisponiveisResposta struct {
	EmpresaID string                  `json:"empresa_id"`
	Trechos   []TrechoDisponivelDados `json:"trechos"`
}

type ErroREST struct {
	Erro string `json:"erro"`
}

// PrepararTransacaoDados é enviado ao participante que deve reservar
// temporariamente os trechos pertencentes à sua empresa.
type PrepararTransacaoDados struct {
	Passageiro string        `json:"passageiro"`
	Trechos    []TrechoDados `json:"trechos"`
}

// TransacaoParticipanteResposta confirma o estado observado pelo participante.
type TransacaoParticipanteResposta struct {
	TransacaoID string `json:"transacao_id"`
	EmpresaID   string `json:"empresa_id"`
	Estado      string `json:"estado"`
	ReservaID   int    `json:"reserva_id,omitempty"`
}

// CoordenarReservaResposta é devolvido ao passageiro pelo servidor que
// coordenou esta transação específica.
type CoordenarReservaResposta struct {
	TransacaoID       string            `json:"transacao_id"`
	Estado            string            `json:"estado"`
	Decisao           string            `json:"decisao,omitempty"`
	Mensagem          string            `json:"mensagem,omitempty"`
	Reservas          []ReservaResposta `json:"reservas,omitempty"`
	EmpresasPendentes []string          `json:"empresas_pendentes,omitempty"`
}

// TransacaoEstadoResposta é usado pelo endpoint de consulta local do participante.
type TransacaoEstadoResposta struct {
	TransacaoID string `json:"transacao_id"`
	EmpresaID   string `json:"empresa_id"`
	Estado      string `json:"estado"`
	ReservaID   int    `json:"reserva_id,omitempty"`
}
