// main.go
//
// Entry-point dell'applicazione: carica configurazione, apre il DB, esegue le migrazioni,
// assicura una schedulazione di esempio e avvia il cron scheduler. Gestisce anche il
// graceful shutdown su SIGINT/SIGTERM.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"test-scheduler-poc/internal/config"
	"test-scheduler-poc/internal/db"
	"test-scheduler-poc/internal/scheduler"
	"test-scheduler-poc/internal/schema"
)

func main() {
	// 1) Config da env (con default sicuri per il POC)
	cfg := config.Load()

	// 2) Connessione al DB
	conn, err := db.Open(cfg.PostgresURI)
	if err != nil {
		log.Fatalf("connessione DB: %v", err)
	}
	defer conn.Close()

	if err := db.Ping(conn); err != nil {
		log.Fatalf("ping DB: %v", err)
	}

	// 3) Migrazioni/DDL: crea le tabelle di esempio
	if err := schema.Init(conn, cfg.MigrationsPath); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	// 4) Inserisce una schedule di esempio (se manca) per il plan_id richiesto
	// Inserisce/garantisce la schedule CRON per il plan (se non esiste)
	if _, err := conn.Exec(`
        INSERT INTO test_plan_schedule (plan_id, kind, cron_expr, timezone, enabled)
        SELECT $1, 'cron', $2, $3, TRUE;
    `, cfg.PlanID, cfg.CronExpr, cfg.Timezone); err != nil {
		log.Fatalf("insert cron schedule: %v", err)
	}

	// ONE-SHOT: pianifica una run esatta tra 30 secondi da adesso
	// NB: usiamo UTC per coerenza con l'idempotency key del job
	executeAt := time.Now().Add(90 * time.Second).UTC()

	if _, err := conn.Exec(`
        INSERT INTO test_plan_schedule (plan_id, kind, execute_at, timezone, enabled)
        VALUES ($1, 'oneshot', $2, $3, TRUE);
    `, cfg.PlanID, executeAt, cfg.Timezone); err != nil {
		log.Fatalf("insert oneshot schedule: %v", err)
	}

	log.Printf("One-shot schedulata per plan_id=%s alle %s (UTC)", cfg.PlanID, executeAt.Format(time.RFC3339))

	// 5) Avvia lo scheduler cron
	cr, err := scheduler.New(cfg)
	if err != nil {
		log.Fatalf("cron init: %v", err)
	}
	if err := scheduler.Register(cr, conn, cfg); err != nil {
		log.Fatalf("cron register: %v", err)
	}
	scheduler.StartOneShotPoller(conn, 10*time.Second)

	cr.Start()
	log.Printf("Scheduler avviato | plan_id=%s | cron=%q | tz=%s", cfg.PlanID, cfg.CronExpr, cfg.Timezone)

	// 6) Attende segnali per graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cr.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = db.CloseGraceful(ctx, conn)
	log.Println("Bye")
}
