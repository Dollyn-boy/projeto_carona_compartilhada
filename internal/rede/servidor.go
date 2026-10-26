// Package rede implementa a parte de rede do servidor VaiJunto.
//
// Responsabilidades deste arquivo:
//  1. abrir uma porta TCP para receber conexoes;
//  2. ler as requisicoes enviadas pelos clientes;
//  3. colocar cada requisicao em uma fila;
//  4. deixar um conjunto fixo de workers executar as operacoes;
//  5. enviar a resposta de volta pela mesma conexao TCP.
//

package rede

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"vaijunto/internal/casosdeuso"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

const (
	// Se o cliente ficar cinco minutos sem mandar uma mensagem, encerramos
	// a conexao para nao manter conexoes abandonadas abertas indefinidamente.
	timeoutInatividade = 5 * time.Minute

	// Prazo maximo configurado para tentar escrever uma resposta no socket.
	timeoutEscrita = 30 * time.Second

	// Quantidade maxima padrao de requisicoes esperando na fila.
	// Isso NAO inclui as requisicoes que os workers ja estao executando.
	filaPadrao = 64

	// Quantidade padrao de requisicoes que podem ser processadas ao mesmo tempo.
	workersPadrao = 8
)

// workerPool representa a fila de trabalho e os trabalhadores que a consomem.
//
// A fila guarda funcoes sem argumentos (func()). Cada funcao representa uma
// requisicao que ja foi lida da rede e esta pronta para ser processada.
// Aqui as tarefas sao funcoes anonimas que chamam casosdeuso.Despachar() e depois respondem ao cliente.
type workerPool struct {
	fila chan func()    // Canal compartilhado: conexoes colocam tarefas, workers retiram.
	wg   sync.WaitGroup // Permite esperar que todos os workers terminem ao fechar o pool.
}

// novoWorkerPool cria um pool com um numero fixo de goroutines trabalhadoras.
//
// Exemplo: novoWorkerPool(8, 64) cria 8 workers e uma fila que comporta ate
// 64 tarefas aguardando. Quando a fila esta cheia, quem tenta inserir tarefa
// fica bloqueado ate algum worker retirar uma tarefa da fila.
func novoWorkerPool(numeroWorkers, capacidadeFila int) *workerPool {
	// Protecao para chamadas internas ou testes com valores invalidos.
	// IniciarComWorkers valida os mesmos valores na entrada do servidor.
	if numeroWorkers < 1 {
		numeroWorkers = 1
	}
	if capacidadeFila < 1 {
		capacidadeFila = 1
	}

	// O segundo argumento de make cria um canal com buffer.
	// Um canal sem buffer faria o produtor esperar por um worker imediatamente.

	p := &workerPool{fila: make(chan func(), capacidadeFila)}

	for i := 0; i < numeroWorkers; i++ {
		//  Para cada worker, incrementamos o contador do WaitGroup. Ele sera decrementado
		//  quando o worker terminar (ou seja, quando a goroutine terminar).
		p.wg.Add(1)

		// Cada goroutine abaixo é um worker. Todos consomem a MESMA fila.
		go func() {
			defer p.wg.Done() // Sinaliza que este worker terminou.

			// range aguarda novas tarefas. Quando a fila for fechada, o worker
			// ainda consome as tarefas que ja estavam nela e depois termina.
			for tarefa := range p.fila {
				tarefa() // Executa uma requisicao; depois volta a buscar outra.
			}
		}()
	}

	return p
}

// enfileirar adiciona uma tarefa para os workers executarem.
//
// Esta operacao pode bloquear: se todos os espacos da fila estiverem ocupados,
// a goroutine que chamou enfileirar espera ate existir espaco. Esse efeito e
// chamado backpressure; ele evita criar uma goroutine adicional por requisicao.
func (p *workerPool) enfileirar(tarefa func()) {
	// tarefa() e uma funcao anonima que encapsula a logica de processar uma requisicao
	p.fila <- tarefa
}

