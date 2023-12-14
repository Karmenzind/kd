package daemon

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/pkg"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

const DATA_ZIP_URL = "https://gitee.com/void_kmz/kd/releases/download/v0.0.1/kd_data.zip"

func InitCron() {
	go checkUpdate()
	go updateDataZip()
}

// check for update
func checkUpdate() {
	ticker := time.NewTicker(120 * time.Second)
	go func() {
		for {
			t := <-ticker.C
			zap.S().Debugf("Tick at %s", t)
		}
	}()
}

func updateDataZip() {
	ticker := time.NewTicker(3 * time.Second)
	go func() {
		for {
			<-ticker.C
			zap.S().Infof("Start check updating data zip")

			dbPath := filepath.Join(cache.CACHE_ROOT_PATH, cache.DB_FILENAME)
			tsFile := filepath.Join(cache.CACHE_RUN_PATH, "last_fetch_db")
			zipPath := filepath.Join(cache.CACHE_ROOT_PATH, "kd_data.zip")
			tempDBPath := dbPath + ".temp"

			if pkg.IsPathExists(tsFile) {
				zap.S().Infof("Found last update record. Nothing to do.")
				break
			}
			var need2Dl bool
			dbFileInfo, _ := os.Stat(dbPath)
			mb := dbFileInfo.Size() / 1024 / 1024
			zap.S().Debugf("Current db file size: %dMB", mb)

			if mb < 50 {
				fmt.Println(1)
				if pkg.IsPathExists(zipPath) {
					// TODO checksum
					fmt.Println(2)
					if checksumZIP(zipPath) {
						fmt.Println(3)
						need2Dl = false
					}
				} else {
					need2Dl = true
					fmt.Println(":)")
				}
			}
			zap.S().Debugf("Need to dl: %s", need2Dl)
			var err error
			// if need to download
			if need2Dl {
				zap.S().Debugf("Start downloading %s", DATA_ZIP_URL)
				err = downloadDataZip(zipPath)
				if err != nil {
					zap.S().Warnf("Failed to download zip file: %s", err)
					continue
				}
			}
			err = decompressDBZip(tempDBPath, zipPath)
			if err != nil {
				zap.S().Warnf("Failed: %s", err)
				continue
			}
			zap.S().Infof("Decompressed DB zip file -> %s", tempDBPath)

			// err = parseDBAndInsertOffsetVersion(tempDBPath)
			// os.Remove(tempDBPath)
			// if err != nil {
			// 	zap.S().Warnf("[parseDBAndInsert] Failed: %s", err)
			// 	continue
			// }

			applyTempDB(dbPath, tempDBPath)

			// success
			os.WriteFile(tsFile, []byte(fmt.Sprint(time.Now().Unix())), os.ModePerm)

			ticker.Stop()
			// TODO 以消息通知形式
			zap.S().Infof("DB文件发生改变，进程主动退出")
			fmt.Println("DB文件发生改变，进程主动退出")
			os.Exit(0)
			break
			// TODO (k): <2023-12-15> 看情况删除
			// os.Remove(zipPath)
		}
	}()
}

func checksumZIP(zipPath string) bool {
	return true
}

// download and checksum
func downloadDataZip(savePath string) (err error) {
	err = pkg.DownloadFile(savePath, DATA_ZIP_URL)
	if err != nil {
		zap.S().Warnf("Failed to download %s: %s", DATA_ZIP_URL)
	}
	zap.S().Info("Downloaded %s: %s", DATA_ZIP_URL)
	return
}

func decompressDBZip(tempDBPath, zipPath string) (err error) {
	a, err := zip.OpenReader(zipPath)
	if err != nil {
		zap.S().Errorf("Failed to open archive %s: %s", zipPath, err)
		return err
	}
	defer a.Close()

	f := a.File[0]
	dstFile, err := os.OpenFile(tempDBPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		zap.S().Warnf("Failed to open file %s to write", tempDBPath, err)
		return
	}
	defer dstFile.Close()

	srcFile, err := f.Open()
	if err != nil {
		zap.S().Warnf("Failed to open file %s to read", f.Name, err)
		return
	}
	defer srcFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		zap.S().Warnf("Failed to copy file %s to %s", f.Name, tempDBPath, err)
		return
	}
	return
}

