//
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	// cos its cgo...
	_ "github.com/mattn/go-sqlite3"
)

const latestVersion string = "goatsms v1"

func main() {
	var dbname, driver string
	var fromGoSMS bool
	flag.StringVar(&dbname, "d", "goatsms.sqlite", "path to database")
	flag.StringVar(&driver, "t", "sqlite3", "database type")
	flag.BoolVar(&fromGoSMS, "from_gosms", false, "convert a gosms database to goatsms")
	flag.Parse()

	db, err := sql.Open(driver, dbname)
	if err != nil {
		fmt.Println("Opening database returned error: ", err)
		os.Exit(1)
	}
	defer db.Close()
	var version string
	if fromGoSMS {
		version = "gosms"
	} else {
		row := db.QueryRow("SELECT version FROM schema_version ORDER BY id DESC LIMIT 1")
		if err = row.Scan(&version); err != nil {
			fmt.Println("Reading schema version returned error: ", err)
			os.Exit(1)
		}
	}
	switch version {
	default:
		fmt.Printf("Don't know how to update database schema '%s'.\n", version)
		os.Exit(1)
	case latestVersion:
		fmt.Printf("Database '%s' schema '%s' is up to date.\n", dbname, version)
	case "gosms":
		if err := gosmsToV1(db); err != nil {
			fmt.Println("Conversion from gosms schema returned error: ", err)
			os.Exit(1)
		}
		fmt.Printf("Updated database '%s' schema to 'goatsms v1'.\n", dbname)
		// to chain updates, fall through to subsequent versions as schema versions change.
	}
}

// Conversion functions.

// gosmsToV1 converts a database from gosms to goatsms v1
func gosmsToV1(db *sql.DB) error {
	cmds := []string{
		"CREATE INDEX messages_status ON messages (status)",
		`CREATE TABLE schema_version (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
		version char(16) NOT NULL,
		created_at TIMESTAMP default CURRENT_TIMESTAMP
		);`,
		"INSERT INTO schema_version(version) VALUES('goatsms v1')",
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, cmd := range cmds {
		_, err = tx.Exec(cmd, nil)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	err = tx.Commit()
	return err
}
