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
