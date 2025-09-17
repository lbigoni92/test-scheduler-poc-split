// job.go
//
// Contiene la logica di esecuzione del job: idempotenza, lock distribuito,
// aggiornamento stato su DB e la funzione che esegue il "lavoro" vero e proprio.
package job

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"test-scheduler-poc/internal/db"
)

// Run esegue un job per (planID, scheduledAt) con:
// - idempotency key (planID|finestra)
// - advisory lock su Postgres per evitare doppi run
// - tracking stato su test_job_runs
func Run(conn *sql.DB, planID string, scheduledAt time.Time) error {
	ctx := context.Background()

	// Idempotency key: plan|YYYYMMDDTHHMM (UTC) -> precisione al minuto
	idempKey := fmt.Sprintf("%s|%s", planID, scheduledAt.UTC().Format("20060102T1504"))
	lockKey := hashToInt64(idempKey) // chiave di lock derivata dall'idempotency key

	// 1) Tenta il lock: se fallisce, registriamo uno "skipped_lock" per visibilità e usciamo
	got, err := db.TryAdvisoryLock(ctx, conn, lockKey)
	if err != nil {
		return fmt.Errorf("advisory_lock: %w", err)
	}
	if !got {
		_, _ = conn.ExecContext(ctx, `
            INSERT INTO test_job_runs(plan_id, idempotency_key, scheduled_at, outcome, worker_id, lock_key)
            VALUES ($1, $2, $3, 'skipped_lock', $4, $5)
            ON CONFLICT (plan_id, idempotency_key) DO NOTHING;
        `, planID, idempKey, scheduledAt, hostname(), lockKey)
		return nil
	}
	defer func() { _ = db.AdvisoryUnlock(ctx, conn, lockKey) }()

	// 2) Stato -> running (idempotente)
	_, err = conn.ExecContext(ctx, `
        INSERT INTO test_job_runs(plan_id, idempotency_key, scheduled_at, outcome, worker_id, lock_key)
        VALUES ($1, $2, $3, 'running', $4, $5)
        ON CONFLICT (plan_id, idempotency_key)
        DO UPDATE SET outcome='running', started_at=now(), worker_id=EXCLUDED.worker_id;
    `, planID, idempKey, scheduledAt, hostname(), lockKey)
	if err != nil {
		return fmt.Errorf("insert running: %w", err)
	}

	start := time.Now()
	log.Printf("Job START plan=%s scheduled_at=%s", planID, scheduledAt.Format(time.RFC3339))

	// 3) Esecuzione del lavoro vero e proprio
	if err := doWork(ctx); err != nil {
		// fallimento
		_, _ = conn.ExecContext(ctx, `
            UPDATE test_job_runs
            SET outcome='error', error=$1, ended_at=now()
            WHERE plan_id=$2 AND idempotency_key=$3;
        `, err.Error(), planID, idempKey)
		return err
	}

	// 4) Success
	_, _ = conn.ExecContext(ctx, `
        UPDATE test_job_runs
        SET outcome='success', ended_at=now()
        WHERE plan_id=$1 AND idempotency_key=$2;
    `, planID, idempKey)

	dur := time.Since(start)
	log.Printf("Job END plan=%s ok in %s", planID, dur)
	return nil
}

// doWork contiene la logica "utile" del job. Nel POC è una sleep di 2 secondi.
func doWork(ctx context.Context) error {
	select {
	case <-time.After(2 * time.Second):
		log.Printf("... doing work ...")
		return nil //fine lavoro
	case <-ctx.Done():
		return errors.New("context canceled")
	}
}

// hashToInt64 converte una stringa in un int64 deterministico (per advisory lock).
func hashToInt64(s string) int64 {
	h := sha1.Sum([]byte(s))
	u := binary.BigEndian.Uint64(h[:8])
	return int64(u)
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}