// XXX 目前直接替换，后续可能需要插入的方式
func applyTempDB(dbPath, tempDBPath string) error {
	var err error
	now := time.Now()
	// cache.LiteDB.Close()
	backupPath := fmt.Sprintf("%s.backup.%d-%d-%d", dbPath, now.Year(), now.Month(), now.Day())
	err = os.Rename(dbPath, backupPath)
	if err != nil {
		zap.S().Errorf("Failed to backup %s: %s", dbPath, err)
		return err
	}
	zap.S().Info("Backup old dbfile -> ", backupPath)
	err = os.Rename(tempDBPath, dbPath)
	zap.S().Infof("Move %s -> %s", tempDBPath, dbPath)
	if err != nil {
		zap.S().Errorf("Failed to apply %s: %s", tempDBPath, err)
		return err
	}
	return err
}

func parseDBAndInsert(tempDBPath string) (err error) {
	now := time.Now()
	var tempDB *sql.DB
	tempDB, err = sql.Open("sqlite3", tempDBPath)
	if err != nil {
		zap.S().Errorf("Failed to open db %s: %s", tempDBPath, err)
		return
	}
	defer tempDB.Close()
	for _, table := range []string{"en", "ch"} {
		success := 0
		total := 0
		sql := fmt.Sprintf("SELECT query, detail FROM %s", table)
		rows, err := tempDB.Query(sql)
		if err != nil {
			zap.S().Warnf("Failed to query %s", table, err)
			return err
		}
		defer rows.Close()
		// insertSQL := fmt.Sprintf("INSERT OR IGNORE INTO %s (query, detail, update_time) VALUES (?, ?, ?)", table)
		// stmt, err := cache.LiteDB.Prepare(insertSQL)
		if err != nil {
			zap.S().Warnf("Failed to invoke db.Prepare for %s: %s", table, err)
			continue
		}
		for rows.Next() {
			total++
			var query []byte
			var detail []byte
			scanErr := rows.Scan(&query, &detail)
			if scanErr != nil {
				zap.S().Warnf("Failed to scan row for %s: %s", table, scanErr)
				continue
			}
			// _, insertErr := cache.LiteDB.Exec(insertSQL, query, detail, now)
			// _, insertErr := stmt.Exec(query, detail, now)
			// if insertErr != nil {
			// 	zap.S().Warnf("Failed to insert for %s: %s", table, insertErr)
			// 	continue
			// }
			_ = now
			fmt.Println(query)
			time.Sleep(100 * time.Microsecond)
			success++
			// zap.S().Debugf("Inserted into %s: %s", table, success)
		}
		// stmt.Close()
		// TODO (k): <2023-12-14> 计算成功多少，判定
		fmt.Printf("Table %s total %d success %d", table, total, success)
	}

	return
}

func parseDBAndInsertOffsetVersion(tempDBPath string) (err error) {
	now := time.Now()
	var tempDB *sql.DB
	tempDB, err = sql.Open("sqlite3", tempDBPath)
	if err != nil {
		zap.S().Errorf("Failed to open db %s: %s", tempDBPath, err)
		return
	}
	defer tempDB.Close()
	for _, table := range []string{"en", "ch"} {
		success := 0
		total := 0
		offset := 0
		batch := 100
		for {
			sql := fmt.Sprintf("SELECT query, detail FROM %s LIMIT %d, %d", table, offset, batch)
			rows, err := tempDB.Query(sql)

			if err != nil {
				zap.S().Warnf("Failed to query %s", table, err)
				return err
			}
			defer rows.Close()
			insertSQL := fmt.Sprintf("INSERT OR IGNORE INTO %s (query, detail, update_time) VALUES (?, ?, ?)", table)
			stmt, err := cache.LiteDB.Prepare(insertSQL)
			if err != nil {
				zap.S().Warnf("Failed to invoke db.Prepare for %s: %s", table, err)
				continue
			}
			params := make([][][]byte, 0, 100)
			fetched := 0
			for rows.Next() {
				total++
				var query []byte
				var detail []byte
				scanErr := rows.Scan(&query, &detail)
				if scanErr != nil {
					zap.S().Warnf("Failed to scan row for %s: %s", table, scanErr)
					continue
				}
				params = append(params, [][]byte{query, detail})

				_ = now
				// fmt.Println(query)
				// time.Sleep(100 * time.Microsecond)
				fetched += 1
				// zap.S().Debugf("Inserted into %s: %s", table, success)
			}

			// insertSQL := fmt.Sprintf("INSERT OR IGNORE INTO %s (query, detail, update_time) VALUES (?, ?, ?)", table)

			for _, row := range params {
				_, insertErr := stmt.Exec(row[0], row[1], now)
				if insertErr == nil {
					success++
				}
			}
			stmt.Close()
			if len(params) == 0 {
				break
			}

			time.Sleep(time.Duration(batch*10) * time.Millisecond)
			// TODO (k): <2023-12-14> 计算成功多少，判定
			fmt.Printf("Table %s total %d success %d \n", table, total, success)
			offset += batch
		}
	}
	return
}
