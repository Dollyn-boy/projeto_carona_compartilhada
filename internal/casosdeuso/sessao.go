package casosdeuso

import (
	"time"
)

type Sessao struct {
	Usuario         string
	Autenticado     bool
	Role            string // Adicionei o campo Role para armazenar o papel do usuário (Admin, Motorista, Passageiro)
	UltimaAtividade time.Time
	// Você pode adicionar outras regras aqui, como "Role" (Admin, Motorista, Passageiro)
}

func NovaSessao() *Sessao {
	return &Sessao{
		Autenticado: false,
	}
}
