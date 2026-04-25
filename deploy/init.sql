-- PostgreSQL initialization script for AI Orchestrator

-- ============================================
-- Task States Table
-- ============================================
CREATE TABLE IF NOT EXISTS task_states (
    task_id          TEXT PRIMARY KEY,
    idempotency_key  TEXT UNIQUE,
    state            TEXT NOT NULL,
    worker_id        TEXT,
    attempts         INT DEFAULT 0,
    last_error       TEXT,
    result           TEXT,
    created_at       TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for task_states
CREATE INDEX IF NOT EXISTS idx_task_states_idempotency ON task_states(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_task_states_state ON task_states(state);
CREATE INDEX IF NOT EXISTS idx_task_states_created ON task_states(created_at);

-- ============================================
-- Execution Traces Table
-- ============================================
CREATE TABLE IF NOT EXISTS execution_traces (
    trace_id        TEXT PRIMARY KEY,
    goal           TEXT NOT NULL,
    plan           JSONB,
    results        JSONB,
    total_steps    INT,
    completed_steps INT,
    failed_steps  INT,
    duration_ms   BIGINT,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_execution_traces_goal ON execution_traces(goal);
CREATE INDEX IF NOT EXISTS idx_execution_traces_created ON execution_traces(created_at);

-- ============================================
-- Worker Registry Table
-- ============================================
CREATE TABLE IF NOT EXISTS workers (
    id              TEXT PRIMARY KEY,
    address         TEXT NOT NULL,
    capacity        INT DEFAULT 4,
    active_tasks    INT DEFAULT 0,
    healthy         BOOLEAN DEFAULT true,
    last_heartbeat  TIMESTAMP,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workers_healthy ON workers(healthy);

-- ============================================
-- Dead Letter Queue Table
-- ============================================
CREATE TABLE IF NOT EXISTS dlq (
    id              SERIAL PRIMARY KEY,
    task_id         TEXT NOT NULL,
    goal            TEXT,
    attempts        INT NOT NULL,
    fail_reason     TEXT,
    payload         JSONB,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dlq_task_id ON dlq(task_id);
CREATE INDEX IF NOT EXISTS idx_dlq_created ON dlq(created_at);

-- ============================================
-- Idempotency Cache Table
-- ============================================
CREATE TABLE IF NOT EXISTS idempotency_cache (
    key             TEXT PRIMARY KEY,
    result          JSONB NOT NULL,
    created_at     TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_idempotency_created ON idempotency_cache(created_at);

-- ============================================
-- Function to auto-update updated_at
-- ============================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers
CREATE TRIGGER update_task_states_updated_at
    BEFORE UPDATE ON task_states
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_workers_updated_at
    BEFORE UPDATE ON workers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_idempotency_updated_at
    BEFORE UPDATE ON idempotency_cache
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();