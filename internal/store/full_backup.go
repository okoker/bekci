package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mattn/go-sqlite3"
)

// FullBackup creates a consistent snapshot of the database at destPath
// using the SQLite online backup API.
func (s *Store) FullBackup(destPath string) error {
	destDB, err := sql.Open("sqlite3", destPath)
	if err != nil {
		return fmt.Errorf("opening destination: %w", err)
	}
	defer destDB.Close()

	if err := destDB.Ping(); err != nil {
		return fmt.Errorf("pinging destination: %w", err)
	}

	ctx := context.Background()

	// Get the raw source connection
	var srcConn *sqlite3.SQLiteConn
	rawSrc, err := s.db.Conn(ctx)
	if err != nil {
		return s.fullBackupFallback(destPath)
	}
	defer rawSrc.Close()

	err = rawSrc.Raw(func(driverConn interface{}) error {
		var ok bool
		srcConn, ok = driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("source is not a sqlite3 connection")
		}
		return nil
	})
	if err != nil {
		return s.fullBackupFallback(destPath)
	}

	// Get the raw destination connection
	var destConn *sqlite3.SQLiteConn
	rawDest, err := destDB.Conn(ctx)
	if err != nil {
		return s.fullBackupFallback(destPath)
	}
	defer rawDest.Close()

	err = rawDest.Raw(func(driverConn interface{}) error {
		var ok bool
		destConn, ok = driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("destination is not a sqlite3 connection")
		}
		return nil
	})
	if err != nil {
		return s.fullBackupFallback(destPath)
	}

	// Perform the online backup
	backup, err := destConn.Backup("main", srcConn, "main")
	if err != nil {
		return fmt.Errorf("initiating backup: %w", err)
	}

	// Step through all pages at once (-1 = copy everything)
	done, err := backup.Step(-1)
	if err != nil {
		backup.Finish()
		return fmt.Errorf("backup step: %w", err)
	}
	if !done {
		backup.Finish()
		return fmt.Errorf("backup did not complete")
	}

	return backup.Finish()
}

// fullBackupFallback uses VACUUM INTO as a fallback if we can't get raw connections.
func (s *Store) fullBackupFallback(destPath string) error {
	_, err := s.db.Exec(`VACUUM INTO ?`, destPath)
	if err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	return nil
}
