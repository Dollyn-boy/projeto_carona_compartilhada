# QUEM VAI BORA — Sistema de Caronas Compartilhadas

Projeto da disciplina TEC502 (Concorrencia e Conectividade). Servidor
central com um cliente unico (motorista e passageiro, papel escolhido
no cadastro/login), comunicando-se por sockets TCP com protocolo de
aplicacao proprio.

## Arquitetura em camadas do servidor

Cinco camadas, cada uma so conhecendo a que esta logo abaixo:

  1. Rede (E/S)               -> internal/rede
  2. Protocolo                -> internal/protocolo
  3. Casos de uso              -> internal/casosdeuso
  4. Dominio + concorrencia    -> internal/dominio + internal/concorrencia
  5. Estado em memoria          -> internal/estado

## Estrutura do projeto

```
cmd/                       pontos de entrada (binarios)
  servidor/                  bootstrap do servidor central
  cliente/                    cliente unico: cadastro/login, depois menu
                             de motorista ou passageiro conforme o papel

internal/
  rede/                      camada 1 — aceita conexoes TCP; uma fila limitada
                             distribui requisicoes para workers fixos
  protocolo/                 camada 2 — formato das mensagens e framing sobre TCP
  casosdeuso/                 camada 3 — roteia por tipo de operacao, exige
                             sessao autenticada para operacoes privadas, chama
                             internal/estado
  dominio/                    camada 4 (parte 1) — tipos e regras de negocio
                             puras (carona, trecho, reserva, usuario, busca
                             de itinerarios — BFS e DFS)
  concorrencia/                camada 4 (parte 2) — travas por trecho
  estado/                     camada 5 — repositorio, mapas em memoria e
                             salvamento/carregamento do snapshot JSON
  clientenet/                  camada de rede do lado do cliente

test/carga/                  harness de teste automatizado de concorrencia
                             (multiplos clientes reais via rede, disputando
                             os mesmos trechos)
docker/                      Dockerfiles do servidor e do cliente
docs/protocolo.md            especificacao do protocolo de aplicacao
```


## As tres refatoracoes

### 1. Travas por trecho

Cada trecho elementar tem uma chave no formato `ID_DA_CARONA:INDICE`.
Uma reserva que usa um unico trecho trava apenas aquele trecho; uma reserva
com varios trechos adquire todas as travas em ordem fixa e as libera no fim.
Isso evita que duas reservas concorrentes confirmem o mesmo assento e evita
deadlock quando itinerarios se sobrepoem.

O repositorio tambem usa um `sync.RWMutex` para proteger os mapas, os
identificadores e o snapshot completo. Esse mutex nao substitui as travas por
trecho: ele protege a estrutura compartilhada do repositorio. Como a versao
simples salva o estado inteiro em um unico JSON a cada alteracao, as operacoes
de escrita ainda sao serializadas durante a atualizacao/persistencia. O ganho
principal desta versao e deixar a regra de concorrencia por recurso explicita
e testavel, sem adicionar diario de eventos ou transacoes distribuidas.

### 2. Fila de requisicoes e worker pool

O servidor cria oito workers por padrao e uma fila com capacidade de 64 tarefas.
A fila tem tamanho limitado; quando fica cheia, a leitura de novas mensagens
aguarda uma vaga em vez de criar mais goroutines. `rede.IniciarComWorkers` permite
configurar os valores para testes e testes de carga.

Continua existindo uma goroutine por conexao TCP para ler mensagens. O que nao
existe e uma goroutine nova para cada requisicao: o processamento vai para o
pool. Cada conexao espera sua resposta antes de enviar a requisicao seguinte,
preservando a ordem e evitando acesso concorrente a sessao de autenticacao.

### 3. Persistencia em JSON

Por padrao, `go run ./cmd/servidor` usa `dados/dados.json`. O arquivo guarda
caronas, assentos livres por trecho, reservas, usuarios (somente hash de senha)
e os proximos identificadores. Ao iniciar, o servidor carrega esse arquivo;
a cada alteracao bem-sucedida, escreve um novo snapshot em arquivo temporario
e troca o arquivo anterior. Se a gravacao falhar, a alteracao e desfeita em
memoria e a operacao retorna erro.

