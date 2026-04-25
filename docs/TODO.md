# AI Orchestrator V5 — План доработки

> TODO-лист для превращения рабочего прототипа в production-ready систему.

---

## Содержание

1. [Текущее состояние](#1-текущее-состояние)
2. [Приоритет P0 — Минимально работающая система](#2-приоритет-p0---минимально-работающая-система)
3. [Приоритет P1 — Cloud Deployment](#3-приоритет-p1---cloud-deployment)
4. [Приоритет P2 — SDK и клиенты](#4-приоритет-p2---sdk-и-клиенты)
5. [Приоритет P3 — Observability](#5-приоритет-p3---observability)
6. [Приоритет P4 — Интеграции](#6-приоритет-p4---интеграции)

---

## 1. Текущее состояние

| Компонент | Статус |
|-----------|-------|
| Локальный режим | ✅ Работает |
| Распределённый режим | ✅ Работает с demo воркерами |
| HTTP REST API | ✅ Работает на :8080 |
| Circuit Breaker | ✅ Подключён |
| Visibility Reaper | ✅ Запускается |
| gRPC Proto | ✅ Скомпилирован |

---

## 2. Приоритет P0 — Минимально работающая система ✅ ЗАВЕРШЕНО

### Выполнено:

- [x] P0.1: Circuit Breaker подключён в RPC (internal/rpc/rpc.go)
- [x] P0.2: Visibility Reaper запускается (cmd/orchestrator/main.go)
- [x] P0.3: HTTP сервер добавлен (cmd/server/main.go)
- [x] P0.4: gRPC proto скомпилирован

---

## 3. Приоритет P1 — Cloud Deployment ✅ ЗАВЕРШЕНО

Деплой в облако.

### P1.1: Docker Compose для production

**Выполнено:**
- [x] docker-compose.yml для production
- [x] Environment variables (deploy/.env.example)
- [x] Health checks
- [x] Volumes
- [x] Dockerfile с multi-stage builds

### P1.2: Kubernetes manifests

**Выполнено:**
- [x] Namespace, ConfigMap, Secrets
- [x] PostgreSQL Deployment + Service
- [x] Redis Deployment + Service
- [x] Orchestrator Deployment + LoadBalancer
- [x] Workers Deployment
- [x] HPA (HorizontalPodAutoscaler)
- [x] PVC для persistent storage
- [x] Kustomization.yaml

### P1.3: CI/CD pipeline

**Выполнено:**
- [x] GitHub Actions workflow (.github/workflows/ci-cd.yml)
- [x] Makefile для разработки
- [x] Test, Lint, Build, Deploy stages

### P1.4: PostgreSQL интеграция

**Выполнено:**
- [x] init.sql схема
- [x] Docker volumes
- [ ] GitHub Actions workflow: Lint → Test → Build → Push
- [ ] Docker build с Multi-arch (amd64, arm64)

### P1.4: PostgreSQL интеграция

**TODO:**
- [ ] Убрать build tag для PostgreSQL
- [ ] Добавить миграции БД

---

## 4. Приоритет P2 — SDK и клиенты

Клиентские библиотеки.

### P2.1: Go SDK

**TODO:**
- [ ] Создать orchestrator-go SDK
- [ ] Методы: SubmitTask, GetTask, ListTasks, CancelTask

### P2.2: Python SDK

**TODO:**
- [ ] Создать orchestrator-python пакет
- [ ] Async support

### P2.3: CLI инструмент

**TODO:**
- [ ] Команды: run, get, list, cancel

---

## 5. Приоритет P3 — Observability

Мониторинг.

### P3.1: Prometheus метрики

**TODO:**
- [ ] Добавить метрики (task duration, queue size, workers)
- [ ] /metrics endpoint

### P3.2: Structured logging

**TODO:**
- [ ] JSON логи
- [ ] TraceID для каждого запроса

### P3.3: OpenTelemetry

**TODO:**
- [ ] Tracing для задач
- [ ] Export в Jaeger/Zipkin

---

## 6. Приоритет P4 — Интеграции

Реальные интеграции.

### P4.1: Real LLM интеграция

**TODO:**
- [ ] Интерфейс для LLM клиента
- [ ] Поддержка OpenAI / Anthropic

### P4.2: Real MCP Tools

**TODO:**
- [ ] HTTP MCP клиент
- [ ] Tool execution через сеть

### P4.3: Redis/Kafka для очереди

**TODO:**
- [ ] Redis queue
- [ ] Multi-instance support
- [ ] Kafka events

---

## Приоритеты суммарно

| Приоритет | Задач  | Статус      |
| -----------| --------| -------------|
| **P0**    | 4      | ✅ Завершено |
| **P1**    | 4      | В работе    |
| **P2**    | 3      | —           |
| **P3**    | 3      | —           |
| **P4**    | 3      | —           |
| **Итого** | **17** |             |

---

*TODO list updated: 2025-04-25*
*Status: P0 completed → P1 in progress*