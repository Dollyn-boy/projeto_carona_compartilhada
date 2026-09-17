# Protocolo de aplicação — VaiJunto

> Especificação completa do protocolo entre cliente e servidor, conforme exigido pelo enunciado.  
> Preencha cada seção conforme for definindo e implementando o protocolo (ver `internal/protocolo`).

## 1. Envelope da mensagem

Todas as mensagens enviadas e recebidas pelo servidor utilizam o formato JSON. O envelope padroniza os campos necessários para identificar a operação e mapear a resposta ao respectivo pedido do cliente.

### Requisição (Cliente → Servidor)

- `tipo` (string): Identifica a operação desejada (ex.: `login`, `publicar_carona`).
- `id_requisicao` (string): Identificador único gerado pelo cliente para parear a requisição com a resposta assíncrona.
- `dados` (objeto JSON): Payload específico da operação. Pode ser omitido se a operação não exigir parâmetros.

### Resposta (Servidor → Cliente)

- `status` (string): Indica o resultado. Pode ser `ok` para sucesso ou `erro` para falha.
- `id_requisicao` (string): O mesmo ID recebido na requisição correspondente.
- `dados` (objeto JSON, opcional): Payload de retorno em caso de sucesso.
- `motivo` (string, opcional): Mensagem descritiva enviada apenas quando `status` é `erro`.

Mensagens malformadas (JSON inválido) são capturadas na desserialização (`json.Unmarshal`) no nível de protocolo, retornando imediatamente um envelope de erro informando a falha no parse, sem chegar à camada de casos de uso.

## 2. Framing sobre TCP

A estratégia adotada é de **TCP puro com delimitador de mensagem**, gerenciado pelo `bufio.Reader` e pelas funções encapsuladas no pacote `protocolo` (`LerMensagem` e `EscreverMensagem`).

As mensagens JSON são enviadas na íntegra, sendo o delimitador gerenciado pelo `json.Decoder`, que compreende os limites dos objetos JSON no fluxo contínuo (*stream*).

Essa abordagem foi escolhida por ser nativa do Go, simplificar o *parsing* diretamente da rede para a `struct` e não requerer o cálculo manual de prefixos de tamanho fixo no cabeçalho.

## 3. Fluxo de conexão e desconexão

- **Conexão:** O protocolo baseia-se em conexões TCP persistentes (*stateful*). Assim que o cliente se conecta, o servidor cria uma goroutine dedicada e inicializa uma `Sessao` em memória (`Autenticado: false`). Nenhuma operação privada pode ser feita neste estado. O cliente deve enviar uma requisição `cadastro` (opcional) seguida de `login`.

- **Autenticação:** O comando `login` atualiza o estado da `Sessao` na memória do servidor, vinculando o ID do usuário àquela conexão TCP específica. A partir de então, o cliente não envia mais seu identificador no payload; o servidor confia exclusivamente na identidade registrada na sessão da conexão.

- **Desconexão/Queda:** Se o cliente fechar a conexão, a rede cair ou o limite de inatividade da sessão expirar, o loop de leitura daquela goroutine é encerrado. A memória alocada para a `Sessao` daquele socket é liberada automaticamente. Não há estado "fantasma" mantido após a queda.

## 4. Operações

### `login`

**Requisição:** `email` e `senha`.

**Resposta:** Mensagem de boas-vindas.

**Exemplo:**

```json
// Envio
{
  "tipo": "login",
  "id_requisicao": "1",
  "dados": {
    "email": "joao",
    "senha": "123"
  }
}

// Resposta
{
  "status": "ok",
  "id_requisicao": "1",
  "dados": {
    "mensagem": "bem-vindo, joao"
  }
}
```

### `publicar_carona`

**Requisição:** `rota`, `capacidade`, `preco` e `data`. O motorista é extraído da sessão.

**Resposta:** Estrutura da carona recém-criada, contendo o ID gerado pelo servidor.

**Exemplo:**

```json
// Envio
{
  "tipo": "publicar_carona",
  "id_requisicao": "2",
  "dados": {
    "rota": ["Salvador", "Feira"],
    "capacidade": 4,
    "preco": 30,
    "data": "2026-09-20"
  }
}

// Resposta
{
  "status": "ok",
  "id_requisicao": "2",
  "dados": {
    "id": 1,
    "motorista": "joao",
    "rota": ["Salvador", "Feira"],
    "capacidade": 4,
    "preco": 30,
    "data": "2026-09-20"
  }
}
```

### `consultar_caronas`

**Requisição:** Payload vazio. O motorista é extraído da sessão segura.

**Resposta:** Lista das caronas publicadas pelo usuário logado, incluindo a ocupação atual (`passageiros_por_trecho`).

**Exemplo:**

