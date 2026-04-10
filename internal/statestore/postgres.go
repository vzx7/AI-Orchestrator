//go:build postgres
// +build postgres

package statestore

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

// PostgresStore implements Store using PostgreSQL.
//
// Build with: go build -tags postgres
//
// Required table schema:
//
//	CREATE TABLE task_states (
//	    task_id         TEXT PRIMARY KEY,
//	    idempotency_key TEXT,
//	    state           TEXT NOT NULL,
//	    worker_id       TEXT,
//	    attempts        INT DEFAULT 0,
//	    last_error      TEXT,
//	    result          TEXT,
//	    created_at      TIMESTAMP NOT NULL,
//	    updated_at      TIMESTAMP NOT NULL
//	);
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a PostgreSQL-backed state store.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// SaveTaskState creates or updates a task's state using UPSERT.
func (s *PostgresStore) SaveTaskState(ts TaskState) error {
	ts.UpdatedAt = time.Now()
	if ts.CreatedAt.IsZero() {
		ts.CreatedAt = ts.UpdatedAt
	}

	query := `
		INSERT INTO task_states (task_id, idempotency_key, state, worker_id, attempts, last_error, result, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (task_id) DO UPDATE SET
			idempotency_key = EXCLUDED.idempotency_key,
			state = EXCLUDED.state,
			worker_id = EXCLUDED.worker_id,
			attempts = EXCLUDED.attempts,
			last_error = EXCLUDED.last_error,
			result = EXCLUDED.result,
			updated_at = EXCLUDED.updated_at
	`

	_, err := s.db.Exec(query,
		ts.TaskID, ts.IdempotencyKey, ts.State, ts.WorkerID,
		ts.Attempts, ts.LastError, ts.Result, ts.CreatedAt, ts.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save task state: %w", err)
	}

	return nil
}

// GetTaskState retrieves a task's current state.
func (s *PostgresStore) GetTaskState(taskID string) (TaskState, error) {
	query := `
		SELECT task_id, idempotency_key, state, worker_id, attempts, last_error, result, created_at, updated_at
		FROM task_states
		WHERE task_id = $1
	`

	var ts TaskState
	err := s.db.QueryRow(query, taskID).Scan(
		&ts.TaskID, &ts.IdempotencyKey, &ts.State, &ts.WorkerID,
		&ts.Attempts, &ts.LastError, &ts.Result, &ts.CreatedAt, &ts.UpdatedAt,
	)
	if err != nil {
		return TaskState{}, fmt.Errorf("failed to get task state: %w", err)
	}

	return ts, nil
}

// DeleteTaskState removes a task's state.
func (s *PostgresStore) DeleteTaskState(taskID string) error {
	_, err := s.db.Exec("DELETE FROM task_states WHERE task_id = $1", taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task state: %w", err)
	}
	return nil
}

// ListTaskStates returns all task states.
func (s *PostgresStore) ListTaskStates() ([]TaskState, error) {
	query := `
		SELECT task_id, idempotency_key, state, worker_id, attempts, last_error, result, created_at, updated_at
		FROM task_states
		ORDER BY updated_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list task states: %w", err)
	}
	defer rows.Close()

	var states []TaskState
	for rows.Next() {
		var ts TaskState
		if err := rows.Scan(
			&ts.TaskID, &ts.IdempotencyKey, &ts.State, &ts.WorkerID,
			&ts.Attempts, &ts.LastError, &ts.Result, &ts.CreatedAt, &ts.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		states = append(states, ts)
	}

	return states, nil
}
