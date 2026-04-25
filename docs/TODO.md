# AI Orchestrator V5 — План доработки

> TODO-лист для превращения рабочего прототипа в production-ready систему с облачным деплоем и доступом с локальных машин.

---

## Содержание

1. [Текущее состояние](#1-текущее-состояние)
2. [Приоритет P0 — Минимально работающая система](#2-приоритет-p0---минимально-работающая-система)
3. [Приоритет P1 — Cloud Deployment](#3-приоритет-p1---cloud-deployment)
4. [Приоритет P2 — SDK и клиенты](#4-приоритет-p2---sdk-и-клиенты)
5. [Приоритет P3 — Observability](#5-приоритет-p3---observability)
6. [Приоритет P4 — Интеграции](#6-приоритет-p4---интеграции)
7. [Необходимые компоненты](#7-необходимые-компоненты)

---

## 1. Текущее состояние

| Компонент | Статус |
|-----------|-------|
| Локальный режим | ✅ Работает |
| Распределённый режим (демо) | ⚠️ Симуляция |
| HTTP REST API | ❌ Нет сервера |
| gRPC (реальный) | ❌ Не скомпилирован |
| Circuit Breaker | ❌ Не подключён |
| Visibility Reaper | ❌ Не запускается |
| PostgreSQL | ⚠️ Build tag |
| Мониторинг | ❌ Нет |

---

## 2. Приоритет P0 — Минимально работающая система

Минимальный набор для работы системы без облака (локальная разработка).

### P0.1: Подключить Circuit Breaker

```go
// internal/resilience/circuitbreaker.go уже существует
// Нужно добавить в:
// internal/rpc/rpc.go - оборачивать вызовы в CircuitBreaker
// internal/executor/distributed.go - использовать CB для каждого воркера
```

**Задачи:**
- [ ] Импортировать `resilience.CircuitBreaker` в RPC client
- [ ] Создать мапу `map[string]*CircuitBreaker` для каждого воркера
- [ ] Оборачивать `client.ExecuteTask()` в `cb.Execute()`
- [ ] Обрабатывать `ErrCircuitOpen` — выбирать другой воркер

**Файл:** `internal/rpc/rpc.go`

### P0.2: Запустить Visibility Reaper

```go
// internal/maintenance/reaper.go уже существует
// Нужно добавить в cmd/orchestrator/main.go
```

**Задачи:**
- [ ] Создать `queue.MemoryQueue` с настроенным VisibilityConfig
- [ ] Импортировать `maintenance.VisibilityReaper`
- [ ] Вызвать `reaper.Start(ctx)` при старте
- [ ] Вызвать `reaper.Stop()` при shutdown

**Файл:** `cmd/orchestrator/main.go`

### P0.3: Добавить REAL HTTP сервер

```go
// Пока нет net/http - сервер не существует
```

**Задачи:**
- [ ] Добавить `net/http` или `gin`/`chi` в зависимости
- [ ] Создать обработчики:
  - `GET /health` — health check
  - `POST /v1/tasks` — создать задачу
  - `GET /v1/tasks/{id}` — статус задачи
  - `GET /v1/tasks` — список задач
  - `POST /v1/tasks/{id}/cancel` — отмена
  - `GET /v1/queue` — статус очереди
  - `GET /v1/dlq` — DLQ entries
- [ ] Добавить middleware: API Key аутентификация
- [ ] Запускать сервер ��а порту 8080

**Новые файлы:**
- `internal/server/server.go` — HTTP сервер
- `internal/server/handlers.go` — обработчики
- `internal/server/middleware.go` — middleware

### P0.4: Скомпилировать gRPC

```go
// proto/worker.proto существует, но не скомпилирован
```

**Задачи:**
- [ ] Установить protoc и плагины
- [ ] Запустить генерацию: `protoc --go_out=. --go-grpc_out=. proto/worker.proto`
- [ ] Обновить internal/rpc/rpc.go — использовать сгенерированные stubs
- [ ] Заменить демо-RPC на реальные gRPC вызовы

**Команда:**
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
protoc --go_out=. --go-grpc_out=. proto/worker.proto
```

---

## 3. Приоритет P1 — Cloud Deployment

Деплой в облако.

### P1.1: Docker Compose для production

**Задачи:**
- [ ] Обновить docker-compose.yml:
  - Orchestrator сервис с HTTP портом
  - Worker сервисы
  - Конфиги через environment variables
  - Health checks
  - Volumes для persistency

**Файл:** `deploy/docker-compose.yml`

### P1.2: Kubernetes manifests

**Задачи:**
- [ ] Deployment для orchestrator
- [ ] Deployment для workers
- [ ] Service с LoadBalancer
- [ ] ConfigMap / Secret
- [ ] Readiness/Liveness probes
- [ ] HorizontalPodAutoscaler (опционально)

**Файл:** `deploy/k8s/`

### P1.3: CI/CD pipeline

**Задачи:**
- [ ] GitHub Actions workflow:
  -Lint → Test → Build → Push Docker image
- [ ] Docker build с Multi-arch (amd64, arm64)
- [ ] Теги и версионирование

**Файл:** `.github/workflows/`

### P1.4: Настройка PostgreSQL

**Задачи:**
- [ ] Подключить PostgreSQL store в код (убрать build tag)
- [ ] Добавить миграции БД
- [ ] Конфигурация через environment
- [ ] Connection pooling

**Файл:** `internal/statestore/postgres.go`

---

## 4. Приоритет P2 — SDK и клиенты

Клиентские библиотеки для работы с локальной машины.

### P2.1: Go SDK

**Задачи:**
- [ ] Создать orchestrator-go SDK пакет
- [ ] Методы:
  - `NewClient(url, apiKey)` — создать клиент
  - `SubmitTask(goal, opts)` — отправить задачу
  - `GetTask(id)` — получить статус
  - `ListTasks(opts)` — список задач
  - `CancelTask(id)` — отмена
- [ ] Примеры использования
- [ ] Documentation

**Файл:** `sdk/go/`

### P2.2: Python SDK

**Задачи:**
- [ ] Создать orchestrator-python пакет
- [ ] Методы: аналогично Go SDK
- [ ] Async support
- [ ] Type hints
- [ ] Documentation

**Файл:** `sdk/python/`

### P2.3: CLI инструмент

**Задачи:**
- [ ] CLI для командной строки
- [ ] Команды:
  - `orch run "goal"` — запустить задачу
  - `orch get <id>` — статус
  - `orch list` — список
  - `orch cancel <id>` — отмена
  - `orch queue status` — статус очереди
- [ ] Autocompletion
- [ ] Конфиг файл

**Файл:** `cmd/cli/`

---

## 5. Приоритет P3 — Observability

Мониторинг и debugging.

### P3.1: Prometheus метрики

**Задачи:**
- [ ] Добавить метрики:
  - Task duration (histogram)
  - Task success/failure (counter)
  - Queue size (gauge)
  - Workers active (gauge)
  - Retry count (counter)
- [ ] `/metrics` endpoint в HTTP сервере
- [ ] Grafana dashboard

**Новые файлы:** `internal/metrics/`

### P3.2: Structured logging

**Задачи:**
- [ ] JSON логи в stderr
- [ ] TraceID для каждого запроса
- [ ] Log levels: DEBUG, INFO, WARN, ERROR

### P3.3: OpenTelemetry

**Задачи:**
- [ ] Tracing для каждой задачи
- [ ] Span attributes: task_id, worker_id, agent
- [ ] Export в Jaeger/Zipkin

---

## 6. Приоритет P4 — Интеграции

Реальные интеграции с внешними сервисами.

### P4.1: Real LLM интеграция

**Задачи:**
- [ ] Интерфейс для LLM клиента
- [ ] Поддержка OpenAI / Anthropic / Claude
- [ ] Planner использует реальный LLM
- [ ] Rate limiting
- [ ] Retry с backoff

**Файл:** `internal/planner/llm.go`

### P4.2: Real MCP Tools

**Задачи:**
- [ ] HTTP MCP клиент
- [ ] Tool execution через сеть
- [ ] File I/O — реальный
- [ ] Shell exec — реальный
- [ ] Deploy — интеграция с K8s/Docker

**Файл:** `internal/mcp/client.go`

### P4.3: Redis/Kafka для очереди

**Задачи:**
- [ ] Redis queue вместо in-memory
- [ ] Multi-instance support
- [ ] Kafka events для async

**Файл:** `internal/queue/redis.go`

---

## 7. Необходимые компоненты

### Структура после доработок

```
AI-Orchestrator/
├── cmd/
│   ├── orchestrator/          # Main (оставить)
│   ├── worker/               # Worker (оставить)
│   ├── server/               # HTTP server (NEW)
│   └── cli/                 # CLI tool (NEW)
├── internal/
│   ├── server/              # HTTP handlers (NEW)
│   ├── metrics/             # Prometheus (NEW)
│   ├── tracing/             # OpenTelemetry (NEW)
│   ├── queue/
│   │   ├── memory.go        # Оставить
│   │   └── redis.go         # NEW
│   ├── mcp/
│   │   ├── registry.go       # Оставить
│   │   └── client.go       # NEW
│   ├── planner/
│   │   ├── planner.go      # Оставить
│   │   └── llm.go       # NEW
│   ├── statestore/
│   │   ├── memory.go      # Оставить
│   │   └── postgres.go  # Без build tag
│   ├── rpc/
│   │   └── rpc.go        # Обновить на real gRPC
│   └── ...остальное как есть
├── proto/
│   └── worker.proto     # Скомпилировать
├── sdk/
│   ├── go/            # NEW
│   └── python/         # NEW
├── deploy/
│   ├── docker-compose.yml  # NEW
│   └── k8s/           # NEW
└── docs/
```

---

## Приоритеты суммарно

| Приоритет | Задач | Описание |
|----------|------|----------|
| **P0** | 4 | Минимально рабочая система |
| **P1** | 4 | Cloud deployment |
| **P2** | 3 | SDK и клиенты |
| **P3** | 3 | Observability |
| **P4** | 3 | Интеграции |
| **Итого** | **17** | |

---

*TODO list created: 2025-04-25*
*Status: Working prototype → Production-ready*