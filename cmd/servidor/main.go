package main

import (
	"log"

	"vaijunto/internal/estado"
	"vaijunto/internal/rede"
)

func main() {
	repo := estado.NovoRepositorio("VaiJunto")

	if err := rede.Iniciar(":8080", repo); err != nil {
		log.Fatal(err)
	}
}