```json
// Envio
{
  "tipo": "consultar_caronas",
  "id_requisicao": "3",
  "dados": {}
}

// Resposta
{
  "status": "ok",
  "id_requisicao": "3",
  "dados": {
    "caronas": [
      {
        "id": 1,
        "motorista": "joao",
        "rota": ["Salvador", "Feira"],
        "capacidade": 4,
        "preco": 30,
        "data": "2026-09-20",
        "passageiros_por_trecho": [
          ["maria", "pedro"]
        ]
      }
    ]
  }
}
```

### `cancelar_carona`

**Requisição:** `id_carona`. O usuário é validado no repositório para evitar exclusões indevidas.

**Resposta:** Mensagem de confirmação. O servidor cancela em cascata as reservas afetadas.

**Exemplo:**

```json
// Envio
{
  "tipo": "cancelar_carona",
  "id_requisicao": "4",
  "dados": {
    "id_carona": 1
  }
}

// Resposta
{
  "status": "ok",
  "id_requisicao": "4",
  "dados": {
    "mensagem": "carona cancelada"
  }
}
```

### `buscar_itinerarios`

**Requisição:** `origem`, `destino`, `data` e `num_passageiros`.

**Resposta:** Lista de itinerários disponíveis. Cada itinerário contém uma lista de trechos ordenados.

**Exemplo:**

```json
// Envio
{
  "tipo": "buscar_itinerarios",
  "id_requisicao": "5",
  "dados": {
    "origem": "Salvador",
    "destino": "Feira",
    "data": "2026-09-20",
    "num_passageiros": 1
  }
}

// Resposta
{
  "status": "ok",
  "id_requisicao": "5",
  "dados": {
    "itinerarios": [
      {
        "trechos": [
          {
            "id_carona": "1",
            "indice": 0,
            "origem": "Salvador",
            "destino": "Feira"
          }
        ]
      }
    ]
  }
}
```

### `confirmar_reserva`

**Requisição:** `trechos` (matriz reenviada da busca). O passageiro é extraído da sessão.

**Resposta:** Estrutura da reserva confirmada com ID gerado.

**Exemplo:**

```json
// Envio
{
  "tipo": "confirmar_reserva",
  "id_requisicao": "6",
  "dados": {
    "trechos": [
      {
        "id_carona": "1",
        "indice": 0,
        "origem": "Salvador",
        "destino": "Feira"
      }
    ]
  }
}

// Resposta
{
  "status": "ok",
  "id_requisicao": "6",
  "dados": {
    "id": 101,
    "passageiro": "maria",
    "trechos": [
      {
        "id_carona": "1",
        "indice": 0,
        "origem": "Salvador",
        "destino": "Feira"
      }
    ]
  }
}
```

### `consultar_reservas`

**Requisição:** Payload vazio. O passageiro é extraído da sessão.

**Resposta:** Lista das reservas efetuadas pelo usuário.

**Exemplo:**

```json
// Envio
{
  "tipo": "consultar_reservas",
  "id_requisicao": "7",
  "dados": {}
}

// Resposta
{
  "status": "ok",
  "id_requisicao": "7",
  "dados": {
    "reservas": [
      {
        "id": 101,
        "passageiro": "maria",
        "trechos": [
          {
            "id_carona": "1",
            "indice": 0,
            "origem": "Salvador",
            "destino": "Feira"
          }
        ]
      }
    ]
  }
}
```

### `cancelar_reserva`

**Requisição:** `id_reserva`. O usuário é validado no repositório.

**Resposta:** Mensagem de confirmação e devolução atômica dos assentos aos respectivos contadores.

**Exemplo:**

```json
// Envio
{
  "tipo": "cancelar_reserva",
  "id_requisicao": "8",
  "dados": {
    "id_reserva": 101
  }
}

// Resposta
{
  "status": "ok",
  "id_requisicao": "8",
  "dados": {
    "mensagem": "reserva cancelada"
  }
}
```

## 5. Códigos de erro

Erros não possuem códigos numéricos complexos. Eles utilizam a chave `status: "erro"` no envelope, combinada com uma mensagem descritiva no campo `motivo`.

Os principais cenários de erro incluem:

### Acesso negado

Ocorre quando uma operação privada é executada sem autenticação ou quando o usuário tenta acessar/cancelar um recurso pertencente a outro usuário.

Exemplos:

```text
Acesso negado: faça login primeiro
acesso negado: voce nao tem permissao...
```

### Dados inválidos

Ocorre quando há erro de *parse* do JSON ou quando os campos obrigatórios estão ausentes ou apresentam formato incorreto.

Formato:

```text
dados invalidos: [detalhe]
```

### Sessão expirada

Ocorre quando a sessão expira por inatividade.

Mensagem:

```text
Sessão expirada por inatividade. Faça login novamente.
```

### Regra de negócio violada

São erros relacionados às regras do domínio, com mensagens descritivas repassadas pelo pacote `estado`.

Exemplos:

```text
carona nao encontrada
sem assento disponivel no trecho X
usuario ja cadastrado
```
