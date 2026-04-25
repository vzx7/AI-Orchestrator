# AI Orchestrator V5 — Руководство по развёртыванию

Подробное руководство по развёртыванию AI Orchestrator V5 в средах разработки и продакшена.

> **Текущий статус:** Рабочий прототип. Основные компоненты работают. HTTP API и настоящий gRPC не реализованы. Circuit Breaker и Visibility Reaper есть в коде, но не подключены.

## Содержание

1. [Обзор системы](#обзор-системы)
2. [Требования](#требования)
3. [Быстрый старт](#быстрый-старт)
4. [Локальная разработка](#локальная-разработка)
5. [Распределённый режим](#распределённый-режим)
6. [Новые функции V5](#новые-функции-v5)
7. [Интеграция MCP сервера](#интеграция-mcp-сервера)
8. [Конфигурация](#конфигурация)
9. [Настройка PostgreSQL](#настройка-postgresql)
10. [Продакшен развёртывание](#продакшен-развёртывание)
11. [Устранение проблем](#устранение-проблем)

---

## Обзор системы

### Диаграмма архитектуры

```
┌─────────────────────────────────────────────────────────────┐
│                        Orchestrator                          │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │ Controller  │  │   Planner    │  │  DistributedExec  │   │
│  │ (Loop Prot) │  │  (DAG Gen)   │  │                  │   │
│  └─────────────┘  └──────────────┘  └──────────────────┘   │
│         │                │                   │              │
│  ┌──────▼────────────────▼──────────────────▼────────┐    │
│  │  ┌────────┐ ┌─────────┐ ┌──────┐ ┌──────────────┐ │    │
│  │  │ Queue  │ │Idempotency│ │ DLQ │ │State Store │ │    │
│  │  │(A/N)  │ │ Store   │ │     │ │(PostgreSQL) │ │    │
│  │  └────────┘ └─────────┘ └──────┘ └──────────────┘ │    │
│  └────────────────────────────────────────────────────┘    │
└────────────────────────────┬────────────────────────────────┘
                             │ gRPC
         ┌───────────────────┼───────────────────┐
         ▼                   ▼                   ▼
    ┌─────────┐         ┌─────────┐         ┌─────────┐
    │Worker-1 │         │Worker-2 │         │Worker-N │
    │(Agents) │         │(Agents) │         │(Agents) │
    └────┬────┘         └────┬────┘         └────┬────┘
         │                   │                   │
         ▼                   ▼                   ▼
    ┌─────────────────────────────────────────────────┐
    │              MCP Tool Gateway                      │
    │  (file I/O, shell, deploy, tests, etc.)         │
    └─────────────────────────────────────────────────┘
```

### Ключевые компоненты

| Компонент | Пакет | Назначение |
|-----------|-------|------------|
| **Надёжная очередь** | `internal/queue` | Ack/Nack семантика, отслеживание in-flight |
| **Visibility Timeout** | `internal/queue` | V5: Авто-восстановление зависших задач |
| **Очередь мёртвых писем** | `internal/dlq` | Захват исчерпавших задачи |
| **Магазин идемпотентности** | `internal/idempotency` | Безопасные повторы — задача = один раз |
| **Политика повторов** | `internal/retry` | V5: Экспоненциальная задержка + jitter |
| **Circuit Breaker** | `internal/resilience` | V5: Изоляция сбоев |
| **Хранилище состояний** | `internal/statestore` | V5: Валидация переходов состояний |
| **Усиленный Worker** | `internal/worker` | Восстановление после паники, таймауты |
| **Реестр Workers** | `internal/registry` | V5: Балансировка с учётом задержки |
| **Visibility Reaper** | `internal/maintenance` | V5: Фоновый сборщик задач |

---

## Требования

- **Go 1.26.1** или новее
- **Docker** (для продакшена)
- **PostgreSQL 16** (опционально, для постоянного хранения)
- **Git**

### Установка Go

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install golang-go

# Arch Linux / Manjaro
sudo pacman -S go

# macOS
brew install go

# Проверка
go version
```

### Установка Docker

```bash
# Ubuntu/Debian
sudo apt install docker.io docker-compose
sudo systemctl start docker
sudo usermod -aG docker $USER

# Arch Linux / Manjaro
sudo pacman -S docker docker-compose
sudo systemctl start docker
sudo usermod -aG docker $USER

# macOS
brew install --cask docker
```

---

## Быстрый старт

Запустите систему за 5 минут:

```bash
# 1. Клонируйте репозиторий
git clone <repo>
cd AI-Orchestrator

# 2. Загрузите зависимости
go mod download

# 3. Запустите в локальном режиме
go run ./cmd/orchestrator/

# Ожидаемый вывод:
# INFO: AI Orchestrator V5 — Local Mode
# INFO: === Execution Results ===
# INFO: Result 1: task_id=xxx, status=SUCCESS
# INFO: Result 2: task_id=xxx, status=SUCCESS
# INFO: V5 Demo Complete
```

---

## Локальная разработка

### Запуск в локальном режиме

Все компоненты работают в одном процессе (без сети):

```bash
go run ./cmd/orchestrator/
```

### Запуск в распределённом режиме (демо)

Симулирует несколько workers в одном процессе:

```bash
go run ./cmd/orchestrator/ --distributed
```

Демо показывает:
- **Временный сбой** — Worker-2 падает при первой попытке
- **Автоматический повтор** — Задача перенаправляется на Worker-1
- **Наименее загруженный балансировщик** — Задачи распределяются по workers
- **Отслеживание состояния** — Все задачи отмечены как "done"

### Запуск тестов

```bash
# Все тесты
go test ./... -v

# Один тест
go test ./cmd/orchestrator -run TestDAGExecution -v

# Тест по имени
go test ./... -run "^TestEvaluator$" -v

# С покрытием
go test ./... -cover
```

### Форматирование кода

```bash
# Форматировать весь код
gofmt -w .

# Организовать импорты
goimports -w .
```

---

## Распределённый режим

### Архитектура

В распределённом режиме система состоит из:

1. **Orchestrator** — Центральный координатор (1 экземпляр)
2. **Workers** — Исполнители задач (1-N экземпляров)
3. **Коммуникация** — gRPC между компонентами

### Запуск отдельных Workers

Worker 1:
```bash
go run ./cmd/worker --id=worker-1 --addr=localhost:50051
```

Worker 2 (отдельный терминал):
```bash
go run ./cmd/worker --id=worker-2 --addr=localhost:50052
```

### Подключение Orchestrator к Workers

```bash
go run ./cmd/orchestrator/ --distributed
```

### Наименее загруженный балансировщик

Workers выбираются на основе:
1. **Статус здоровья** — Только здоровые workers (heartbeat OK)
2. **Активные задачи** — Worker с наименьшим количеством задач
3. **Ёмкость** — Максимум параллельных задач на worker

### Мониторинг здоровья

Workers отправляют heartbeat каждые 10 секунд. Если нет heartbeat 30 секунд — worker помечается как нездоровый.

---

## Новые функции V5

### Circuit Breaker

Предотвращает каскадные сбои, отклоняя вызовы к падающим workers:

```go
import "ai_orchestrator/internal/resilience"

cb := resilience.NewCircuitBreaker(resilience.DefaultConfig())

err := cb.Execute(func() error {
    return worker.ExecuteTask(ctx, task)
})

if errors.Is(err, resilience.ErrCircuitOpen) {
    // Worker недоступен, попробовать другой worker
}
```

Состояния: `closed` → `open` (после 5 сбоев) → `half-open` (после 30с) → `closed`

### Visibility Timeout Reaper

Автоматически восстанавливает задачи в "processing" когда workers крашатся:

```go
import "ai_orchestrator/internal/maintenance"
import "ai_orchestrator/internal/queue"

q := queue.NewMemoryQueue(100, queue.BackpressureBlock)
q.SetVisibilityConfig(queue.VisibilityConfig{
    Timeout:       60 * time.Second,
    CheckInterval: 10 * time.Second,
})

reaper := maintenance.NewVisibilityReaper(logger, q, maintenance.DefaultReaperConfig())
ctx, cancel := context.WithCancel(context.Background())
reaper.Start(ctx)

// При завершении
reaper.Stop()
defer cancel()
```

### Повторы с Jitter

Предотвращает "thundering herd" на повторах:

```go
policy := retry.DefaultPolicy()
// Jitter: true (по умолчанию), JitterFactor: 0.3 (±30% рандомизация)

for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
    delay := policy.NextDelay(attempt)
    time.Sleep(delay)
    // Повтор...
}
```

### Валидация переходов состояний

Принудительно выполняет допустимые переходы состояний:

```go
import "ai_orchestrator/internal/statestore"

// Допустимые: pending → running → done
// Недопустимые: pending → done (должен пройти через running)

if !statestore.IsValidTransition(oldState, newState) {
    return statestore.ErrInvalidTransition
}
```

### Балансировка с учётом задержки

Выбирает workers на основе ёмкости и задержки:

```go
worker, err := registry.Next() // Выбирает worker с:
// 1. Healthy (heartbeat OK)
// 2. Активные задачи < ёмкости
// 3. Наименьшее количество активных задач (при равных - наименьшая задержка)
```

### Контекст с оценкой релевантности

Получает наиболее релевантный контекст:

```go
cm.AddMemory("deployment-result", result, 0.9) // важность 0.9

relevant := cm.RetrieveRelevant(5) // Топ-5 по оценке
// Оценка = 60% свежесть + 40% важность
```

### Пределы безопасности

V5 принудительно устанавливает пределы безопасности для предотвращения бесконечных циклов:

```go
const (
    MaxRetriesPerTask   = 10
    MaxExecutionTime    = 10 * time.Minute
    MaxTasksPerWorkflow = 100
    MaxReplans          = 5
)
```

---

## Интеграция MCP сервера

### Текущее состояние: Mock-реализация

Текущая реализация MCP (`internal/mcp/registry.go`) — это **mock**. Она симулирует поведение инструментов без реальных сетевых вызовов.

### Доступные Mock-инструменты

| Инструмент | Назначение |
|------------|------------|
| `file.read` | Чтение содержимого файла |
| `file.write` | Запись содержимого файла |
| `shell.exec` | Выполнение shell-команды |
| `test.run` | Запуск набора тестов |
| `deploy.service` | Деплой сервиса |

### Интерфейс для реального MCP сервера

Для интеграции реального MCP сервера реализуйте этот интерфейс:

```go
// internal/mcp/client.go
type MCPClient interface {
    Connect(ctx context.Context, serverAddr string) error
    CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error)
    ListTools(ctx context.Context) ([]Tool, error)
    Close() error
}
```

### Пример HTTP/SSE клиента

```go
// internal/mcp/http_client.go
type HTTPClient struct {
    baseURL    string
    authToken  string
    httpClient *http.Client
}

func (c *HTTPClient) CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
    req := ToolRequest{Name: name, Arguments: args}
    
    httpReq, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tools/call", marshal(req))
    httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
    httpReq.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("MCP call failed: %w", err)
    }
    defer resp.Body.Close()
    
    var result ToolResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decode response failed: %w", err)
    }
    
    return result.Data, nil
}
```

### Конфигурация ACL Tool Gateway

```go
// Настроить какие инструменты может использовать каждый агент
toolGW.SetACL("dev", []string{
    "file.read", "file.write", "shell.exec", "git.*",
})
toolGW.SetACL("qa", []string{
    "test.run", "shell.exec", "file.read",
})
toolGW.SetACL("ops", []string{
    "deploy.service", "deploy.rollback", "shell.exec",
})
```

---

## Конфигурация

### Параметры ExecutionConfig

```go
type ExecutionConfig struct {
    DefaultTimeout     time.Duration // Таймаут задачи по умолчанию
    MaxRetries         int           // Макс. попыток повтора на задачу
    RetryBackoffBase   time.Duration // Начальная задержка повтора
    MaxParallelTasks    int           // Макс. параллельных задач
    MaxReplans         int           // Макс. итераций реплана
    
    // V5-специфичные
    QueueCapacity       int           // Размер очереди задач
    RPCCallRetries     int           // Повторы RPC-вызовов
    RPCBackoff         time.Duration // Задержка RPC
    TaskTimeout        time.Duration // Таймаут задачи на workers
    HeartbeatTimeout   time.Duration // Интервал проверки здоровья workers
}
```

### Значения по умолчанию для продакшена

```go
config := types.DefaultExecutionConfig()
// DefaultTimeout:     30s
// MaxRetries:        3
// RetryBackoffBase:  1s
// MaxParallelTasks:  4
// MaxReplans:        3
// QueueCapacity:     100
// RPCCallRetries:    3
// RPCBackoff:        500ms
// TaskTimeout:       60s
// HeartbeatTimeout:  30s
```

### Пользовательская конфигурация

```go
config := types.DefaultExecutionConfig()
config.DefaultTimeout = 10 * time.Second
config.MaxRetries = 5
config.QueueCapacity = 200
config.TaskTimeout = 120 * time.Second
```

---

## Настройка PostgreSQL

### Когда использовать PostgreSQL

- **Множественные экземпляры orchestrator** — Общее состояние между оркестраторами
- **Долгие задачи** — Состояние переживает перезапуски
- **Аудит** — Исторические данные задач
- **High availability** — Восстановление задач после сбоев

### Схема БД

```sql
CREATE TABLE task_states (
    task_id          TEXT PRIMARY KEY,
    idempotency_key  TEXT,
    state            TEXT NOT NULL,
    worker_id        TEXT,
    attempts         INT DEFAULT 0,
    last_error       TEXT,
    result           TEXT,
    created_at       TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_states_idempotency ON task_states(idempotency_key);
CREATE INDEX idx_task_states_state ON task_states(state);
```

### PostgreSQL в Docker

```bash
docker run -d \
  --name orchestrator-postgres \
  -e POSTGRES_DB=orchestrator \
  -e POSTGRES_USER=admin \
  -e POSTGRES_PASSWORD=secret \
  -p 5432:5432 \
  postgres:16-alpine
```

### Сборка с поддержкой PostgreSQL

```bash
go build -tags postgres ./...
```

---

## Продакшен развёртывание

### Docker Compose

```yaml
version: '3.8'

services:
  # ============================================
  # Orchestrator — Центральный координатор
  # ============================================
  orchestrator:
    build: .
    ports:
      - "8080:8080"
    environment:
      - WORKERS=worker-1:50051,worker-2:50052
      - POSTGRES_URL=postgres://admin:secret@postgres:5432/orchestrator
      - LOG_LEVEL=info
    depends_on:
      - postgres
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  # ============================================
  # Worker 1 — Исполнитель задач
  # ============================================
  worker-1:
    build: .
    command: worker --id=worker-1 --addr=:50051
    ports:
      - "50051:50051"
    environment:
      - LOG_LEVEL=info
    restart: unless-stopped

  # ============================================
  # Worker 2 — Исполнитель задач
  # ============================================
  worker-2:
    build: .
    command: worker --id=worker-2 --addr=:50052
    ports:
      - "50052:50052"
    environment:
      - LOG_LEVEL=info
    restart: unless-stopped

  # ============================================
  # PostgreSQL — Постоянное хранилище
  # ============================================
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: orchestrator
      POSTGRES_USER: admin
      POSTGRES_PASSWORD: secret
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U admin -d orchestrator"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
```

### Переменные окружения

| Переменная | Описание | По умолчанию |
|------------|----------|---------------|
| `WORKERS` | Список адресов workers через запятую | — |
| `POSTGRES_URL` | Строка подключения к PostgreSQL | — |
| `LOG_LEVEL` | Уровень логирования (debug/info/warn/error) | info |
| `QUEUE_CAPACITY` | Размер очереди задач | 100 |
| `MAX_RETRIES` | Макс. попыток повтора | 3 |

### Dockerfile

```dockerfile
FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -tags postgres -o orchestrator ./cmd/orchestrator
RUN CGO_ENABLED=0 GOOS=linux go build -tags postgres -o worker ./cmd/worker

FROM alpine:latest
RUN apk --no-cache add ca-certificates curl

WORKDIR /app
COPY --from=builder /app/orchestrator /app/
COPY --from=builder /app/worker /app/

ENTRYPOINT ["/app/orchestrator"]
CMD ["--distributed"]
```

### Проверки здоровья

```bash
# Проверить здоровье orchestrator
curl http://localhost:8080/health

# Проверить статус worker
curl http://localhost:50051/health
```

---

## Устранение проблем

### Частые проблемы

#### 1. "No healthy workers registered"

**Проблема:** Orchestrator не может найти доступные workers.

**Решение:**
```bash
# Проверить работающие workers
docker ps | grep worker

# Посмотреть логи worker
docker logs worker-1

# Проверить регистрацию worker
curl http://localhost:50051/health
```

#### 2. "Queue is full (backpressure: reject)"

**Проблема:** Очередь задач заполнена.

**Решение:**
```bash
# Увеличить ёмкость очереди
config.QueueCapacity = 500  # или через env: QUEUE_CAPACITY=500

# Или переключить на политику block (ожидание)
```

#### 3. "Panic recovered during task execution"

**Проблема:** Код задачи упал, но worker выжил.

**Решение:**
```bash
# Проверить логи задачи на stack trace паники
docker logs worker-1

# Проверить DLQ на предмет проваленных задач
# Доступ через API orchestrator или логи
```

#### 4. PostgreSQL connection failed

**Проблема:** Не удаётся подключиться к базе данных.

**Решение:**
```bash
# Убедиться что PostgreSQL запущен
docker ps | grep postgres

# Проверить формат строки подключения
POSTGRES_URL=postgres://user:pass@host:5432/dbname

# Тест подключения
docker exec -it postgres psql -U admin -d orchestrator -c "SELECT 1"
```

### Уровни логирования

Настраивается через окружение или код:

```bash
# Через окружение
LOG_LEVEL=debug go run ./cmd/orchestrator/

# Через код
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
```

### Режим отладки

Включить подробное логирование:

```bash
LOG_LEVEL=debug go run ./cmd/orchestrator/ --distributed
```

---

## Следующие шаги

1. **Заменить mock MCP инструменты** на реальные сетевые вызовы
2. **Настроить PostgreSQL** для постоянного хранения состояния
3. **Добавить Prometheus метрики** для мониторинга
4. **Настроить распределённую трассировку** с OpenTelemetry
5. **Настроить Redis/Kafka** для общей очереди задач (мульти-оркестратор)

---

## Поддержка

По вопросам и проблемам:
- Проверьте логи (установите `LOG_LEVEL=debug`)
- Изучите [Чеклист развёртывания в продакшен](../README.md#production-deployment-checklist)
- Проверьте Dead Letter Queue на предмет проваленных задач
