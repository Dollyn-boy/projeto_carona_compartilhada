# QUEM VAI BORA — Sistema de Caronas Compartilhadas

Projeto da disciplina **TEC502 — Concorrência e Conectividade**. Sistema de
caronas compartilhadas formado por empresas independentes, clientes TCP e
comunicação HTTP/REST entre servidores.

Cada empresa mantém seu próprio repositório. O servidor ao qual o passageiro
está conectado coordena as reservas que envolvem mais de uma empresa; não há
um coordenador central permanente.

## Arquitetura

O servidor está organizado em camadas:

```text
Cliente
  |
  | TCP / JSON
  v
internal/rede
  |
  v
internal/protocolo
  |
  v
internal/casosdeuso
  |
  v
internal/estado
  |                 \
  v                  v
internal/dominio   internal/concorrencia

Integração entre empresas:
internal/consorcio <---- HTTP/REST ----> internal/api
```

- **`rede`**: aceita conexões TCP e distribui requisições por uma fila de workers.
- **`protocolo`**: define as mensagens e o framing TCP.
- **`casosdeuso`**: autentica sessões e direciona as operações.
- **`dominio`**: contém as regras de negócio de usuários, caronas, trechos,
  reservas e busca de itinerários.
- **`concorrencia`**: controla o acesso simultâneo aos trechos por meio de travas.
- **`estado`**: mantém o repositório, a persistência JSON e as transações locais.
- **`api`**: disponibiliza os endpoints REST usados pelas empresas.
- **`consorcio`**: realiza a busca distribuída e coordena transações de reserva.

### Estrutura principal

```text
cmd/
  servidor/       inicialização de uma empresa
  cliente/        aplicação cliente TCP

internal/
  rede/
  protocolo/
  casosdeuso/
  dominio/
  concorrencia/
  estado/
  api/
  consorcio/
  clientenet/

docker/           Dockerfiles
docs/             documentação da API entre empresas
test/carga/       testes de concorrência e carga
protocolo.md      protocolo TCP cliente-servidor
docker-compose.yml
```

## Executar as três empresas na mesma máquina

Requisitos: Docker e Docker Compose.

Na raiz do projeto:

```bash
docker compose config
docker compose up --build -d
docker compose ps
```

O Compose inicia as empresas A, B e C e o cliente, conforme definido em
`docker-compose.yml`. Cada servidor deve ter um `VAIJUNTO_EMPRESA_ID` próprio,
um arquivo/volume de dados independente e os outros dois servidores configurados
em `VAIJUNTO_PEERS`.

Dentro da rede Docker, os peers devem usar os nomes dos serviços:

```text
A: empresa-b=http://servidor-b:8085,empresa-c=http://servidor-c:8085
B: empresa-a=http://servidor-a:8085,empresa-c=http://servidor-c:8085
C: empresa-a=http://servidor-a:8085,empresa-b=http://servidor-b:8085
```

Para abrir clientes conectados a cada empresa:

```bash
docker compose run --rm cliente
docker compose run --rm cliente -servidor servidor-b:8080
docker compose run --rm cliente -servidor servidor-c:8080
```

A porta TCP interna é `8080` e a REST interna é `8085`. No host, o Compose
publica as portas TCP `8080` (A), `8081` (B) e `8082` (C). A flag `-servidor`
deve corresponder à opção definida em `cmd/cliente`.

Acompanhar os servidores:

```bash
docker compose logs -f servidor-a servidor-b servidor-c
```

## Executar as três empresas em máquinas diferentes

Cada computador executa **somente uma empresa**. As máquinas devem ter
conectividade IP entre si e executar a mesma versão do projeto.

1. Descubra o IPv4 de cada computador (`ipconfig` no Windows).
2. Publique as portas `8080` (TCP) e `8085` (REST) no computador de cada empresa.
3. Configure `VAIJUNTO_EMPRESA_ID` com a identidade da empresa e `VAIJUNTO_PEERS`
   com os IPs das outras duas máquinas.
4. Libere as portas TCP `8080` e `8085` no firewall da rede local.
5. Inicie, em cada computador, apenas o serviço correspondente à empresa.

Exemplo de endereços:

| Máquina | Empresa | IP |
|---|---|---|
| 1 | A | `192.168.1.10` |
| 2 | B | `192.168.1.11` |
| 3 | C | `192.168.1.12` |

Exemplo de `VAIJUNTO_PEERS`:

```text
A: empresa-b=http://192.168.1.11:8085,empresa-c=http://192.168.1.12:8085
B: empresa-a=http://192.168.1.10:8085,empresa-c=http://192.168.1.12:8085
C: empresa-a=http://192.168.1.10:8085,empresa-b=http://192.168.1.11:8085
```

Substitua os IPs pelos endereços reais. Diferentemente do modo em uma única
máquina, **não use os nomes `servidor-a`, `servidor-b` e `servidor-c`**:
eles são nomes DNS internos do Docker. Configure as variáveis no serviço
correspondente do `docker-compose.yml` (`environment:` ou `env_file:`).

Em cada computador, inicie apenas seu servidor:

```bash
# Máquina A
docker compose up --build -d servidor-a

# Máquina B
docker compose up --build -d servidor-b

# Máquina C
docker compose up --build -d servidor-c
```

O cliente pode ser executado em qualquer máquina com acesso ao servidor:

```bash
go run ./cmd/cliente -servidor 192.168.1.10:8080
```

Troque o IP para conectar-se a B ou C. Confirme o nome da flag em
`cmd/cliente`, caso necessário.

## Configuração dos servidores

| Variável | Finalidade |
|---|---|
| `VAIJUNTO_EMPRESA_ID` | Identificador único e estável da empresa. |
| `VAIJUNTO_ENDERECO` | Endereço TCP; normalmente `:8080`. |
| `VAIJUNTO_REST_ENDERECO` | Endereço HTTP/REST; normalmente `:8085`. |
| `VAIJUNTO_ARQUIVO_DADOS` | Caminho do JSON persistente da empresa. |
| `VAIJUNTO_PEERS` | Lista `empresa=url` dos outros servidores. |

Cada empresa deve usar um arquivo/volume de dados próprio. No Docker, use
o nome DNS do serviço para peers na mesma rede Compose; entre máquinas,
use o endereço IP da máquina remota.

## Testes automatizados

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
```

## Observações

- Os volumes nomeados preservam os dados. `docker compose down` remove os
  containers, mas preserva os volumes; `docker compose down -v` também os apaga.
- Uma reserva distribuída em estado `PENDENTE_CONFIRMAR` ou `PENDENTE_ABORTAR`
  ainda não terminou em todos os participantes.
- O cancelamento de itinerários distribuídos não está implementado nesta versão.
- A recuperação depende do retorno da coordenadora e da preservação de seu
  volume de dados.
- A comunicação entre empresas não possui autenticação criptográfica adequada;
  execute os testes somente em rede confiável, não exponha as portas à Internet.
