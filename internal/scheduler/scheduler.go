// scheduler.go
//
// Incapsula l'istanza di robfig/cron e la registrazione del job periodico.
// Si occupa anche di rispettare la timezone richiesta per il calcolo della finestra.
package scheduler

import (
	"database/sql"
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"test-scheduler-poc/internal/config"
	"test-scheduler-poc/internal/job"
)

// New crea un cron scheduler con supporto ai secondi e (opzionale) con la timezone richiesta.
func New(cfg config.Config) (*cron.Cron, error) {
	// Carichiamo la location per poter calcolare 'scheduledAt' e, se vuoi, anche la TZ del cron stesso.
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, err
	}
	// Se vuoi che la SCHEDULE segua la TZ specifica, decommenta WithLocation(loc).
	return cron.New(cron.WithSeconds(), cron.WithLocation(loc)), nil
}

type ScheduleRow struct {
	ID        int64
	PlanID    string
	Kind      string // "cron" | "oneshot"
	CronExpr  string
	Timezone  string
	ExecuteAt *time.Time // nil per cron
	Consumed  bool
	Enabled   bool
}

func loadSchedules(db *sql.DB) ([]ScheduleRow, error) {
	rows, err := db.Query(`
      SELECT id, plan_id::text, kind, COALESCE(cron_expr, '') AS cron_expr, timezone, execute_at, consumed, enabled
      FROM test_plan_schedule
      WHERE enabled = TRUE
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScheduleRow
	for rows.Next() {
		var s ScheduleRow
		var execAt sql.NullTime
		if err := rows.Scan(&s.ID, &s.PlanID, &s.Kind, &s.CronExpr, &s.Timezone, &execAt, &s.Consumed, &s.Enabled); err != nil {
			return nil, err
		}
		if execAt.Valid {
			t := execAt.Time
			s.ExecuteAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func Register(c *cron.Cron, conn *sql.DB, cfg config.Config) error {
	schedules, err := loadSchedules(conn)
	if err != nil {
		return err
	}

	for _, s := range schedules {
		loc, err := time.LoadLocation(s.Timezone)
		if err != nil {
			loc = time.UTC
		}

		switch s.Kind {
		case "cron":
			if s.CronExpr == "" {
				continue
			}
			// usa la tua Register cron esistente
			_, err = c.AddFunc(s.CronExpr, func(planID, tz string) func() {
				return func() {
					now := time.Now().In(loc)
					scheduledAt := now.Truncate(time.Minute)
					_ = job.Run(conn, s.PlanID, scheduledAt)
				}
			}(s.PlanID, s.Timezone))
			if err != nil {
				return err
			}

		case "oneshot":
			if s.ExecuteAt == nil || s.Consumed {
				// niente da fare
				continue
			}
			runAt := s.ExecuteAt.In(loc)
			delay := time.Until(runAt)
			if delay <= 0 {
				// già scaduto → esegui subito (catch-up post-restart)
				go runOneShot(conn, s)
			} else {
				time.AfterFunc(delay, func() { runOneShot(conn, s) })
			}
		}
	}
	return nil
}

func runOneShot(conn *sql.DB, s ScheduleRow) {
	var consumed bool
	_ = conn.QueryRow(`SELECT consumed FROM test_plan_schedule WHERE id=$1`, s.ID).Scan(&consumed)
	if consumed {
		return
	}

	scheduledAt := s.ExecuteAt.UTC().Truncate(time.Minute)
	_ = job.Run(conn, s.PlanID, scheduledAt)

	_, _ = conn.Exec(`UPDATE test_plan_schedule SET consumed=TRUE WHERE id=$1 AND consumed=FALSE`, s.ID)
	log.Printf("One-shot eseguita per plan_id=%s scheduled_at=%s (UTC)", s.PlanID, scheduledAt.Format(time.RFC3339))
}

func StartOneShotPoller(conn *sql.DB, interval time.Duration) {
	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for range t.C {
			rows, err := conn.Query(`
              SELECT id, plan_id::text, timezone, execute_at
              FROM test_plan_schedule
              WHERE enabled=TRUE AND kind='oneshot' AND consumed=FALSE
                AND execute_at <= now()
            `)
			if err != nil {
				continue
			}
			for rows.Next() {
				var s ScheduleRow
				var execAt time.Time
				if err := rows.Scan(&s.ID, &s.PlanID, &s.Timezone, &execAt); err == nil {
					s.ExecuteAt = &execAt
					go runOneShot(conn, s)
				}
			}
			rows.Close()
		}
	}()
}