// fechar encerra a fila e espera os workers acabarem.
//
// Regra importante: so chame fechar depois que todos os produtores tiverem
// parado de chamar enfileirar. Enfileirar em um canal fechado provoca panic.
func (p *workerPool) fechar() {
	close(p.fila)
	p.wg.Wait()
}

// conexaoCliente junta o socket e um mutex para proteger as escritas.
//
// Mesmo que atualmente uma conexao espere cada resposta antes de ler a proxima
// requisicao, o mutex garante que duas chamadas a responder nao misturem os
// bytes das mensagens no mesmo socket.
type conexaoCliente struct {
	conn net.Conn
	mu   sync.Mutex
}

// responder serializa a escrita de uma resposta naquela conexao.
func (c *conexaoCliente) responder(resposta protocolo.Resposta) error {
	c.mu.Lock()
	defer c.mu.Unlock() // Libera o mutex inclusive se a escrita retornar erro.

	// Se a escrita demorar demais, o socket deixa de esperar indefinidamente.
	// O erro ao configurar o prazo e ignorado nesta implementacao.
	_ = c.conn.SetWriteDeadline(time.Now().Add(timeoutEscrita))

	// EscreverMensagem pertence ao pacote protocolo: codifica a resposta no
	// formato definido pelo protocolo e a transmite pela conexao.
	return protocolo.EscreverMensagem(c.conn, resposta)
}

// Iniciar usa os valores padrao: oito workers e uma fila para 64 tarefas.
//
// Este metodo e um atalho para nao precisar configurar os numeros em todos os
// lugares que iniciam o servidor.

func Iniciar(endereco string, repo *estado.Repositorio) error {
	return IniciarComWorkers(
		endereco,
		repo,
		workersPadrao,
		filaPadrao,
	)
}

// IniciarComBusca inicia o servidor TCP com suporte à busca
// distribuída entre empresas. O serviço pode ser nil, caso em
// que as buscas continuam sendo exclusivamente locais.
func IniciarComBusca(
	endereco string,
	repo *estado.Repositorio,
	busca *consorcio.BuscaDistribuida,
) error {
	return iniciarComWorkersEBusca(
		endereco,
		repo,
		workersPadrao,
		filaPadrao,
		busca,
	)
}

// IniciarComWorkers mantém a inicialização com quantidade
// configurável de workers, sem serviço de busca distribuída.
func IniciarComWorkers(
	endereco string,
	repo *estado.Repositorio,
	numeroWorkers int,
	capacidadeFila int,
) error {
	return iniciarComWorkersEBusca(
		endereco,
		repo,
		numeroWorkers,
		capacidadeFila,
		nil,
	)
}

// iniciarComWorkersEBusca contém a implementação compartilhada
// pelas diferentes formas de inicializar o servidor.
func iniciarComWorkersEBusca(
	endereco string,
	repo *estado.Repositorio,
	numeroWorkers int,
	capacidadeFila int,
	busca *consorcio.BuscaDistribuida,
) error {
	if numeroWorkers < 1 {
		return fmt.Errorf("numero de workers precisa ser maior que zero")
	}

	if capacidadeFila < 1 {
		return fmt.Errorf("capacidade da fila precisa ser maior que zero")
	}

	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		return fmt.Errorf("falha ao escutar em %s: %w", endereco, err)
	}
	defer listener.Close()

	pool := novoWorkerPool(numeroWorkers, capacidadeFila)
	defer pool.fechar()

	log.Printf(
		"servidor escutando em %s (workers=%d, fila=%d)",
		endereco,
		numeroWorkers,
		capacidadeFila,
	)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("falha ao aceitar conexao: %v", err)
			continue
		}

		go tratarConexao(conn, repo, pool, busca)
	}
}

