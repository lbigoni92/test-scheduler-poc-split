# test Scheduler POC (Split files + commenti)

Questo POC mostra come schedulare job con Go + robfig/cron, evitare doppie esecuzioni con
i **PostgreSQL advisory lock**, e tracciare gli esiti su DB. Il codice è suddiviso in più file
e pacchetti, con **commenti** per spiegare le scelte.

## Struttura
```
test-scheduler-poc/
├─ docker-compose.yml
├─ .env.example
├─ go.mod
├─ migrations/
│  └─ 001_init.sql
├─ cmd/
│  └─ scheduler/
│     └─ main.go
└─ internal/
   ├─ config/
   │  └─ config.go
   ├─ db/
   │  └─ db.go
   ├─ schema/
   │  └─ migrate.go
   ├─ job/
   │  └─ job.go
   └─ scheduler/
      └─ scheduler.go
```

## Quickstart
```bash
docker compose up -d
cp .env.example .env
export $(grep -v '^#' .env | xargs)
go mod tidy
go run ./cmd/scheduler
```

Controlla i run:
```bash
psql "postgres://test:test@localhost:5432/test?sslmode=disable" -c "TABLE test_job_runs ORDER BY id DESC LIMIT 10;"
```

# Esempio di schedulazione 
Attuamente si schedula:

┌───────────── secondi (0 - 59)
│ ┌─────────── minuti (0 - 59)
│ │ ┌───────── ore (0 - 23)
│ │ │ ┌─────── giorno del mese (1 - 31)
│ │ │ │ ┌───── mese (1 - 12)
│ │ │ │ │ ┌─── giorno della settimana (0 - 6) (domenica=0)
│ │ │ │ │ │
0 */1 * * * *