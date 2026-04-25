"""Python SDK for AI Orchestrator.

Installation (from this repo):

    pip install -e ai_orchestrator/sdk/python

Usage:

    import asyncio
    from vzx7_orchestrator import OrchestratorClient, TaskRequest

    async def main():
        async with OrchestratorClient(url="http://localhost:8080") as client:
            health = await client.health()
            print(f"Status: {health.status}")

    asyncio.run(main())
"""
from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
from datetime import timedelta
from typing import Any

import aiohttp


@dataclass
class TaskRequest:
    """Task creation request."""
    goal: str
    idempotency_key: str | None = None
    timeout: timedelta | None = None
    metadata: dict[str, Any] | None = None

    def to_dict(self) -> dict[str, Any]:
        result = {"goal": self.goal}
        if self.idempotency_key:
            result["idempotency_key"] = self.idempotency_key
        if self.timeout:
            result["timeout"] = int(self.timeout.total_seconds())
        if self.metadata:
            result["metadata"] = self.metadata
        return result


@dataclass
class TaskResult:
    """Task execution result."""
    task_id: str
    success: bool
    output: dict[str, Any] | None = None
    error: str | None = None


@dataclass
class TaskResponse:
    """API response for task submission."""
    status: str
    results: list[TaskResult] = field(default_factory=list)


@dataclass
class TaskInfo:
    """Task information."""
    task_id: str
    state: str
    attempts: int = 0
    last_error: str | None = None
    worker_id: str | None = None
    created_at: str | None = None
    updated_at: str | None = None


@dataclass
class TaskList:
    """List of tasks."""
    tasks: list[TaskInfo] = field(default_factory=list)
    count: int = 0


@dataclass
class QueueStatus:
    """Queue statistics."""
    pending: int = 0
    in_flight: int = 0
    dlq_count: int = 0


@dataclass
class DLQEntry:
    """Dead Letter Queue entry."""
    task_id: str
    attempt: int
    reason: str
    failed_at: str


@dataclass
class DLQ:
    """Dead Letter Queue."""
    count: int = 0
    entries: list[DLQEntry] = field(default_factory=list)


@dataclass
class HealthInfo:
    """Health check response."""
    status: str
    version: str
    workers_total: int = 0
    workers_healthy: int = 0


class OrchestratorClient:
    """Async client for AI Orchestrator HTTP API."""

    def __init__(
        self,
        url: str = "http://localhost:8080",
        api_key: str | None = None,
        timeout: float = 30.0,
    ):
        self.url = url.rstrip("/")
        self.api_key = api_key
        self.timeout = aiohttp.ClientTimeout(total=timeout)
        self._session: aiohttp.ClientSession | None = None

    async def _get_session(self) -> aiohttp.ClientSession:
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession(timeout=self.timeout)
        return self._session

    def _headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        return headers

    async def close(self) -> None:
        """Close the HTTP session."""
        if self._session and not self._session.closed:
            await self._session.close()

    async def __aenter__(self) -> "OrchestratorClient":
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.close()

    async def health(self) -> HealthInfo:
        """Check server health."""
        session = await self._get_session()
        async with session.get(f"{self.url}/health", headers=self._headers()) as resp:
            resp.raise_for_status()
            data = await resp.json()
            return HealthInfo(
                status=data.get("status", ""),
                version=data.get("version", ""),
                workers_total=data.get("workers", {}).get("total", 0),
                workers_healthy=data.get("workers", {}).get("healthy", 0),
            )

    async def submit_task(self, request: TaskRequest) -> TaskResponse:
        """Submit a new task for execution."""
        session = await self._get_session()
        async with session.post(
            f"{self.url}/v1/tasks",
            json=request.to_dict(),
            headers=self._headers(),
        ) as resp:
            resp.raise_for_status()
            data = await resp.json()
            results = [
                TaskResult(
                    task_id=r.get("task_id", ""),
                    success=r.get("success", False),
                    output=r.get("output"),
                    error=r.get("error"),
                )
                for r in data.get("results", [])
            ]
            return TaskResponse(status=data.get("status", ""), results=results)

    async def get_task(self, task_id: str) -> TaskInfo | None:
        """Get task by ID."""
        session = await self._get_session()
        async with session.get(
            f"{self.url}/v1/tasks/{task_id}",
            headers=self._headers(),
        ) as resp:
            if resp.status == 404:
                return None
            resp.raise_for_status()
            data = await resp.json()
            return TaskInfo(
                task_id=data.get("task_id", ""),
                state=data.get("state", ""),
                attempts=data.get("attempts", 0),
                last_error=data.get("last_error"),
                worker_id=data.get("worker_id"),
                created_at=data.get("created"),
                updated_at=data.get("updated"),
            )

    async def list_tasks(self) -> TaskList:
        """List all tasks."""
        session = await self._get_session()
        async with session.get(
            f"{self.url}/v1/tasks",
            headers=self._headers(),
        ) as resp:
            resp.raise_for_status()
            data = await resp.json()
            tasks = [
                TaskInfo(
                    task_id=t.get("task_id", ""),
                    state=t.get("state", ""),
                    attempts=t.get("attempts", 0),
                )
                for t in data.get("tasks", [])
            ]
            return TaskList(tasks=tasks, count=data.get("count", 0))

    async def cancel_task(self, task_id: str) -> None:
        """Cancel a running task."""
        session = await self._get_session()
        async with session.post(
            f"{self.url}/v1/tasks/{task_id}/cancel",
            headers=self._headers(),
        ) as resp:
            resp.raise_for_status()

    async def queue_status(self) -> QueueStatus:
        """Get queue statistics."""
        session = await self._get_session()
        async with session.get(
            f"{self.url}/v1/queue",
            headers=self._headers(),
        ) as resp:
            resp.raise_for_status()
            data = await resp.json()
            return QueueStatus(
                pending=data.get("pending", 0),
                in_flight=data.get("in_flight", 0),
                dlq_count=data.get("dlq_count", 0),
            )

    async def get_dlq(self) -> DLQ:
        """Get Dead Letter Queue."""
        session = await self._get_session()
        async with session.get(
            f"{self.url}/v1/dlq",
            headers=self._headers(),
        ) as resp:
            resp.raise_for_status()
            data = await resp.json()
            entries = [
                DLQEntry(
                    task_id=e.get("task_id", ""),
                    attempt=e.get("attempt", 0),
                    reason=e.get("reason", ""),
                    failed_at=e.get("failed_at", ""),
                )
                for e in data.get("entries", [])
            ]
            return DLQ(count=data.get("count", 0), entries=entries)


async def main() -> None:
    """Example usage."""
    async with OrchestratorClient(url="http://localhost:8080") as client:
        health = await client.health()
        print(f"Status: {health.status}, Version: {health.version}")

        resp = await client.submit_task(TaskRequest(goal="Analyze codebase"))
        print(f"Submitted: {len(resp.results)} results")

        tasks = await client.list_tasks()
        print(f"Total tasks: {tasks.count}")


if __name__ == "__main__":
    asyncio.run(main())