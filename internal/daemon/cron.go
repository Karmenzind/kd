package daemon

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/internal/update"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

func InitCron() {
	go cronCheckUpdate()
	go cronUpdateDataZip()
	go cronDeleteSpam()
	go cronEnsureDaemonJsonFile()
}

func cronEnsureDaemonJsonFile() {
	ticker := time.NewTicker(5 * time.Minute)
	for {
		<-ticker.C
		di, err := GetDaemonInfoFromFile()
		// TODO 检查信息与当前是否匹配
		var needUpdate bool
		if err != nil {
			zap.S().Warnf("Failed to get daemon info from file: %s", err)
			needUpdate = true
		} else {
			ri := run.Info
			if !(ri.StartTime == di.StartTime &&
				ri.PID == di.PID &&
				ri.Port == di.Port &&
				ri.ExeName == di.ExeName &&
				ri.ExePath == di.ExePath &&
				ri.Version == di.Version) {
				zap.S().Warn("DaemonInfo from json is different from current run.Info")
				needUpdate = true
			}
		}
		if needUpdate {
			run.Info.SaveToFile(GetDaemonInfoPath())
			d.EchoRun("Update daemon.json")
		}
	}
}

func cronDeleteSpam() {
	ticker := time.NewTicker(600 * time.Second)
	for {
		<-ticker.C
		zap.S().Debugf("Started clearing spam")
		p, err := pkg.GetExecutablePath()
		if err == nil {
			bakPath := p + ".update_backup"
			if pkg.IsPathExists(bakPath) {
				err = os.Remove(bakPath)
				if err != nil {
					zap.S().Warnf("Failed to remove %s: %s", bakPath, err)
				}
			}
		}
	}
}

func cronCheckUpdate() {
	ticker := time.NewTicker(3600 * 12 * time.Second)
	for {
		// TODO (k): <2024-01-01> 改成检查文件Stat来判断时长
		<-ticker.C

		for i := 0; i < 3; i++ {
			tag, err := update.GetLatestTag()
			if err == nil {
				fmt.Println("最新Tag", tag)
				break
			}
			zap.S().Warnf("Failed to get latest tag: %s", err)
			time.Sleep(5 * time.Second)
		}

	}
}

const DATA_ZIP_URL = "https://gitee.com/void_kmz/kd/releases/download/v0.0.1/kd_data.zip"

func cronUpdateDataZip() {
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

			err = applyTempDB(dbPath, tempDBPath)
			if err != nil {
				zap.S().Warnf("Failed: %s", err)
				continue
			}

			// success
			os.WriteFile(tsFile, []byte(fmt.Sprint(time.Now().Unix())), os.ModePerm)

			ticker.Stop()
			// TODO 以消息通知形式
			zap.S().Info("DB文件发生改变，进程主动退出")
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
	if runtime.GOOS == "windows" {
		cache.LiteDB.Close()
		zap.S().Info("Waiting for 3 sec...")
		time.Sleep(3 * time.Second)
	}
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
