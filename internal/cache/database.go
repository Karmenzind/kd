package cache

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

var DB_FILENAME = "kd_data.db"

var LiteDB *sql.DB

const sqliteDriverName = "sqlite"

const schema = `
CREATE TABLE IF NOT EXISTS en (
    query text NOT NULL UNIQUE PRIMARY KEY,
    detail text NOT NULL,
    update_time datetime NOT NULL) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS ch (
    query text NOT NULL UNIQUE PRIMARY KEY,
    detail text NOT NULL,
    update_time datetime NOT NULL) WITHOUT ROWID;
`

func initDBAtPath(dbPath string) (*sql.DB, error) {
	db, err := sql.Open(sqliteDriverName, dbPath)
	if err != nil {
		zap.S().Errorf("Failed to open litedb: %s", err)
		return nil, err
	}

	_, err = db.Exec(schema)
	if err != nil {
		db.Close()
		zap.S().Errorf("Failed to create tables: %s", err)
		return nil, err
	}
	return db, nil
}

func InitDB() error {
	dbPath := filepath.Join(CACHE_ROOT_PATH, DB_FILENAME)
	db, err := initDBAtPath(dbPath)
	if err != nil {
		return err
	}
	LiteDB = db
	return nil
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
	var updateValue any
	var detail []byte

	err = row.Scan(&detail, &updateValue)
	if err != nil {
		return []byte{}, err
	}
	if updateTime, timeErr := parseDatabaseTime(updateValue); timeErr == nil {
		zap.S().Debugf("Got %s with detail length %d update time: %s", query, len(detail), updateTime)
	} else {
		zap.S().Debugf("Got %s with detail length %d and unrecognized update time %q", query, len(detail), updateValue)
	}
	return detail, nil
}

func parseDatabaseTime(value any) (time.Time, error) {
	if value == nil {
		return time.Time{}, errors.New("database time is nil")
	}
	if parsed, ok := value.(time.Time); ok {
		return parsed, nil
	}

	var text string
	switch value := value.(type) {
	case string:
		text = value
	case []byte:
		text = string(value)
	default:
		return time.Time{}, fmt.Errorf("unsupported database time type %T", value)
	}
	text = strings.TrimSpace(text)
	fields := strings.Fields(text)
	if len(fields) == 4 {
		if parsed, err := time.Parse("2006-01-02 15:04:05.999999999 -0700", strings.Join(fields[:3], " ")); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported database time %q", text)
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
	defer tx.Rollback()
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

func GetCachedWordList(isEN bool) ([]string, error) {
	var table string
	if isEN {
		table = "en"
	} else {
		table = "ch"
	}

	sqlStr := fmt.Sprintf("SELECT query FROM %s ORDER BY query ASC", table)

	rows, err := LiteDB.Query(sqlStr)
	if err != nil {
		zap.S().Errorf("Failed to query all words from %s: %v", table, err)
		return nil, err
	}
	defer rows.Close()

	var words []string
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			zap.S().Warnf("Failed to scan row from %s: %v", table, err)
			continue
		}
		words = append(words, word)
	}

	if err = rows.Err(); err != nil {
		zap.S().Errorf("Row iteration error for %s: %v", table, err)
		return nil, err
	}

	zap.S().Debugf("Got %d cached words from %s", len(words), table)
	return words, nil
}
