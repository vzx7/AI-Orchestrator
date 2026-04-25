# AI Orchestrator V5 — Руководство пользователя

> **Важно:** Этот документ описывает текущее состояние реализации. AI Orchestrator V5 — это **рабочий прототип** с некоторыми V5-фичами (Circuit Breaker, Visibility Reaper), которые присутствуют в коде, но ещё не подключены к основному потоку выполнения. Подробности в разделе [Статус реализации](#9-статус-реализации).

---

## Содержание

1. [Обзор](#1-обзор)
2. [Быстрый старт](#2-быстрый-старт)
3. [Локальная разработка](#3-локальная-разработка)
4. [Распределённый режим (демо)](#4-распределённый-режим-демо)
5. [Запуск воркеров](#5-запуск-воркеров)
6. [Интеграция в код](#6-интеграция-в-код)
7. [Настройка](#7-настройка)
8. [Справочник API](#8-справочник-api)
9. [Статус реализации](#9-статус-реализации)
10. [Решение проблем](#10-решение-проблем)

---

## 1. Обзор

```
┌─────────────────────────────────────────────────────────────────┐
│                     ЛОКАЛЬНАЯ РАЗРАБОТКА                        │
│                                                                  │
│  Терминал 1:             Терминал 2:             Терминал 3:    │
│  ┌─────────────┐        ┌─────────────┐        ┌─────────────┐ │
│  │ Orchestrator│  gRPC   │   Worker 1  │        │   Worker 2  │ │
│  │  (main.go)  │◄───────▶│ (cmd/worker)│        │ (cmd/worker)│ │
│  └─────────────┘  демо   └─────────────┘        └─────────────┘ │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Текущее состояние

| Функция | Статус | Примечания |
|---------|--------|------------|
| Локальный режим | ✅ Работает | Всё в одном процессе |
| Распределённый режим | ⚠️ Демо | Симуляция RPC в одном процессе |
| HTTP REST API | ❌ Не реализовано | Нет HTTP сервера |
| Настоящий gRPC | ❌ Не реализовано | Использует прямые Go вызовы |
| Circuit Breaker | ⚠️ Код есть | Не подключён к выполнению |
| Visibility Reaper | ⚠️ Код есть | Не запускается в main.go |
| Поддержка PostgreSQL | ⚠️ Требует build tag | `go build -tags postgres` |

---

## 2. Быстрый старт

### Требования

```bash
# Установите Go 1.26.1+
go version

# Клонируйте репозиторий
git clone <repo>
cd AI-Orchestrator

# Скачайте зависимости
go mod download
```

### Локальный режим

```bash
# Всё работает в одном процессе
go run ./cmd/orchestrator/

# Ожидаемый вывод:
# INFO: ===========================================
# INFO:    AI Orchestrator V5 — Local Mode
# INFO: ===========================================
# INFO: User goal: Fix failing test and deploy service
# INFO: === Execution Results ===
# INFO: Result 1: task_id=xxx, status=SUCCESS
# INFO: === V5 Demo Complete ===
```

### Распределённый режим (демо)

```bash
# Симулирует несколько воркеров в одном процессе
go run ./cmd/orchestrator/ --distributed
```

---

## 3. Локальная разработка

### Структура проекта

```
AI-Orchestrator/
├── cmd/
│   ├── orchestrator/          # Главная точка входа
│   │   └── main.go
│   └── worker/                # Точка входа воркера
│       └── main.go
├── internal/
│   ├── orchestrator/          # Главный координатор
│   ├── controller/            # Контроллер (Plan→Execute→Evaluate)
│   ├── planner/              # Генерация плана
│   ├── executor/             # Исполнение задач
│   ├── execution/            # Движок выполнения (DAG)
│   ├── evaluator/            # Оценка результатов
│   ├── agents/               # Реализации агентов
│   ├── tools/                # Tool Gateway (ACL)
│   ├── mcp/                  # MCP реестр инструментов
│   ├── queue/                # Очередь в памяти
│   ├── registry/             # Реестр воркеров
│   ├── rpc/                  # RPC слой (демо)
│   ├── resilience/           # Circuit breaker (V5)
│   ├── maintenance/          # Reaper (V5)
│   ├── idempotency/          # Магазин идемпотентности
│   ├── dlq/                  # Dead Letter Queue
│   ├── statestore/           # Сохранение состояния
│   ├── context/              # Менеджер контекста
│   ├── events/               # Шина событий
│   └── types/                # Определения типов
├── proto/
│   └── worker.proto          # gRPC определения (не скомпилированы)
└── docs/
    └── *.md
```

### Основные компоненты

| Компонент | Файл | Назначение |
|-----------|------|------------|
| **Orchestrator** | `internal/orchestrator/orchestrator.go` | Главный координатор |
| **Controller** | `internal/controller/controller.go` | Цикл Plan→Execute→Evaluate |
| **Execution Engine** | `internal/execution/engine.go` | DAG-параллельное выполнение |
| **Queue** | `internal/queue/queue.go` | Очередь с Ack/Nack |
| **Registry** | `internal/registry/registry.go` | Отслеживание здоровья воркеров |

### Команды разработки

```bash
# Запустить тесты
go test ./... -v

# Запустить конкретный тест
go test ./cmd/orchestrator -run TestDAGExecution -v

# Форматировать код
gofmt -w .
goimports -w .

# Сборка
go build ./...
go build -tags postgres ./...
```

---

## 4. Распределённый режим (демо)

### Как это работает

Распределённый режим симулирует несколько воркеров в одном процессе:

```bash
go run ./cmd/orchestrator/ --distributed
```

**Возможности демо:**
- Worker-1: Надёжный, всегда успешен
- Worker-2: Симулирует временный сбой при первой попытке, затем успех
- Автоматический механизм повтора
- Симуляция балансировки нагрузки
- Отслеживание состояния задач

### Пример вывода

```
INFO: Distributed workers initialized
INFO: Worker-1 executing task, task_id=xxx, agent=dev
INFO: Worker-2 simulated transient failure, task_id=xxx
INFO: Retrying RPC call, worker_id=worker-2, attempt=1
INFO: Worker-2 executing task (retry succeeded), task_id=xxx
```

---

## 5. Запуск воркеров

### Запуск Worker 1

**Терминал 1:**
```bash
go run ./cmd/worker --id=worker-1 --addr=localhost:50051
```

### Запуск Worker 2

**Терминал 2:**
```bash
go run ./cmd/worker --id=worker-2 --addr=localhost:50052
```

### Опции воркера

```bash
go run ./cmd/worker --help
```

Вывод:
```
Usage of worker:
  --id string      ID воркера (по умолчанию: worker-1)
  --addr string    Адрес прослушивания (по умолчанию: localhost:50051)
```

### Агенты воркера

Каждый воркер запускает три агента:

| Агент | Инструменты | Назначение |
|-------|-------------|------------|
| **DevAgent** | file.read, file.write, shell.exec | Изменения кода |
| **QAAgent** | test.run, shell.exec, file.read | Тестирование |
| **OpsAgent** | deploy.service, shell.exec | Деплой |

---

## 6. Интеграция в код

### Отправить задачу из Go

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "time"

    "ai_orchestrator/internal/orchestrator"
    "ai_orchestrator/internal/types"
)

func main() {
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    config := types.DefaultExecutionConfig()
    config.DefaultTimeout = 30 * time.Second
    config.MaxRetries = 3

    o := orchestrator.NewOrchestrator(logger, config)

    ctx := context.Background()
    results, err := o.Execute(ctx, "Fix failing test and deploy service")
    if err != nil {
        logger.Error("Execution failed", "error", err)
        return
    }

    for _, result := range results {
        logger.Info("Result",
            "task_id", result.TaskID,
            "success", result.Success,
            "duration_ms", result.Duration.Milliseconds(),
        )
    }
}
```

### Использовать Execution Engine напрямую

```go
import (
    "ai_orchestrator/internal/execution"
    "ai_orchestrator/internal/agents"
    "ai_orchestrator/internal/types"
)

engine := execution.NewEngine(config, logger, eventBus)
engine.RegisterAgent(agents.NewDevAgent(toolGateway, logger))
engine.RegisterAgent(agents.NewQAAgent(toolGateway, logger))

plan := types.Plan{Goal: "Fix test", Nodes: [...]}
results, err := engine.ExecutePlanDAG(ctx, plan)
```

### Свой агент

```go
package agents

import (
    "context"
    "ai_orchestrator/internal/types"
)

type CustomAgent struct{}

func (a *CustomAgent) Name() string {
    return "custom"
}

func (a *CustomAgent) Execute(ctx context.Context, task types.Task) (types.Result, error) {
    return types.Result{
        TaskID:  task.ID,
        Success: true,
        Output:  map[string]any{"message": "Done"},
    }, nil
}
```

Зарегистрировать в оркестрат��ре:
```go
engine.RegisterAgent(&CustomAgent{})
```

---

## 7. Настройка

### ExecutionConfig

```go
config := types.DefaultExecutionConfig()

// Таймауты
config.DefaultTimeout = 30 * time.Second  // Таймаут задачи по умолчанию
config.TaskTimeout = 60 * time.Second       // Таймаут на воркере
config.HeartbeatTimeout = 30 * time.Second // Проверка здоровья воркера

// Настройки повтора
config.MaxRetries = 3              // Макс попыток
config.RetryBackoffBase = 1 * time.Second  // Начальный backoff
config.RPCCallRetries = 3         // Повторы RPC вызовов
config.RPCBackoff = 500 * time.Millisecond // Backoff RPC

// Параллелизм
config.MaxParallelTasks = 4        // Макс параллельных задач
config.MaxReplans = 3             // Макс перепланирований

// Очередь
config.QueueCapacity = 100        // Размер очереди задач
```

### Переменные окружения

```bash
# Уровень логирования: debug, info, warn, error
LOG_LEVEL=debug go run ./cmd/orchestrator/

# PostgreSQL (с postgres build tag)
POSTGRES_URL=postgres://user:pass@localhost:5432/orchestrator
```

---

## 8. Справочник API

### Методы Orchestrator

```go
type Orchestrator struct {
    // Execute выполняет цель и возвращает результаты
    Execute(ctx context.Context, goal string) ([]Result, error)

    // EnableDistributedMode переключает на распределённый исполнитель
    EnableDistributedMode()

    // RegisterWorker добавляет воркер в реестр
    RegisterWorker(id, address string)

    // GetExecutionTrace возвращает трассировку последнего выполнения
    GetExecutionTrace() ExecutionTrace

    // GetWorkerRegistry возвращает реестр воркеров
    GetWorkerRegistry() *registry.MemoryRegistry

    // GetDeadLetterQueue возвращает DLQ
    GetDeadLetterQueue() *dlq.DeadLetterQueue
}
```

### Статусы задач

```go
const (
    TaskStatusPending   TaskStatus = "pending"
    TaskStatusRunning   TaskStatus = "running"
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusFailed    TaskStatus = "failed"
    TaskStatusCancelled TaskStatus = "cancelled"
    TaskStatusRetrying  TaskStatus = "retrying"
)
```

### Структура Result

```go
type Result struct {
    TaskID    string         // ID задачи
    Success   bool           // Успех/неудача
    Output    any            // Выходные данные
    Error     string         // Сообщение об ошибке
    Metadata  map[string]any// Дополнительные данные
    Duration  time.Duration  // Время выполнения
    Timestamp time.Time      // Время завершения
}
```

---

## 9. Статус реализации

### ✅ Реализовано

| Функция | Пакет | Статус |
|---------|-------|--------|
| Интерфейс агента | `internal/agents/` | ✅ Работает |
| DevAgent, QAAgent, OpsAgent | `internal/agents/` | ✅ Работает (mock) |
| Tool Gateway с ACL | `internal/tools/` | ✅ Работает |
| MCP реестр инструментов | `internal/mcp/` | ✅ Работает (mock) |
| Planner с DAG | `internal/planner/` | ✅ Работает |
| Controller (Plan→Execute→Evaluate) | `internal/controller/` | ✅ Работает |
| Execution Engine (DAG, parallel) | `internal/execution/` | ✅ Работает |
| Evaluator | `internal/evaluator/` | ✅ Работает |
| Memory Queue (Ack/Nack) | `internal/queue/` | ✅ Работает |
| Idempotency Store | `internal/idempotency/` | ✅ Работает |
| Dead Letter Queue | `internal/dlq/` | ✅ Работает |
| Worker Registry | `internal/registry/` | ✅ Работает |
| Event Bus | `internal/events/` | ✅ Работает |
| Context Manager | `internal/context/` | ✅ Работает |
| Task State Store | `internal/statestore/` | ✅ Работает |
| Memory-based Worker Registry | `internal/registry/` | ✅ Работает |
| RPC слой (демо) | `internal/rpc/` | ✅ Работает (в процессе) |

### ⚠️ Частичная реализация

| Функция | Пакет | Статус |
|---------|-------|--------|
| Circuit Breaker | `internal/resilience/` | ⚠️ Код есть, не подключён |
| Visibility Reaper | `internal/maintenance/` | ⚠️ Код есть, не запускается |
| Retry with Jitter | `internal/retry/` | ⚠️ Код есть, используется базовый retry |
| PostgreSQL State Store | `internal/statestore/` | ⚠️ Требует build tag |
| Latency-aware LB | `internal/registry/` | ⚠️ Код есть, ограниченный эффект |

### ❌ Не реализовано

| Функция | Примечания |
|---------|------------|
| HTTP REST API | Нет HTTP сервера в коде |
| Настоящий gRPC | Proto файл не скомпилирован |
| Prometheus метрики | Не добавлено |
| OpenTelemetry tracing | Не добавлено |
| Redis/Kafka очередь | Не добавлено |
| Настоящая LLM интеграция | Только mock planner |
| Настоящие MCP сетевые вызовы | Только mock инструменты |

### V5 фичи в коде

Несмотря на маркировку "V5", некоторые фичи требуют работы по интеграции:

```go
// Circuit breaker - существует, но не используется
cb := resilience.NewCircuitBreaker(resilience.DefaultConfig())
// Не вызывается нигде в пути выполнения

// Visibility reaper - существует, но не запускается
reaper := maintenance.NewVisibilityReaper(logger, q, config)
reaper.Start(ctx)  // Не вызывается в cmd/orchestrator/main.go
```

---

## 10. Решение проблем

### Ошибки сборки

```bash
# Отсутствуют зависимости
go mod download

# Неправильная версия Go
go version  # Нужна 1.26.1+
```

### Ошибки тестов

```bash
# Запустить с подробным выводом
go test ./... -v

# Запустить конкретный тест
go test ./cmd/orchestrator -run TestDAGExecution -v
```

### Воркер не подключается

Текущая реализация использует **демо RPC в процессе**. Воркеры не подключаются к оркестратору через реальную сеть. Это ограничение демо-режима.

```bash
# Для реального распределённого режима используйте флаг --distributed
go run ./cmd/orchestrator/ --distributed
```

### Проблемы с PostgreSQL

```bash
# Собрать с поддержкой postgres
go build -tags postgres ./...

# Проверить что PostgreSQL работает
docker ps | grep postgres

# Проверить подключение
docker exec -it postgres psql -U admin -d orchestrator -c "SELECT 1"
```

### Подробное логирование

```bash
# Режим отладки
LOG_LEVEL=debug go run ./cmd/orchestrator/ --distributed
```

---

## Следующие шаги

- [Руководство по развёртыванию](DEPLOYMENT.md) — инструкции по развёртыванию
- [Описание сервиса](SERVICE_EXPLAINED.md) — внутреннее устройство системы
- [README.md](../README.md) — общая информация о проекте

---

*Документ подготовлен для пользователей AI Orchestrator V5*