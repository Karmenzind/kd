package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	"go.uber.org/zap"
)

// sanitizeVersion 是文本清洗规则的版本号。清洗规则变化时+1，
// daemon会据此重新扫描一遍词库。
// v2: 换行折成空格而非直接删除，避免`young\nsheep`被粘成`youngsheep`
const sanitizeVersion = 2

const sanitizeVersionKey = "sanitize_version"

const (
	sanitizeBatchSize = 200
	// 每批之间的间歇，避免长时间占用写锁影响查询
	sanitizeBatchRest = 50 * time.Millisecond
)

type SanitizeReport struct {
	Scanned int
	Cleaned int
}

type sanitizedRow struct {
	query string
	// original 是写回时的比对基准，用于跳过扫描期间被刷新过的行
	original []byte
	detail   []byte
}

// SanitizeLegacyRows 清洗词库中残留的未规整文本。早期版本（以及下发的离线词库）
// 写入的条目保存的是未清洗的HTML原文，含换行与大段缩进，渲染时会导致排版错乱。
//
// 清洗是幂等的，完成后把版本号写入meta表。meta表随词库文件一起存在，
// 因此整库被替换后会自动重新扫描一次，其余情况下只查一次meta表即返回。
// 中途失败不会记录版本号，留待下次重来。
func SanitizeLegacyRows(ctx context.Context) (report SanitizeReport, err error) {
	done, err := sanitizeDone()
	if err != nil {
		return report, err
	}
	if done {
		zap.S().Debugf("Cached rows already sanitized (version %d)", sanitizeVersion)
		return report, nil
	}

	zap.S().Infof("Start sanitizing cached rows (version %d)", sanitizeVersion)
	for _, table := range []string{"en", "ch"} {
		tableReport, tableErr := sanitizeTable(ctx, table)
		report.Scanned += tableReport.Scanned
		report.Cleaned += tableReport.Cleaned
		if tableErr != nil {
			return report, tableErr
		}
	}

	if err = setMetaValue(sanitizeVersionKey, strconv.Itoa(sanitizeVersion)); err != nil {
		return report, err
	}
	zap.S().Infof("Sanitized %d of %d cached rows", report.Cleaned, report.Scanned)
	return report, nil
}

func sanitizeDone() (bool, error) {
	value, err := getMetaValue(sanitizeVersionKey)
	if err != nil {
		return false, err
	}
	if value == "" {
		return false, nil
	}
	recorded, convErr := strconv.Atoi(value)
	if convErr != nil {
		zap.S().Warnf("Unrecognized %s %q, will sanitize again", sanitizeVersionKey, value)
		return false, nil
	}
	return recorded >= sanitizeVersion, nil
}

func sanitizeTable(ctx context.Context, table string) (report SanitizeReport, err error) {
	selectSQL := fmt.Sprintf("SELECT query, detail FROM %s WHERE query > ? ORDER BY query LIMIT ?", table)

	// 主键即query，用游标翻页，避免边写边翻页导致的漏扫
	var cursor string
	for {
		if err = ctx.Err(); err != nil {
			return report, err
		}

		rows, queryErr := LiteDB.QueryContext(ctx, selectSQL, cursor, sanitizeBatchSize)
		if queryErr != nil {
			zap.S().Warnf("Failed to scan table %s: %s", table, queryErr)
			return report, queryErr
		}

		fetched := 0
		pending := make([]sanitizedRow, 0, sanitizeBatchSize)
		for rows.Next() {
			var query string
			var detail []byte
			if scanErr := rows.Scan(&query, &detail); scanErr != nil {
				rows.Close()
				zap.S().Warnf("Failed to scan row of %s: %s", table, scanErr)
				return report, scanErr
			}
			fetched++
			cursor = query
			if cleaned, changed := sanitizeDetail(table, detail); changed {
				pending = append(pending, sanitizedRow{query: query, original: detail, detail: cleaned})
			}
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			zap.S().Warnf("Failed to iterate table %s: %s", table, rowsErr)
			return report, rowsErr
		}
		report.Scanned += fetched

		if len(pending) > 0 {
			applied, applyErr := applySanitized(ctx, table, pending)
			if applyErr != nil {
				return report, applyErr
			}
			report.Cleaned += applied
		}

		if fetched < sanitizeBatchSize {
			return report, nil
		}
		// 只在真正写过的批次之后让出写锁
		if len(pending) == 0 {
			continue
		}

		select {
		case <-ctx.Done():
			return report, ctx.Err()
		case <-time.After(sanitizeBatchRest):
		}
	}
}

// sanitizeDetail 清洗单行的压缩内容，返回新的压缩内容与是否发生变化。
// 解析失败的行原样保留，由后续的在线查询覆盖。
// 出于隐私考虑不记录具体的查询词（见AGENTS.md的日志约定）。
func sanitizeDetail(table string, detail []byte) ([]byte, bool) {
	body, err := decompressDetail(detail)
	if err != nil {
		zap.S().Debugf("Skipped a row of %s: failed to decompress: %s", table, err)
		return nil, false
	}

	var r model.Result
	if err = json.Unmarshal(body, &r); err != nil {
		zap.S().Debugf("Skipped a row of %s: failed to unmarshal: %s", table, err)
		return nil, false
	}
	before, err := json.Marshal(&r)
	if err != nil {
		zap.S().Debugf("Skipped a row of %s: failed to marshal: %s", table, err)
		return nil, false
	}
	r.Sanitize()
	after, err := json.Marshal(&r)
	if err != nil {
		zap.S().Debugf("Skipped a row of %s: failed to marshal sanitized result: %s", table, err)
		return nil, false
	}
	if bytes.Equal(before, after) {
		return nil, false
	}

	cleaned, err := compressDetail(after)
	if err != nil {
		zap.S().Debugf("Skipped a row of %s: failed to compress: %s", table, err)
		return nil, false
	}
	return cleaned, true
}

// applySanitized 只更新detail，保留update_time，以免污染内容的新鲜度。
// 更新以原内容为条件：扫描与写回之间如果有在线查询刷新了该行，
// 就跳过它，以免用清洗过的旧内容覆盖刚拿到的新结果。
// 返回实际写入的行数。
func applySanitized(ctx context.Context, table string, rows []sanitizedRow) (int, error) {
	tx, err := LiteDB.BeginTx(ctx, nil)
	if err != nil {
		zap.S().Warnf("Failed to open transaction for sanitizing %s: %s", table, err)
		return 0, err
	}
	defer tx.Rollback()

	updateSQL := fmt.Sprintf("UPDATE %s SET detail = ? WHERE query = ? AND detail = ?", table)
	stmt, err := tx.PrepareContext(ctx, updateSQL)
	if err != nil {
		zap.S().Warnf("Failed to prepare sanitizing statement for %s: %s", table, err)
		return 0, err
	}
	defer stmt.Close()

	applied := 0
	for _, row := range rows {
		result, execErr := stmt.ExecContext(ctx, row.detail, row.query, row.original)
		if execErr != nil {
			zap.S().Warnf("Failed to sanitize a row of %s: %s", table, execErr)
			return 0, execErr
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			zap.S().Warnf("Failed to inspect sanitized row of %s: %s", table, affectedErr)
			return 0, affectedErr
		}
		applied += int(affected)
	}
	if skipped := len(rows) - applied; skipped > 0 {
		zap.S().Debugf("Skipped %d row(s) of %s refreshed during sanitizing", skipped, table)
	}
	if err = tx.Commit(); err != nil {
		zap.S().Warnf("Failed to commit sanitized rows of %s: %s", table, err)
		return 0, err
	}
	return applied, nil
}
