package cache

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

var DB_FILENAME = "kd_data.db"

var LiteDB *sql.DB

func InitDB() error {
	var err error

	dbPath := filepath.Join(CACHE_ROOT_PATH, DB_FILENAME)

	LiteDB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		zap.S().Errorf("Failed to open litedb: %s", err)
		return err
	}

	sqlStmt := `
CREATE TABLE IF NOT EXISTS en (
    query text NOT NULL UNIQUE PRIMARY KEY,
    detail text NOT NULL,
    update_time datetime NOT NULL) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS ch (
    query text NOT NULL UNIQUE PRIMARY KEY,
    detail text NOT NULL,
    update_time datetime NOT NULL) WITHOUT ROWID;
    `
	_, err = LiteDB.Exec(sqlStmt)

	if err != nil {
		zap.S().Errorf("Failed to create table", err)
		return err
	}
	return err
}

func getCachedRow(query string, isEN bool) ([]byte, error) {
	var err error
	var sql string
	if isEN {
		sql = "SELECT detail, update_time FROM en WHERE query = ?"
	} else {
		sql = "SELECT detail, update_time FROM ch WHERE query = ?"
	}
	row := LiteDB.QueryRow(sql, query)
	if row.Err() != nil {
		zap.S().Warnf("Failed to query %s: %s", query, row.Err())
		return []byte{}, row.Err()
	}
	var updateTime time.Time
	var detail []byte

	err = row.Scan(&detail, &updateTime)
	if err != nil {
		return []byte{}, err
	}
	zap.S().Debugf("Got %s with detail length %d update time: %s", query, len(detail), updateTime)
	return detail, nil
}

func saveCachedRow(query string, isEN bool, detail []byte) error {
	var table string
	var err error
	if isEN {
		table = "en"
	} else {
		table = "ch"
	}
	tx, err := LiteDB.Begin()
	if err != nil {
		zap.S().Warnf("Failed to open transaction for %s: %s", query, err)
		return err
	}
	sql := fmt.Sprintf("INSERT OR REPLACE INTO %s (query, detail, update_time) VALUES(?, ?, ?)", table)
	_, err = tx.Exec(sql, query, detail, time.Now())
	if err != nil {
		zap.S().Warnf("Failed to insert %s: %s", query, err)
		return err
	}
	err = tx.Commit()
	if err != nil {
		zap.S().Warnf("Failed to commit transaction for %s: %s", query, err)
	}
	return err
}
