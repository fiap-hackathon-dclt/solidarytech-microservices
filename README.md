# solidarytech-microservices

Os três microsserviços da **SolidaryTech**, a plataforma que conecta ONGs, doadores e voluntários, no Hackathon da Fase 5 da POSTECH FIAP (DevOps/SRE).

O código de partida é o fornecido pelo curso em [`dougls/hackathon-DCLT`](https://github.com/dougls/hackathon-DCLT). A partir dele, cada serviço vai ganhar Dockerfile, testes, instrumentação e esteira própria; hoje o repositório ainda tem só o código original. A visão geral do projeto, a equipe e o mapa dos repositórios estão no [perfil da organização](https://github.com/fiap-hackathon-dclt).

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

### Pré-requisitos

- Python 3.9+
- Go 1.21+
- PostgreSQL
- AWS CLI configurado, com credenciais válidas, para o DynamoDB e o SQS

### 1. Dependências

**PostgreSQL.** Crie dois bancos independentes e rode o script de cada um:

| Banco | Script |
|---|---|
| `ngo_db` | [`ngo-service/db/init.sql`](ngo-service/db/init.sql) |
| `donation_db` | [`donation-service/db/init.sql`](donation-service/db/init.sql) |

**DynamoDB.** Uma tabela `SolidaryTechVolunteers`, com partition key `volunteer_id` do tipo `String`.

**SQS.** Uma fila Standard. Guarde a URL para as variáveis de ambiente. A fila é opcional: sem ela, o `donation-service` sobe e só não publica eventos.

### 2. Variáveis de ambiente

Crie um `.env` dentro de cada serviço.

`ngo-service/.env`

```env
PORT=8081
DATABASE_URL="postgres://SEU_USUARIO:SUA_SENHA@localhost:5432/ngo_db"
```

`donation-service/.env`

```env
PORT=8082
DATABASE_URL="postgres://SEU_USUARIO:SUA_SENHA@localhost:5432/donation_db"
AWS_REGION="us-east-1"
AWS_SQS_URL="SUA_URL_DA_FILA_SQS"
```

`volunteer-service/.env`

```env
PORT=8083
AWS_REGION="us-east-1"
AWS_DYNAMODB_TABLE="SolidaryTechVolunteers"
```

### 3. Subindo os serviços

Um terminal para cada serviço:

```bash
cd ngo-service
pip install -r requirements.txt
gunicorn --bind 0.0.0.0:8081 app:app
```

```bash
cd donation-service
go mod tidy
go run .
```

```bash
cd volunteer-service
pip install -r requirements.txt
gunicorn --bind 0.0.0.0:8083 app:app
```

| Serviço | URL |
|---|---|
| `ngo-service` | http://localhost:8081 |
| `donation-service` | http://localhost:8082 |
| `volunteer-service` | http://localhost:8083 |
