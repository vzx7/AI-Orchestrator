# Python SDK

Async Python client for AI Orchestrator HTTP API.

## Installation (from this repo)

```bash
pip install -e sdk/python
```

## Installation (from PyPI - after publish)

```bash
pip install ai-orchestrator
```

## Usage

```python
import asyncio
from ai_orchestrator import OrchestratorClient, TaskRequest

async def main():
    async with OrchestratorClient(url="http://localhost:8080") as client:
        # Health check
        health = await client.health()
        print(f"Status: {health.status}")

        # Submit task
        resp = await client.submit_task(TaskRequest(goal="Analyze codebase"))
        print(f"Results: {resp.results}")

        # List tasks
        tasks = await client.list_tasks()
        print(f"Total: {tasks.count}")

asyncio.run(main())
```

## API

| Method | Description |
|--------|-------------|
| `health()` | Check server health |
| `submit_task(req)` | Submit a new task |
| `get_task(id)` | Get task by ID |
| `list_tasks()` | List all tasks |
| `cancel_task(id)` | Cancel a task |
| `queue_status()` | Get queue statistics |
| `get_dlq()` | Get Dead Letter Queue |