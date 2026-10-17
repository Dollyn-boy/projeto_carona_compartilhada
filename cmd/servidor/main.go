package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"vaijunto/internal/estado"
	"vaijunto/internal/rede"
)

func main() {
	// Identificação da instância do servidor que irá rodar
	empresaID := strings.TrimSpace(os.Getenv("VAIJUNTO_EMPRESA_ID"))

	if empresaID == "" {
		empresaID = "vaijunto-local"
	}

	// Endereco no qual o servidor vai escutar.
	endereco := strings.TrimSpace(os.Getenv("VAIJUNTO_ENDERECO"))
	if endereco == "" {
		endereco = ":8080"
	}

	arquivo := os.Getenv("VAIJUNTO_ARQUIVO_DADOS")
	if arquivo == "" {
		arquivo = filepath.Join("dados", "dados.json")
	}

	repo, err := estado.NovoRepositorioDeEmpresaComArquivo(
		empresaID,
		arquivo,
	)
	if err != nil {
		log.Fatalf("nao foi possivel abrir os dados: %v", err)
	}

	log.Printf("empresa=%s; dados carregados de %s", empresaID, arquivo)

	if err := rede.Iniciar(endereco, repo); err != nil {
		log.Fatal(err)
	}
}
