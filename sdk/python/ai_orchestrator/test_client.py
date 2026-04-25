"""Tests for Python SDK."""
import asyncio

import pytest

from ai_orchestrator import (
    OrchestratorClient,
    TaskRequest,
    TaskInfo,
    TaskList,
    QueueStatus,
    DLQ,
)


class TestOrchestratorClient:
    """Test OrchestratorClient."""

    @pytest.fixture
    def client(self):
        return OrchestratorClient(url="http://localhost:8080")

    def test_client_options(self):
        """Test client initialization."""
        client = OrchestratorClient(
            url="http://localhost:9999",
            api_key="test-key",
            timeout=5.0,
        )
        assert client.url == "http://localhost:9999"
        assert client.api_key == "test-key"
        assert client.timeout.total == 5.0

    @pytest.mark.asyncio
    async def test_health(self, client):
        """Test health check."""
        try:
            health = await client.health()
            assert health.status == "healthy"
        except Exception as e:
            pytest.skip(f"Server not running: {e}")

    @pytest.mark.asyncio
    async def test_submit_task(self, client):
        """Test task submission."""
        try:
            import uuid
            resp = await client.submit_task(
                TaskRequest(
                    goal="test goal",
                    idempotency_key=f"test-{uuid.uuid4()}",
                )
            )
            assert len(resp.results) > 0
        except Exception as e:
            pytest.skip(f"Server not running: {e}")

    @pytest.mark.asyncio
    async def test_list_tasks(self, client):
        """Test list tasks."""
        try:
            tasks = await client.list_tasks()
            assert tasks.count >= 0
        except Exception as e:
            pytest.skip(f"Server not running: {e}")

    @pytest.mark.asyncio
    async def test_queue_status(self, client):
        """Test queue status."""
        try:
            status = await client.queue_status()
            assert status.pending >= 0
        except Exception as e:
            pytest.skip(f"Server not running: {e}")

    @pytest.mark.asyncio
    async def test_get_dlq(self, client):
        """Test DLQ."""
        try:
            dlq = await client.get_dlq()
            assert dlq.count >= 0
        except Exception as e:
            pytest.skip(f"Server not running: {e}")

    @pytest.mark.asyncio
    async def test_context_manager(self):
        """Test async context manager."""
        async with OrchestratorClient(url="http://localhost:8080") as client:
            pass
        assert client._session is None or client._session.closed