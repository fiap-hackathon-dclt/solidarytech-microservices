# solidarytech-microservices

Os três microsserviços da **SolidaryTech**, a plataforma que conecta ONGs, doadores e voluntários, no Hackathon da Fase 5 da POSTECH FIAP (DevOps/SRE).

O código de partida é o fornecido pelo curso em [`dougls/hackathon-DCLT`](https://github.com/dougls/hackathon-DCLT). A partir dele, cada serviço ganhou Dockerfile e roda localmente em docker-compose; testes, instrumentação e esteira própria vêm a seguir. A visão geral do projeto, a equipe e o mapa dos repositórios estão no [perfil da organização](https://github.com/fiap-hackathon-dclt).

## Os serviços

| Serviço | Stack | Estado | Porta | Responsabilidade |
|---|---|---|---|---|
| [`ngo-service`](ngo-service/) | Python · Flask | PostgreSQL | `8081` | Cadastro e gestão das ONGs parceiras |
| [`donation-service`](donation-service/) | Go | PostgreSQL · SQS | `8082` | Processamento das doações e publicação de eventos na fila |
| [`volunteer-service`](volunteer-service/) | Python · Flask | DynamoDB | `8083` | Cadastro de voluntários por ONG |

O **`donation-service` é o caminho crítico** da plataforma: é onde estão os SLOs, o error budget e o RPO do plano de continuidade, e é ele que recebe prioridade em toda decisão de arquitetura.

## API

| Serviço | Método e rota | Faz |
|---|---|---|
| todos | `GET /health` | Responde `{"status":"ok"}` com o nome do serviço |
| `ngo-service` | `POST /ngos` | Cadastra uma ONG: `name`, `email`, `cause`, `city`. E-mail repetido responde `409` |
| `ngo-service` | `GET /ngos` | Lista as ONGs |
| `donation-service` | `POST /donations` | Registra uma doação: `ngo_id`, `amount`, `donor_name`. Publica o evento na fila SQS, se configurada |
| `donation-service` | `GET /donations` | Lista as doações |
| `volunteer-service` | `POST /volunteers` | Cadastra um voluntário: `name`, `email`, `ngo_id` |
| `volunteer-service` | `GET /volunteers/<ngo_id>` | Lista os voluntários de uma ONG |

## Como isto chega à produção

Ninguém vai fazer deploy daqui. O caminho previsto é sempre o mesmo:

```
merge na main deste repositório
  → CI do serviço alterado: testes, lint, SAST, SCA, build e scan da imagem
  → imagem publicada no ECR, com o SHA do commit como tag
  → CD grava a nova tag no solidarytech-gitops
  → Argo CD sincroniza o cluster
```

Cada serviço vai ter esteira própria, disparada só quando o diretório dele muda. Os workflows vêm do [`reusable-workflows`](https://github.com/fiap-hackathon-dclt/reusable-workflows).

---

## Executando localmente

Só é preciso Docker. Não há conta na AWS nem configuração manual:

```bash
docker compose up --build
```

| Serviço | URL |
|---|---|
| `ngo-service` | http://localhost:8081 |
| `donation-service` | http://localhost:8082 |
| `volunteer-service` | http://localhost:8083 |

O compose sobe, junto dos três serviços:

| Peça | Papel |
|---|---|
| PostgreSQL | Os bancos `ngo_db` e `donation_db`, criados pelos `db/init.sql` dos serviços |
| DynamoDB Local | Emulador do DynamoDB, em memória, com a tabela `SolidaryTechVolunteers` |
| ElasticMQ | Emulador do SQS, com a fila `solidary-donations` |

Os serviços falam com os emuladores pelas variáveis `AWS_ENDPOINT_URL_SQS` e `AWS_ENDPOINT_URL_DYNAMODB`, que os SDKs da AWS leem sozinhos. Em produção elas não existem, e o mesmo código fala com a AWS. O motivo da escolha está na [ADR 0008](https://github.com/fiap-hackathon-dclt/.github/blob/main/docs/decisoes/0008-ambiente-local-com-emuladores.md).

O ambiente é efêmero: `docker compose down -v` apaga tudo, e a próxima subida começa limpa.

### Inspecionando a fila e a tabela

A AWS CLI roda num contêiner, na rede do compose:

```bash
docker run --rm --network solidarytech_default \
  -e AWS_REGION=us-east-1 -e AWS_ACCESS_KEY_ID=local -e AWS_SECRET_ACCESS_KEY=local \
  amazon/aws-cli:2.42.3 sqs receive-message \
  --endpoint-url http://elasticmq:9324 \
  --queue-url http://elasticmq:9324/000000000000/solidary-donations
```

Para a tabela: `dynamodb scan --table-name SolidaryTechVolunteers --endpoint-url http://dynamodb:8000`.

### Variáveis de ambiente

| Serviço | Variável | Obrigatória |
|---|---|---|
| `ngo-service` | `DATABASE_URL` | sim |
| `donation-service` | `PORT` | não; padrão `8082` |
| `donation-service` | `DATABASE_URL` | sim |
| `donation-service` | `AWS_REGION`, `AWS_SQS_URL` | não; sem as duas, o serviço não publica eventos |
| `volunteer-service` | `AWS_DYNAMODB_TABLE` | sim |
| `volunteer-service` | `AWS_REGION` | não; padrão `us-east-1` |
