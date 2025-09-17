// migrate.go
//
// Carica ed esegue il file SQL di migrazione/DDL.
package schema

import (
    "database/sql"
    "os"
)

func Init(db *sql.DB, migrationsPath string) error {
    ddl, err := os.ReadFile(migrationsPath)
    if err != nil {
        return err
    }
    _, err = db.Exec(string(ddl))
    return err
}