// tratarConexao administra uma conexao TCP durante o tempo em que estiver aberta.
// Enfileira cada requisicao recebida para o pool de workers e envia a resposta de volta ao cliente.
func tratarConexao(
	conn net.Conn,
	repo *estado.Repositorio,
	pool *workerPool,
	busca *consorcio.BuscaDistribuida,
) {
	defer conn.Close() // Ao sair deste metodo, encerra o socket do cliente.

	// conexaoCliente empacota o socket e o mutex para proteger as escritas.
	cliente := &conexaoCliente{conn: conn}
	reader := bufio.NewReader(conn)   // Reutiliza o buffer para ler mensagens completas.
	sessao := casosdeuso.NovaSessao() // Login e dados de sessao pertencem a esta conexao.

	for {
		// Limita quanto tempo esperamos pela proxima mensagem do cliente.
		_ = conn.SetReadDeadline(time.Now().Add(timeoutInatividade))

		var req protocolo.Requisicao
		// LerMensagem interpreta uma mensagem do protocolo (JSON delimitado
		// pelo framing usado pelo pacote protocolo).
		err := protocolo.LerMensagem(reader, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// EOF normalmente significa que o cliente fechou a conexao.
				return
			}

			var erroRede net.Error
			if errors.As(err, &erroRede) && erroRede.Timeout() {
				log.Printf("conexao encerrada por inatividade (%s sem mensagens)", timeoutInatividade)
				return
			}

			// Se o JSON estiver sintaticamente incorreto ou tiver um tipo de
			// campo incompatível, tentamos informar o cliente sem derrubar a
			// conexao imediatamente. Como a mensagem pode nao ter sido lida por
			// completo como uma requisicao valida, esta resposta nao inclui ID.
			var erroSintaxe *json.SyntaxError
			var erroTipo *json.UnmarshalTypeError
			if errors.As(err, &erroSintaxe) || errors.As(err, &erroTipo) {
				respErro := protocolo.Resposta{Status: "erro", Motivo: "mensagem malformada: " + err.Error()}
				if err := cliente.responder(respErro); err != nil {
					log.Printf("erro ao responder mensagem malformada: %v", err)
					return
				}
				continue // Tenta ler outra mensagem nesta conexao.
			}

			// Erros de leitura que nao foram reconhecidos acima encerram a conexao.
			log.Printf("erro ao ler mensagem: %v", err)
			return
		}

		// Aqui a mensagem foi lida e convertida para protocolo.Requisicao.
		// Ainda nao estamos executando a regra de negocio: primeiro criamos
		// uma tarefa para o worker pool.
		concluida := make(chan struct{})

		// Encaminhanmento da requisicao para o pool de workers. A tarefa e uma funcao anonima
		// que chama casosdeuso.Despachar() e depois responde ao cliente.
		pool.enfileirar(func() {
			// Implementação da tarefa anonima que o worker vai executar. Ela tem acesso a req, sessao, repo e cliente.

			// A tarefa fecha este canal ao terminar. Isso avisa a goroutine da
			// conexao que ela pode ler a proxima requisicao.
			defer close(concluida)

			// O despachante escolhe o caso de uso a partir de req.Tipo. Ele pode
			// chamar o repositorio para consultar ou modificar os dados.
			var resposta protocolo.Resposta

			if busca != nil {
				resposta = casosdeuso.DespacharComBusca(
					repo,
					sessao,
					req,
					busca,
				)
			} else {
				resposta = casosdeuso.Despachar(
					repo,
					sessao,
					req,
				)
			}

			// Depois de processar, o worker envia a resposta pela mesma conexao.
			if err := cliente.responder(resposta); err != nil {
				log.Printf("erro ao enviar resposta: %v", err)
				_ = conn.Close() // Se nao conseguimos responder, encerramos o socket.
			}
		})

		// Ponto importante: esta goroutine espera a tarefa terminar antes de
		// voltar ao inicio do loop e ler outra mensagem.
		//
		// Assim, uma conexao tem no maximo uma requisicao em processamento por
		// vez. Isso preserva a ordem das respostas e evita que dois workers
		// alterem simultaneamente a mesma sessao (por exemplo, login/logout).
		// Conexoes diferentes, por outro lado, podem usar workers diferentes.
		<-concluida
	}
}

// A diferença essencial: sem o worker pool, seria comum criar uma goroutine para processar cada requisição.
// Com o pool, as requisições são encaminhadas para uma quantidade fixa de workers.
// Uma goroutine por conexão continua existindo; o que fica limitado é o número de requisições em processamento simultâneo.
