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

## Como usar este esqueleto

Todos os arquivos em `internal/`, `cmd/`, `test/` e `docker/` contem
apenas comentarios de instrucao (TODO) — nenhuma logica foi implementada
ainda. O projeto compila do jeito que esta (`go build ./...`).

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
