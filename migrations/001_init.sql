CREATE TABLE IF NOT EXISTS test_plan_schedule (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'cron' CHECK (kind IN ('cron','oneshot')),
    execute_at TIMESTAMPTZ NULL,
    cron_expr VARCHAR(100) NULL,
    timezone VARCHAR(50) NOT NULL DEFAULT 'UTC',
    enabled  BOOLEAN     NOT NULL DEFAULT TRUE,
    consumed BOOLEAN     NOT NULL DEFAULT FALSE, -- marcata TRUE dopo l'esecuzione (oneshot)
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unique parziale: evita duplicati di one-shot (stesso plan_id alla stessa execute_at)
CREATE UNIQUE INDEX IF NOT EXISTS uq_plan_oneshot
  ON test_plan_schedule (plan_id, execute_at)
  WHERE kind = 'oneshot';


-- Storico dei run schedulati/eseguiti
CREATE TABLE IF NOT EXISTS test_job_runs (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL,         -- es. "planID|YYYYMMDDTHHMM"
    scheduled_at TIMESTAMPTZ NOT NULL,     -- finestra/logica del run
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at     TIMESTAMPTZ,
    outcome      VARCHAR(50),              -- running | success | error | skipped_lock
    error        TEXT,
    worker_id    VARCHAR(100),
    lock_key     BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (plan_id, idempotency_key)
);