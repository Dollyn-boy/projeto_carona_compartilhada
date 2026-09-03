# VaiJunto — Sistema de Caronas Compartilhadas

Projeto da disciplina TEC502 (Concorrencia e Conectividade). Servidor
central com clientes de motorista e passageiro, comunicando-se por
sockets TCP com protocolo de aplicacao proprio.

## Arquitetura em camadas do servidor

Conforme o modelo proposto em aula: dois clientes (motorista e
passageiro), cada um em uma maquina/conteiner diferente, conectam-se ao
servidor atraves de um protocolo (a seta). Dentro do servidor existem
cinco camadas, cada uma so conhecendo a que esta logo abaixo:

  1. Rede (E/S)               -> internal/rede
  2. Protocolo                -> internal/protocolo
  3. Casos de uso              -> internal/casosdeuso
  4. Dominio + concorrencia    -> internal/dominio + internal/concorrencia
  5. Estado em memoria          -> internal/estado

## Estrutura do projeto

```
cmd/                       pontos de entrada (binarios)
  servidor/                  bootstrap do servidor central
  cliente-motorista/         bootstrap do cliente motorista
  cliente-passageiro/        bootstrap do cliente passageiro

internal/
  rede/                      camada 1 — aceita conexoes TCP, 1 goroutine por
                             conexao, so fala bytes/mensagens ja parseadas
  protocolo/                 camada 2 — formato das mensagens e framing sobre TCP
  casosdeuso/                 camada 3 — roteia por tipo de operacao e chama
                             os handlers, que por sua vez chamam internal/estado
  dominio/                    camada 4 (parte 1) — tipos e regras de negocio
                             puras (carona, trecho, reserva, busca de itinerarios)
  concorrencia/                camada 4 (parte 2) — mecanismo de travas/versionamento
  estado/                     camada 5 — o repositorio em memoria de fato: combina
                             dominio + concorrencia para expor operacoes seguras
  clientenet/                  camada de rede compartilhada pelos dois clientes

test/carga/                  harness de teste automatizado de concorrencia
docker/                      Dockerfiles do servidor e dos clientes
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
unico pacote que combina dados com controle de concorrencia. Essa
fronteira e o que garante que toda leitura/escrita do estado passa pelo
controle de concorrencia, sem excecao.

## Status atual

Ja implementados de verdade (nao sao mais so TODO):
`internal/protocolo` (envelope + framing), `internal/clientenet`,
`internal/rede`, `internal/casosdeuso` (so a operacao `ping`),
`cmd/servidor` e `cmd/cliente-motorista`. Isso e suficiente pra validar
a tubulação completa com um ping — ver "Testando o ping" abaixo.

Ainda TODO (so comentarios de instrucao): `internal/dominio`,
`internal/concorrencia`, `internal/estado`, os handlers reais de
`internal/casosdeuso` (motorista.go/passageiro.go), `cmd/cliente-passageiro`,
`test/carga` e `docker/Dockerfile.cliente-passageiro`.

Ordem sugerida de implementacao:

1. `internal/dominio` — tipos e regras de negocio puras, sem estado global.
2. `internal/concorrencia` — mecanismo de travas, validado isoladamente
   com goroutines (`go test -race`).
3. `internal/estado` — combina os dois acima num repositorio em memoria
   seguro; valide com testes de integracao concorrentes.
4. `docs/protocolo.md` + `internal/protocolo` — especificar e implementar
   o formato das mensagens e o framing sobre TCP.
5. `internal/rede` — camada de E/S pura (accept loop, goroutine por
   conexao), sem nenhuma logica de negocio.
6. `internal/casosdeuso` — roteamento por tipo de operacao + handlers,
   ligando `internal/rede` a `internal/estado`.
7. `internal/clientenet` + `cmd/cliente-motorista` + `cmd/cliente-passageiro`
   — clientes de terminal.
8. `test/carga` — teste automatizado de concorrencia contra o servidor real.
9. `docker/` + `docker-compose.yml` — conteinerizacao e testes entre maquinas.

## Build

```
go build ./...
go vet ./...
go test -race ./...
```

## Testando o ping (sem Docker)

O servidor fica bloqueado escutando — precisa de dois terminais:

```
# terminal 1
go run ./cmd/servidor

# terminal 2 (com o terminal 1 ainda rodando)
go run ./cmd/cliente-motorista
```

Deve aparecer `resposta do servidor: status=ok dados="pong"`.

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
docker compose run --rm cliente-motorista
```

Dentro da rede que o compose cria, o hostname `servidor` resolve
sozinho pro container do servico `servidor` — por isso o
`command: ["-servidor", "servidor:8080"]` no docker-compose.yml funciona.

### Opcao B — dois containers manuais, rede Docker explicita

Mais proximo do que vai acontecer de verdade (maquinas fisicas
distintas), ainda rodando na mesma maquina pra testar:

```
docker build -f docker/Dockerfile.servidor -t vaijunto-servidor .
docker build -f docker/Dockerfile.cliente-motorista -t vaijunto-cliente-motorista .

docker network create vaijunto-net
docker run --rm -d --network vaijunto-net --name servidor -p 8080:8080 vaijunto-servidor
docker run --rm --network vaijunto-net vaijunto-cliente-motorista -servidor servidor:8080
```

### Opcao C — maquinas fisicas separadas (o teste real do laboratorio)

Sem rede Docker compartilhada nenhuma — so a porta exposta e o IP real
da outra maquina:

```
# Maquina A (roda o servidor)
docker run --rm -p 8080:8080 vaijunto-servidor

# Maquina B (roda o cliente, apontando pro IP real da Maquina A)
docker run --rm vaijunto-cliente-motorista -servidor 192.168.0.10:8080
```
