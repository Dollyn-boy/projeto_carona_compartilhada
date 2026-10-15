package main

import (
	"log"
	"os"
	"path/filepath"

	"vaijunto/internal/estado"
	"vaijunto/internal/rede"
)

func main() {
	// Por padrao, o servidor grava os dados em ./dados/dados.json.
	// Docker Compose configura um volume persistente e substitui esse caminho
	// pela variavel VAIJUNTO_ARQUIVO_DADOS.
	arquivo := os.Getenv("VAIJUNTO_ARQUIVO_DADOS")
	if arquivo == "" {
		arquivo = filepath.Join("dados", "dados.json")
	}

	repo, err := estado.NovoRepositorioComArquivo(arquivo)
	if err != nil {
		log.Fatalf("nao foi possivel abrir os dados: %v", err)
	}
	log.Printf("dados carregados de %s", arquivo)

	if err := rede.Iniciar(":8080", repo); err != nil {
		log.Fatal(err)
	}
}
