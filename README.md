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
  rede/                      camada 1 — aceita conexoes TCP, 1 goroutine por
                             conexao (com timeout de inatividade), so fala
                             bytes/mensagens ja parseadas
  protocolo/                 camada 2 — formato das mensagens e framing sobre TCP
  casosdeuso/                 camada 3 — roteia por tipo de operacao, exige
                             sessao autenticada para operacoes privadas, chama
                             internal/estado
  dominio/                    camada 4 (parte 1) — tipos e regras de negocio
                             puras (carona, trecho, reserva, usuario, busca
                             de itinerarios — BFS e DFS)
  concorrencia/                camada 4 (parte 2) — trava global (RWMutex)
  estado/                     camada 5 — o repositorio em memoria de fato:
                             combina dominio + concorrencia para expor
                             operacoes seguras (inclui cadastro/autenticacao
                             de usuarios)
  clientenet/                  camada de rede do lado do cliente

test/carga/                  harness de teste automatizado de concorrencia
                             (multiplos clientes reais via rede, disputando
                             os mesmos trechos)
docker/                      Dockerfiles do servidor e do cliente
docs/protocolo.md            especificacao do protocolo de aplicacao
```

## Regra de dependencia entre camadas

Cada camada so pode chamar a de baixo, nunca pular ou chamar de volta pra
cima:

```
rede -> casosdeuso -> estado -> (dominio + concorrencia)
```

`internal/rede` nao sabe o que e uma "carona". `internal/casosdeuso` nao
mexe em nenhuma trava diretamente — so chama `internal/estado`, que e o
unico pacote que combina dados com controle de concorrencia.

## Status atual

Implementado e testado: protocolo (envelope + framing), rede (accept
loop, timeout de inatividade, tratamento de mensagem malformada sem
derrubar a conexao), dominio (Carona/Trecho/Reserva/Usuario, grafo de
busca com BFS e DFS, ordenacao por numero de trechos e depois preco),
concorrencia (trava global RWMutex), estado (as 7 operacoes de negocio +
cadastro/autenticacao de usuario, com ownership check em cancelamentos e
devolucao de assento em cascata), autenticacao (sessao por conexao,
gate central de autorizacao), e o cliente unico com os dois menus.

Ainda TODO: `test/carga` (harness de carga obrigatorio), testes
dedicados em `internal/concorrencia` e `internal/dominio` (o grafo ja
tem cobertura em `busca_test.go`; a trava e o restante do dominio
ainda nao).

## Build

```
go build ./...
go vet ./...
go test -race ./...
```

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

Mais simples pra testar localmente antes do laboratorio. Suba o
servidor primeiro, confira que ele esta escutando, e so entao rode o
cliente — assim evita a corrida de o cliente tentar conectar antes do
servidor estar pronto:

```
docker compose up --build -d servidor
docker compose logs servidor          # espere aparecer "servidor escutando em :8080"
docker compose run --rm cliente
```

Dentro da rede que o compose cria, o hostname `servidor` resolve
sozinho pro container do servico `servidor` — por isso o
`command: ["-servidor", "servidor:8080"]` no docker-compose.yml
funciona. Pra simular dois clientes ao mesmo tempo (um motorista, um
passageiro), rode `docker compose run --rm cliente` de novo em outro
terminal — cada `run` sobe um container novo, independente.

### Opcao B — dois containers manuais, rede Docker explicita

Mais proximo do que vai acontecer de verdade (maquinas fisicas
distintas), ainda rodando na mesma maquina pra testar:

```
docker build -f docker/Dockerfile.servidor -t vaijunto-servidor .
docker build -f docker/Dockerfile.cliente -t vaijunto-cliente .

docker network create vaijunto-net
docker run --rm -d --network vaijunto-net --name servidor -p 8080:8080 vaijunto-servidor
docker run --rm -it --network vaijunto-net vaijunto-cliente -servidor servidor:8080
```

### Opcao C — maquinas fisicas separadas (o teste real do laboratorio)

Sem rede Docker compartilhada nenhuma — so a porta exposta e o IP real
da outra maquina. **Este e o cenario que o item 11 do barema pergunta
diretamente** ("como foi resolvido o problema de conectividade entre
conteineres executados em maquinas distintas") — a resposta e: nao ha
mecanismo especial nenhum de descoberta, o cliente so precisa saber o
IP real da maquina do servidor na rede do laboratorio, passado via
`-servidor`, e a porta precisa estar publicada (`-p 8080:8080`) no host
do servidor.

```
# Maquina A (roda o servidor)
docker run --rm -p 8080:8080 vaijunto-servidor

# Maquina B (roda o cliente, apontando pro IP real da Maquina A)
docker run --rm -it vaijunto-cliente -servidor 192.168.0.10:8080
```
