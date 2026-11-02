# QUEM VAI BORA — Sistema de Caronas Compartilhadas

Projeto da disciplina TEC502 (Concorrencia e Conectividade). Sistema de
caronas compartilhadas com empresas independentes, cliente TCP e comunicacao
HTTP/REST entre servidores para busca e coordenacao de reservas distribuidas.
Cada empresa mantem seu proprio repositorio; a empresa a que o passageiro esta
conectado coordena somente aquela transacao, sem coordenador central permanente.

## Arquitetura em camadas do servidor

Camadas locais do servidor e componentes de integracao distribuida:

  1. Rede TCP                  -> internal/rede
  2. Protocolo de mensagens    -> internal/protocolo
  3. Casos de uso              -> internal/casosdeuso
  4. Dominio + concorrencia    -> internal/dominio + internal/concorrencia
  5. Estado/persistencia       -> internal/estado
  6. API entre empresas        -> internal/api (HTTP/REST)
  7. Busca/coordenacao         -> internal/consorcio

A API REST e o pacote consorcio integram instancias independentes. Cada
empresa continua sendo a unica autoridade sobre seu proprio repositorio.

## Estrutura do projeto

```
cmd/                       pontos de entrada (binarios)
  servidor/                  bootstrap de uma instancia/empresa
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
  estado/                     camada 5 — repositorio, mapas em memoria,
                             transacoes locais e logs de coordenacao
  api/                        endpoints HTTP/REST para comunicacao entre empresas
  consorcio/                  busca distribuida e coordenador temporario por reserva
  clientenet/                 camada de rede do lado do cliente

test/carga/                  harness de teste automatizado de concorrencia
                             (multiplos clientes reais via rede, disputando
                             os mesmos trechos)
docker/                      Dockerfiles do servidor e do cliente
protocolo.md                 especificacao TCP entre cliente e servidor
docs/api-consorcio.md        contrato HTTP/REST entre empresas
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

## Rodando o consorcio com Docker Compose

O `docker-compose.yml` inicia tres empresas independentes (`servidor-a`,
`servidor-b` e `servidor-c`) e um cliente conectado inicialmente à empresa A.
Cada empresa possui seu proprio volume JSON e sua identidade (`VAIJUNTO_EMPRESA_ID`).
As empresas se comunicam pela API REST na porta 8085 interna; clientes usam TCP
na porta 8080 interna.

```bash
docker compose config
docker compose up --build -d
docker compose ps
docker compose logs -f servidor-a servidor-b servidor-c
```

Para iniciar um cliente interativo:

```bash
docker compose run --rm cliente
```

Para conectar um cliente à empresa B ou C:

```bash
docker compose run --rm cliente -servidor servidor-b:8080
docker compose run --rm cliente -servidor servidor-c:8080
```

As portas TCP publicadas no host são 8080 (A), 8081 (B) e 8082 (C). A API REST
usa 8085 dentro da rede Docker. Para testar REST a partir do host, publique
portas diferentes no host, por exemplo `8085:8085`, `8086:8085` e `8087:8085`.

Os volumes `vaijunto-a-dados`, `vaijunto-b-dados`, `vaijunto-c-dados` e
`vaijunto-cliente-dados` preservam os estados dos servidores e as chaves de
idempotência pendentes do cliente. Evite `docker compose down -v` se quiser
preservar os dados, pois `-v` remove os volumes nomeados.

## Variáveis de ambiente importantes

| Variável | Função |
|---|---|
| `VAIJUNTO_EMPRESA_ID` | Identidade estável da empresa. |
| `VAIJUNTO_ENDERECO` | Endereço TCP, normalmente `:8080`. |
| `VAIJUNTO_REST_ENDERECO` | Endereço HTTP, normalmente `:8085`. |
| `VAIJUNTO_ARQUIVO_DADOS` | Caminho do JSON persistente da empresa. |
| `VAIJUNTO_PEERS` | Lista `empresa=url` dos servidores autorizados/conhecidos. |
| `VAIJUNTO_CLIENTE_ESTADO` | Arquivo local do cliente com chaves de idempotência pendentes. |

Exemplo de peers dentro do Docker:

```text
VAIJUNTO_PEERS=empresa-b=http://servidor-b:8085,empresa-c=http://servidor-c:8085
```

Dentro de um container, use o nome DNS do serviço, não `localhost`, para
acessar outro servidor. `localhost` sempre aponta para o próprio container.

## Fluxo de reserva distribuída

1. O cliente envia `confirmar_reserva` por TCP ao servidor ao qual está conectado.
2. Esse servidor gera/persiste um log coordenado e particiona o itinerário por `EmpresaID`.
3. Cada participante recebe `POST /v1/transacoes/{id}/preparar` e reserva temporariamente seus assentos.
4. A coordenadora persiste `CONFIRMAR` ou `ABORTAR` antes de distribuir a decisão.
5. Os participantes executam a decisão localmente e respondem com ACK.
6. Se a resposta for perdida, as operações podem ser repetidas com o mesmo ID sem duplicar efeitos.
7. Uma resposta `PENDENTE_CONFIRMAR` ou `PENDENTE_ABORTAR` não é sucesso final. A recuperação automática tenta distribuir novamente a decisão persistida.

A chave de idempotência da solicitação iniciada pelo cliente é persistida no
volume do cliente. Os logs de coordenação ficam apenas no JSON da empresa que
coordenou a transação; cada participante guarda sua própria transação local.

## Limitações conhecidas

- O cancelamento de um itinerário distribuído é bloqueado nesta versão para
  evitar cancelar apenas uma parte da viagem. Cancelamento global requer um
  protocolo distribuído próprio.
- A consulta global de partes de uma reserva é oferecida pelo servidor que
  coordenou a transação e preserva o log correspondente.
- O cabeçalho `X-VaiJunto-Empresa` identifica o peer declarado, mas não é
  autenticação criptográfica. Em rede não confiável, use TLS mútuo/credenciais.
- A recuperação pressupõe que a coordenadora volta a ficar disponível e que
  seu volume persistente foi preservado. Perda permanente do disco do
  coordenador exigiria replicação do log/decisão, fora do escopo atual.
- O protocolo não elimina o bloqueio de participantes PREPARADA quando a
  coordenadora fica permanentemente inacessível antes de registrar uma decisão.

## Testes locais

```bash
gofmt -w ./cmd ./internal ./test
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
```

Os testes automatizados utilizam servidores HTTP de teste (`httptest`) e não
substituem a validação da rede Docker ou de máquinas físicas separadas.

