# AI Orchestrator V5 — Руководство пользователя

> Это руководство объясняет, как использовать AI Orchestrator V5 через HTTP API.

---

## Содержание

1. [Быстрый старт](#1-быстрый-старт)
2. [HTTP API](#2-http-api)
3. [Альтернативы](#3-альтернативы)
4. [Интеграция в код](#4-интеграция-в-код)
5. [Настройка](#5-настройка)

---

## 1. Быстрый старт

### Запуск HTTP сервера

```bash
# Запуск с распределёнными воркерами (рекомендуется)
go run ./cmd/server/main.go -distributed

# Локальный режим (без HTTP)
go run ./cmd/orchestrator/
```

### Проверка соединения

```bash
curl http://localhost:8080/health
```

Ожидаемый ответ:
```json
{
  "status": "healthy",
  "version": "5.0.0",
  "workers": {
    "total": 2,
    "healthy": 2
  }
}
```

---

## 2. HTTP API

### Базовый URL

```
http://localhost:8080
```

### Endpoints

| Метод | Endpoint | Описание |
|-------|----------|----------|
| GET | `/health` | Проверка здоровья |
| POST | `/v1/tasks` | Создать новую задачу |
| GET | `/v1/tasks` | Список всех задач |
| GET | `/v1/tasks/{id}` | Получить задачу по ID |
| GET | `/v1/queue` | Статус очереди |
| GET | `/v1/dlq` | Dead Letter Queue |

### Создать задачу

```bash
curl -X POST http://localhost:8080/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "goal": "Fix failing test and deploy service"
  }'
```

Ответ:
```json
{
  "results": [
    {
      "task_id": "task-generic-xxx",
      "success": true,
      "output": {
        "agent": "dev",
        "executed_by": "worker-1"
      }
    }
  ],
  "status": "completed"
}
```

### С опциями

```bash
curl -X POST http://localhost:8080/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "goal": "Fix failing test",
    "idempotency_key": "my-unique-key",
    "timeout": 300,
    "metadata": {
      "project": "backend"
    }
  }'
```

### Список задач

```bash
curl http://localhost:8080/v1/tasks
```

### Статус задачи

```bash
curl http://localhost:8080/v1/tasks/task-xxx
```

### Статус очереди

```bash
curl http://localhost:8080/v1/queue
```

Ответ:
```json
{
  "pending": 0,
  "in_flight": 0,
  "dlq_count": 0
}
```

### Dead Letter Queue

```bash
curl http://localhost:8080/v1/dlq
```

---

## 3. Альтернативы

### Локальный режим (без сети)

```bash
go run ./cmd/orchestrator/
```

Вывод идёт прямо в консоль.

### Распределённый демо

```bash
go run ./cmd/orchestrator/ --distributed
```

---

## 4. Интеграция в код

### Go

```go
import "ai_orchestrator/internal/orchestrator"
import "ai_orchestrator/internal/types"

func main() {
    logger := slog.New(...)
    config := types.DefaultExecutionConfig()
    
    o := orchestrator.NewOrchestrator(logger, config)
    results, err := o.Execute(ctx, "Fix failing test")
}
```

### Примеры скриптов

```python
import requests

# Создать задачу
response = requests.post(
    "http://localhost:8080/v1/tasks",
    json={"goal": "Fix test"}
)
print(response.json())
```

---

## 5. Настройка

### Переменные окружения

```bash
# Порт сервера (по умолчанию: 8080)
go run ./cmd/server/main.go -addr=:9090

# Распределённый режим
go run ./cmd/server/main.go -distributed
```

### Опции

```bash
go run ./cmd/server/main.go --help
```

Вывод:
```
  -addr string
        HTTP server address (default ":8080")
  -api-key string
        API key for authentication
  -distributed
        Enable distributed mode
```

---

## Следующие шаги

- [Руководство по развёртыванию](DEPLOYMENT.md) — развёртывание в production
- [Описание сервиса](SERVICE_EXPLAINED.md) — внутреннее устройство

---

*Документ подготовлен для пользователей AI Orchestrator V5*