# VaiJunto — Sistema de Caronas Compartilhadas

Projeto da disciplina TEC502 (Concorrencia e Conectividade). Servidor central
com clientes de motorista e passageiro, comunicando-se por sockets TCP com
protocolo de aplicacao proprio.

## Estrutura do projeto

```
cmd/                     pontos de entrada (binarios)
  servidor/               bootstrap do servidor central
  cliente-motorista/      bootstrap do cliente motorista
  cliente-passageiro/     bootstrap do cliente passageiro

internal/                codigo interno ao modulo (nao importavel de fora)
  dominio/                caronas, trechos, reservas, busca de itinerarios
  concorrencia/           controle de concorrencia sobre o estado do dominio
  protocolo/              formato das mensagens e framing sobre TCP
  servidor/                dispatcher de conexoes + handlers das operacoes
  clientenet/             camada de rede compartilhada pelos dois clientes

test/carga/               harness de teste automatizado de concorrencia

docker/                   Dockerfiles do servidor e dos clientes
docs/protocolo.md         especificacao do protocolo de aplicacao
```

## Como usar este esqueleto

Todos os arquivos em `internal/`, `cmd/`, `test/` e `docker/` contem apenas
comentarios de instrucao (TODO) — nenhuma logica foi implementada ainda.
O projeto compila do jeito que esta (`go build ./...`), o que serve como
ponto de partida limpo.

Ordem sugerida de implementacao (a mesma discutida no planejamento):

1. `internal/dominio` — modelar caronas, trechos e reservas em memoria, sem rede.
2. `internal/concorrencia` — implementar e validar o controle de concorrencia
   sobre o dominio, com testes concorrentes (`go test -race`).
3. `docs/protocolo.md` + `internal/protocolo` — especificar e implementar o
   formato das mensagens e o framing sobre TCP.
4. `internal/servidor` — camada de rede do servidor (dispatcher + handlers),
   ligando o protocolo ao dominio ja validado.
5. `internal/clientenet` + `cmd/cliente-motorista` + `cmd/cliente-passageiro`
   — clientes de terminal.
6. `test/carga` — teste automatizado de concorrencia contra o servidor real.
7. `docker/` + `docker-compose.yml` — conteinerizacao e testes entre maquinas.

## Build

```
go build ./...
go vet ./...
go test -race ./...
```
