package dominio

import (
	"crypto/sha256"
	"fmt"
)

// Usuario guarda o hash da senha, e nao a senha original.
type Usuario struct {
	Usuario   string `json:"usuario"`
	SenhaHash string `json:"senha_hash"`
	Role      string `json:"role"`
}

func HashSenha(senha string) string {
	// Hash simples para este prototipo didatico. Sistemas reais devem usar
	// um algoritmo proprio para senhas, como Argon2id ou bcrypt.
	return fmt.Sprintf("%x", sha256.Sum256([]byte(senha)))
}
