
# API REST de Comunicação entre Empresas

## Atenção

A arquitetura do projeto tem dois níveis de comunicação:
- Cliente → servidor: o cliente envia mensagens TCP, como buscar_itinerarios.
- Servidor → servidor: o serviço distribuído consulta /v1/trechos por HTTP.

## 1. Objetivo

A API REST permite que uma empresa do VaiJunto disponibilize
informações sobre seus trechos de caronas para outras empresas
participantes do consórcio.

Cada empresa executa sua própria instância do servidor, possui
seu identificador e mantém seu próprio repositório de dados.

A API REST é usada na comunicação entre servidores. A comunicação
entre o cliente do VaiJunto e o servidor ao qual ele se conecta
continua utilizando o protocolo TCP definido no projeto.

## 2. Endereçamento

Cada servidor possui duas interfaces de rede:

- TCP: recebe requisições dos clientes VaiJunto.
- REST: disponibiliza endpoints para comunicação entre empresas.

Configuração de referência:

| Serviço | Porta TCP no container | Porta REST no container |
|---|---:|---:|
| empresa-a | 8080 | 8085 |
| empresa-b | 8080 | 8085 |
| empresa-c | 8080 | 8085 |

As portas podem ser iguais nos diferentes containers porque cada
container possui sua própria rede. Na rede Docker, os servidores
são identificados pelos nomes dos serviços.

Exemplo: http://servidor-b:8085/v1/health

## 3. Identificação da empresa

Cada servidor é configurado por meio da variável de ambiente:

VAIJUNTO_EMPRESA_ID

O identificador deve ser estável e único dentro do consórcio.

## 4. Endpoint de saúde

### Requisição

GET /v1/health

### Resposta de sucesso — HTTP 200

{
  "status": "ok",
  "empresa_id": "empresa-a"
}

O campo `status` indica que o endpoint está respondendo.
O campo `empresa_id` identifica a empresa que respondeu.

## 5. Consulta de trechos

### Requisição

GET /v1/trechos?data=AAAA-MM-DD&passageiros=N

### Parâmetros

- `data`: data da viagem no formato AAAA-MM-DD.
- `passageiros`: quantidade de passageiros, como inteiro positivo.

### Resposta de sucesso — HTTP 200

{
  "empresa_id": "empresa-a",
  "trechos": [
    {
      "trecho": {
        "empresa_id": "empresa-a",
        "id_carona": "carona-123",
        "indice": 0,
        "origem": "Feira de Santana",
        "destino": "Salvador"
      },
      "assentos_disponiveis": 3,
      "preco": 25
    }
  ]
}

A lista contém os trechos locais que atendem aos critérios da
consulta. Uma lista vazia indica que não foram encontrados trechos
correspondentes, e não que houve uma falha na API.

Os campos do exemplo devem corresponder aos DTOs efetivamente
implementados no pacote `internal/protocolo`.

## 6. Tratamento de erros

| Código HTTP | Significado |
|---|---|
| 200 | Requisição concluída com sucesso. |
| 400 | Parâmetros ausentes ou inválidos. |
| 405 | Método HTTP não permitido. |
| 500 | Falha interna inesperada. |

Formato de erro:

{
  "erro": "mensagem explicativa"
}

A mensagem não deve expor senhas, caminhos internos, dados privados
ou detalhes da implementação.

## 7. Configuração dos peers

A variável de ambiente `VAIJUNTO_PEERS` pode definir empresas
remotas no formato:

VAIJUNTO_PEERS=empresa-b=http://servidor-b:8085,empresa-c=http://servidor-c:8085

Dentro da rede Docker, deve-se utilizar o nome do serviço como host,
e não `localhost`, para alcançar outro container.

## 8. Testes

Os testes automatizados devem verificar:

- resposta e identificação do endpoint de saúde;
- consulta válida de trechos;
- rejeição de parâmetros inválidos;
- rejeição de métodos não permitidos;
- tratamento de falhas do repositório;
- formato dos corpos JSON.

Os testes devem utilizar servidores HTTP de teste, sem depender
de portas externas ou de outros containers.

## 9. Limites da API nesta etapa

Os endpoints de preparação, confirmação e aborto de reservas
distribuídas não fazem parte desta versão. A reserva envolvendo
múltiplas empresas exige um protocolo transacional específico,
que será implementado em etapa posterior.
