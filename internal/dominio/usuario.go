package dominio

import (
	"crypto/sha256"
	"fmt"
)

type Usuario struct {
	Usuario   string
	SenhaHash string
}

func HashSenha(senha string) string {
	// Implementação de hash de senha (exemplo simples, não seguro para produção)
	// Em um cenário real, você deve usar uma biblioteca de hashing segura, como bcrypt.
	return fmt.Sprintf("%x", sha256.Sum256([]byte(senha)))
}