Para escolher outro arquivo, configure `VAIJUNTO_ARQUIVO_DADOS`. O formato tem
campo `versao` para que incompatibilidades futuras possam ser detectadas em vez
de carregar silenciosamente dados incorretos. Esta persistencia foi mantida
intencionalmente simples: e um snapshot completo, nao um log de eventos.

## Regra de dependencia entre camadas

Cada camada so pode chamar a de baixo, nunca pular ou chamar de volta pra
cima:

```
rede -> casosdeuso -> estado -> (dominio + concorrencia)
```

`internal/rede` nao sabe o que e uma "carona". `internal/casosdeuso` nao
mexe em nenhuma trava diretamente — so chama `internal/estado`, que e o
unico pacote que combina dados com controle de concorrencia.


## Build e testes

```bash
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

Os testes cobrem travas por trecho, limite de concorrencia do worker pool,
disputa simultanea por um unico assento, atomicidade de itinerarios e
restauracao de usuarios/reservas/disponibilidade apos reabrir o JSON.

## Testando sem Docker

O servidor fica bloqueado escutando — precisa de dois terminais:

```
# terminal 1
go run ./cmd/servidor

# terminal 2 (com o terminal 1 ainda rodando)
go run ./cmd/cliente
```

O cliente pede pra cadastrar ou autenticar antes de mostrar qualquer
menu — todas as operacoes privadas exigem sessao autenticada.

## Rodando com Docker

O cliente aceita o endereco do servidor via flag (`-servidor`, padrao
`localhost:8080`) — isso existe justamente porque `localhost` dentro de
um container aponta pro proprio container, nunca pro servidor rodando
em outro lugar.

### Opcao A — docker-compose (dois containers, mesma maquina)

```bash
docker compose up --build -d servidor
docker compose logs servidor          # espere aparecer "servidor escutando em :8080"
docker compose run --rm cliente
```

O servico do servidor usa o volume nomeado `vaijunto-dados`, montado em
`/dados`. Assim, o snapshot JSON sobrevive a reinicios e recriacoes do
container. Evite `docker compose down -v` se quiser preservar os dados, pois
a opcao `-v` remove tambem os volumes.

Dentro da rede que o compose cria, o hostname `servidor` resolve
sozinho pro container do servico `servidor` — por isso o
`command: ["-servidor", "servidor:8080"]` no docker-compose.yml
funciona. Pra simular dois clientes ao mesmo tempo (um motorista, um
passageiro), rode `docker compose run --rm cliente` de novo em outro
terminal — cada `run` sobe um container novo, independente.

### Opcao B — dois containers manuais, rede Docker explicita

```
docker build -f docker/Dockerfile.servidor -t vaijunto-servidor .
docker build -f docker/Dockerfile.cliente -t vaijunto-cliente .

docker network create vaijunto-net
docker volume create vaijunto-dados
docker run --rm -d --network vaijunto-net --name servidor -p 8080:8080 \
  -e VAIJUNTO_ARQUIVO_DADOS=/dados/dados.json -v vaijunto-dados:/dados vaijunto-servidor
docker run --rm -it --network vaijunto-net vaijunto-cliente -servidor servidor:8080
```

### Opcao C — maquinas fisicas separadas (o teste real do laboratorio)

```
# Maquina A (roda o servidor)
docker build -f docker/Dockerfile.servidor -t vaijunto-servidor .
docker volume create vaijunto-dados
docker run --rm -p 8080:8080 -e VAIJUNTO_ARQUIVO_DADOS=/dados/dados.json \
  -v vaijunto-dados:/dados vaijunto-servidor

# Maquina B (roda o cliente, apontando pro IP real da Maquina A)
docker build -f docker/Dockerfile.cliente -t vaijunto-cliente .
docker run --rm -it vaijunto-cliente -servidor 192.168.0.10:8080
```
