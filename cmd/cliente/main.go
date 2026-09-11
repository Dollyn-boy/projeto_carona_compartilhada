package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"vaijunto/internal/clientenet"
	"vaijunto/internal/protocolo"
)

var proximoID = 0

var ultimaBusca []protocolo.ItinerarioDados

func main() {
    endereco := flag.String("servidor", "localhost:8080", "endereco do servidor (host:porta)")
    flag.Parse()

    conexao, err := clientenet.Conectar(*endereco)
    if err != nil {
        log.Fatal(err)
    }
    defer conexao.Fechar()

    leitor := bufio.NewReader(os.Stdin)
    
    // Laço para garantir que o usuário escolha uma opção válida
    for {
        opcao := lerLinha(leitor, "Você deseja se cadastrar ou autenticar? (c/a): ")
        if opcao == "c" {
            cadastrar_cliente(conexao, leitor)
            break // Sai do loop após o fluxo de cadastro terminar
        } else if opcao == "a" {
            login_cliente(conexao, leitor)
            break // Sai do loop após o fluxo de login terminar
        } else {
            fmt.Println("Opção inválida. Digite 'c' para cadastrar ou 'a' para autenticar.")
        }
    }
}

func cadastrar_cliente(conexao *clientenet.Conexao, leitor *bufio.Reader) {
    // Laço infinito: só sairemos daqui usando 'return' quando o status for "ok"
    for {
        role := lerLinha(leitor, "Você é motorista ou passageiro? (m/p): ")
        
        // Validação simples (opcional) para evitar enviar dados inúteis ao servidor
        if role != "m" && role != "p" {
            fmt.Println("Papel inválido. Digite 'm' ou 'p'.")
            continue // Reinicia o loop imediatamente
        }

        usuario := lerLinha(leitor, "Nome de Usuário: ")
        senha := lerLinha(leitor, "Senha: ")

        cadastro := protocolo.CadastroDados{
            Usuario: usuario,
            Role:    role,
            Senha:   senha,
        }

        dados, err := json.Marshal(cadastro)
        if err != nil {
            fmt.Println("Erro ao criar dados de cadastro:", err)
            continue
        }

        req := protocolo.Requisicao{
            Tipo:         "cadastro",
            IDRequisicao: proximoIDString(),
            Dados:        dados,
        }

        resp, err := conexao.EnviarRequisicao(req)
        if err != nil {
            // Se der erro de rede, encerramos pois o servidor pode ter caído
            log.Fatal("Erro ao enviar requisição de cadastro:", err)
        }

        // Verifica a resposta do servidor
        if resp.Status == "ok" {
            fmt.Println("\nCadastro realizado com sucesso!")
            menu(conexao, leitor, role)
            return // Sai da função e quebra o loop, pois o cadastro deu certo
        } else {
            // Se der erro (ex: usuário já existe), avisa e deixa o loop recomeçar
            fmt.Printf("\nFalha no cadastro: %s\n", resp.Motivo)
            fmt.Println("Tente novamente.\n")
        }
    }
}


func login_cliente(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	// Laço infinito: só sairemos daqui usando 'return' quando o status for "ok"
	for {
		usuario := lerLinha(leitor, "Nome de Usuário: ")
		senha := lerLinha(leitor, "Senha: ")

		login := protocolo.LoginDados{
			Usuario: usuario,
			Senha:   senha,
		}

		dados, err := json.Marshal(login)
		if err != nil {
			fmt.Println("Erro ao criar dados de login:", err)
			continue
		}

		req := protocolo.Requisicao{
			Tipo:         "login",
			IDRequisicao: proximoIDString(),
			Dados:        dados,
		}

		resp, err := conexao.EnviarRequisicao(req)
		if err != nil {
			// Se der erro de rede, encerramos pois o servidor pode ter caído
			log.Fatal("Erro ao enviar requisição de login:", err)
		}

		if resp.Status == "ok" {
			fmt.Println("\nLogin realizado com sucesso!")
			menu(conexao, leitor, resp.Role)
			return
		} else {
			fmt.Printf("\nFalha no login: %s\n", resp.Motivo)
			fmt.Println("Tente novamente.\n")
		}
	}
}