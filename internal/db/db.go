// db.go
//
// Helper per la connessione, ping, advisory lock e chiusura graceful del DB.
package db

import (
    "context"
    "database/sql"
    "time"

    _ "github.com/jackc/pgx/v5/stdlib"
)

// Open apre una connessione al DB via driver pgx/stdlib.
func Open(dsn string) (*sql.DB, error) {
    return sql.Open("pgx", dsn)
}

// Ping verifica la raggiungibilità del DB.
func Ping(db *sql.DB) error {
    return db.Ping()
}

// TryAdvisoryLock prova a ottenere un lock esclusivo applicativo.
func TryAdvisoryLock(ctx context.Context, db *sql.DB, key int64) (bool, error) {
    var got bool
    err := db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got)
    return got, err
}

// AdvisoryUnlock rilascia il lock precedentemente ottenuto.
func AdvisoryUnlock(ctx context.Context, db *sql.DB, key int64) error {
    _, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", key)
    return err
}

// CloseGraceful chiude il DB con un timeout, senza bloccare il main.
func CloseGraceful(ctx context.Context, db *sql.DB) error {
    ch := make(chan struct{})
    go func() {
        _ = db.Close()
        close(ch)
    }()
    select {
    case <-ch:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

// NowUTC ritorna l'ora corrente in UTC (utility talvolta utile per normalizzare i timestamp).
func NowUTC() time.Time { return time.Now().UTC() }
