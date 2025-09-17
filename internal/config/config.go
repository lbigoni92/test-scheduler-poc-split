// config.go
//
// Lettura delle variabili d'ambiente con default sensati per il POC.
package config

import (
    "os"
    "strings"
)

type Config struct {
    PostgresURI    string // DSN di connessione a PostgreSQL
    CronExpr       string // espressione cron con secondi abilitati
    PlanID         string // identificativo del piano
    Timezone       string // timezone logica del job
    MigrationsPath string // percorso file migrazioni (DDL)
}

func envOrDefault(key, def string) string {
    v := strings.TrimSpace(os.Getenv(key))
    if v == "" {
        return def
    }
    return v
}

func Load() Config {
    return Config{
        PostgresURI:    envOrDefault("POSTGRES_URI", "postgres://test:test@localhost:5432/test?sslmode=disable"),
        CronExpr:       envOrDefault("CRON_EXPR", "0 */1 * * * *"), // default: ogni minuto al secondo 0
        PlanID:         envOrDefault("PLAN_ID", "1"),
        Timezone:       envOrDefault("TIMEZONE", "UTC"),
        MigrationsPath: envOrDefault("MIGRATIONS_PATH", "migrations/001_init.sql"),
    }
}
