package cache

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

var legacyUpdateTime = time.Date(2023, 12, 14, 16, 14, 21, 0, time.UTC)

// insertLegacyRow 写入一条未经清洗的记录，模拟旧版本（及下发词库）的存量数据。
func insertLegacyRow(t *testing.T, table string, query string, r *model.Result) {
	t.Helper()

	body, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal %q error = %v", query, err)
	}
	detail, err := compressDetail(body)
	if err != nil {
		t.Fatalf("compressDetail(%q) error = %v", query, err)
	}
	_, err = LiteDB.Exec(
		"INSERT OR REPLACE INTO "+table+" (query, detail, update_time) VALUES (?, ?, ?)",
		query, detail, legacyUpdateTime,
	)
	if err != nil {
		t.Fatalf("insert %q error = %v", query, err)
	}
}

// readStoredResult 直接读取库中内容，不经过GetCachedQuery的读时清洗。
func readStoredResult(t *testing.T, table string, query string) (*model.Result, time.Time) {
	t.Helper()

	var detail []byte
	var updateValue any
	err := LiteDB.QueryRow(
		"SELECT detail, update_time FROM "+table+" WHERE query = ?", query,
	).Scan(&detail, &updateValue)
	if err != nil {
		t.Fatalf("select %q error = %v", query, err)
	}
	body, err := decompressDetail(detail)
	if err != nil {
		t.Fatalf("decompressDetail(%q) error = %v", query, err)
	}
	r := &model.Result{}
	if err = json.Unmarshal(body, r); err != nil {
		t.Fatalf("unmarshal %q error = %v", query, err)
	}
	updateTime, err := parseDatabaseTime(updateValue)
	if err != nil {
		t.Fatalf("parseDatabaseTime(%q) error = %v", query, err)
	}
	return r, updateTime
}

func TestSanitizeLegacyRows(t *testing.T) {
	useTestDB(t)

	dirty := &model.Result{
		Keyword:    "lamb",
		Paraphrase: []string{"n.\n\n小羊；羔羊\n                        小羊肉", "   \n\t\n"},
	}
	clean := &model.Result{
		Keyword:    "sheep",
		Paraphrase: []string{"n. 羊，绵羊"},
	}
	dirtyCH := &model.Result{
		Keyword:    "羔羊",
		Paraphrase: []string{"lamb\n                   young sheep"},
	}
	insertLegacyRow(t, "en", "lamb", dirty)
	insertLegacyRow(t, "en", "sheep", clean)
	insertLegacyRow(t, "ch", "羔羊", dirtyCH)

	report, err := SanitizeLegacyRows(context.Background())
	if err != nil {
		t.Fatalf("SanitizeLegacyRows() error = %v", err)
	}
	if report.Scanned != 3 {
		t.Fatalf("SanitizeLegacyRows() scanned %d rows, want 3", report.Scanned)
	}
	if report.Cleaned != 2 {
		t.Fatalf("SanitizeLegacyRows() cleaned %d rows, want 2", report.Cleaned)
	}

	got, updateTime := readStoredResult(t, "en", "lamb")
	want := []string{"n.\n小羊；羔羊\n小羊肉"}
	if len(got.Paraphrase) != len(want) || got.Paraphrase[0] != want[0] {
		t.Fatalf("stored paraphrase = %q, want %q", got.Paraphrase, want)
	}
	if !updateTime.Equal(legacyUpdateTime) {
		t.Fatalf("update_time = %s, want %s (清洗不应改变内容新鲜度)", updateTime, legacyUpdateTime)
	}

	if got, _ = readStoredResult(t, "ch", "羔羊"); got.Paraphrase[0] != "lamb\nyoung sheep" {
		t.Fatalf("stored ch paraphrase = %q", got.Paraphrase)
	}

	version, err := getMetaValue(sanitizeVersionKey)
	if err != nil {
		t.Fatalf("getMetaValue() error = %v", err)
	}
	if version != strconv.Itoa(sanitizeVersion) {
		t.Fatalf("recorded version = %q, want %q", version, strconv.Itoa(sanitizeVersion))
	}
}

// 版本号记录后不应再扫描，否则每次启动都要全表遍历。
func TestSanitizeLegacyRowsSkipsWhenVersionRecorded(t *testing.T) {
	useTestDB(t)

	insertLegacyRow(t, "en", "lamb", &model.Result{Paraphrase: []string{"n.\n小羊"}})
	if err := setMetaValue(sanitizeVersionKey, strconv.Itoa(sanitizeVersion)); err != nil {
		t.Fatalf("setMetaValue() error = %v", err)
	}

	report, err := SanitizeLegacyRows(context.Background())
	if err != nil {
		t.Fatalf("SanitizeLegacyRows() error = %v", err)
	}
	if report.Scanned != 0 || report.Cleaned != 0 {
		t.Fatalf("SanitizeLegacyRows() = %+v, want an empty report", report)
	}
}

// 第二次运行必须无事可做，否则说明清洗规则不收敛，会反复重写词库。
func TestSanitizeLegacyRowsIsIdempotent(t *testing.T) {
	useTestDB(t)

	insertLegacyRow(t, "en", "lamb", &model.Result{
		Paraphrase: []string{"n.\n\n小羊；羔羊\n                        小羊肉"},
	})
	if _, err := SanitizeLegacyRows(context.Background()); err != nil {
		t.Fatalf("SanitizeLegacyRows() error = %v", err)
	}

	// 模拟词库被整体替换：版本号随meta表一起消失，任务应重新扫描
	if _, err := LiteDB.Exec("DELETE FROM meta"); err != nil {
		t.Fatalf("delete meta error = %v", err)
	}

	report, err := SanitizeLegacyRows(context.Background())
	if err != nil {
		t.Fatalf("SanitizeLegacyRows() error = %v", err)
	}
	if report.Scanned != 1 {
		t.Fatalf("SanitizeLegacyRows() scanned %d rows, want 1", report.Scanned)
	}
	if report.Cleaned != 0 {
		t.Fatalf("SanitizeLegacyRows() cleaned %d rows on the second pass, want 0", report.Cleaned)
	}
}

// 中断时不能记录版本号，否则剩余的脏数据再也不会被清洗。
func TestSanitizeLegacyRowsKeepsVersionUnsetOnCancel(t *testing.T) {
	useTestDB(t)

	insertLegacyRow(t, "en", "lamb", &model.Result{Paraphrase: []string{"n.\n小羊"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := SanitizeLegacyRows(ctx); err == nil {
		t.Fatal("SanitizeLegacyRows() with a cancelled context returned nil error")
	}
	version, err := getMetaValue(sanitizeVersionKey)
	if err != nil {
		t.Fatalf("getMetaValue() error = %v", err)
	}
	if version != "" {
		t.Fatalf("recorded version = %q, want it unset", version)
	}
}

// 扫描与写回之间如果有在线查询刷新了该行，清洗结果必须让位，
// 否则会用清洗过的旧内容覆盖刚拿到的新结果。
func TestApplySanitizedSkipsRefreshedRows(t *testing.T) {
	useTestDB(t)

	insertLegacyRow(t, "en", "lamb", &model.Result{Paraphrase: []string{"n.\n\n小羊\n   小羊肉"}})
	var original []byte
	if err := LiteDB.QueryRow("SELECT detail FROM en WHERE query = ?", "lamb").Scan(&original); err != nil {
		t.Fatalf("select original detail error = %v", err)
	}
	cleaned, changed := sanitizeDetail("en", original)
	if !changed {
		t.Fatal("sanitizeDetail() reported no change for a dirty row")
	}

	// 模拟扫描之后、写回之前发生的一次在线刷新
	fresh := &model.Result{
		BaseResult: &model.BaseResult{Query: "lamb", IsEN: true, Found: true},
		Paraphrase: []string{"n. 羔羊，小羊"},
	}
	if err := UpdateQueryCache(fresh); err != nil {
		t.Fatalf("UpdateQueryCache() error = %v", err)
	}

	applied, err := applySanitized(
		context.Background(), "en",
		[]sanitizedRow{{query: "lamb", original: original, detail: cleaned}},
	)
	if err != nil {
		t.Fatalf("applySanitized() error = %v", err)
	}
	if applied != 0 {
		t.Fatalf("applySanitized() applied %d rows, want 0", applied)
	}

	got, _ := readStoredResult(t, "en", "lamb")
	if len(got.Paraphrase) != 1 || got.Paraphrase[0] != "n. 羔羊，小羊" {
		t.Fatalf("stored paraphrase = %q, want the refreshed result", got.Paraphrase)
	}
}

// v4：旧网页对含'或&的查询会查错，三处缓存里的这类条目都要删掉，其余保留。
func TestSanitizeLegacyRowsPurgesMisescapedQueries(t *testing.T) {
	useTestDB(t)

	insertLegacyRow(t, "en", "at&t", &model.Result{Keyword: "at", Paraphrase: []string{"prep. 在"}})
	insertLegacyRow(t, "en", "rock 'n' roll", &model.Result{Paraphrase: []string{"n. 摇滚乐"}})
	insertLegacyRow(t, "en", "take", &model.Result{Paraphrase: []string{"v. 拿"}})
	insertLegacyRow(t, "ch", "你好", &model.Result{Paraphrase: []string{"hello"}})

	notFound := []string{"aren't", "qwzxvkj", "at&t", "zzzz"}
	for _, q := range notFound {
		if err := AppendNotFound(q); err != nil {
			t.Fatalf("AppendNotFound(%q) error = %v", q, err)
		}
	}
	longText := map[string]LongTextData{
		"an old horse won't forget the way": {Result: "一匹老马；#x27；我忘了路"},
		"good morning everyone":             {Result: "大家早上好"},
	}
	if err := saveLongTextCache(LONG_TEXT_CACHE_FILE, longText); err != nil {
		t.Fatalf("saveLongTextCache() error = %v", err)
	}

	report, err := SanitizeLegacyRows(context.Background())
	if err != nil {
		t.Fatalf("SanitizeLegacyRows() error = %v", err)
	}
	if report.Cleaned != 5 {
		t.Fatalf("SanitizeLegacyRows() cleaned %d, want 5 (2 rows, 2 not-found lines, 1 long text)", report.Cleaned)
	}

	assertRows(t, "en", []string{"take"})
	assertRows(t, "ch", []string{"你好"})
	for q, want := range map[string]bool{"aren't": false, "at&t": false, "qwzxvkj": true, "zzzz": true} {
		line, err := CheckNotFound(q)
		if err != nil {
			t.Fatalf("CheckNotFound(%q) error = %v", q, err)
		}
		if (line > 0) != want {
			t.Fatalf("CheckNotFound(%q) = %d, want present=%v", q, line, want)
		}
	}
	stored, err := loadLongTextCache(LONG_TEXT_CACHE_FILE)
	if err != nil {
		t.Fatalf("loadLongTextCache() error = %v", err)
	}
	if _, ok := stored["an old horse won't forget the way"]; ok || len(stored) != 1 {
		t.Fatalf("long-text cache = %v, want only the entry without ' or &", stored)
	}

	// 名单改写后追加的新词要另起一行，不能粘在最后一行后面
	if err := AppendNotFound("next"); err != nil {
		t.Fatalf("AppendNotFound() error = %v", err)
	}
	if line, _ := CheckNotFound("next"); line != 3 {
		t.Fatalf("CheckNotFound(next) = %d, want 3", line)
	}

	// 再跑一遍（如词库被替换后）不能删掉别的内容
	if _, err := LiteDB.Exec("DELETE FROM meta"); err != nil {
		t.Fatalf("delete meta error = %v", err)
	}
	report, err = SanitizeLegacyRows(context.Background())
	if err != nil {
		t.Fatalf("second SanitizeLegacyRows() error = %v", err)
	}
	if report.Cleaned != 0 {
		t.Fatalf("second SanitizeLegacyRows() cleaned %d, want 0", report.Cleaned)
	}
	assertRows(t, "en", []string{"take"})
}

// 版本号已是4时不做任何删除。
func TestSanitizeLegacyRowsSkipsPurgeWhenVersionRecorded(t *testing.T) {
	useTestDB(t)

	insertLegacyRow(t, "en", "at&t", &model.Result{Paraphrase: []string{"prep. 在"}})
	if err := AppendNotFound("aren't"); err != nil {
		t.Fatalf("AppendNotFound() error = %v", err)
	}
	if err := setMetaValue(sanitizeVersionKey, strconv.Itoa(sanitizeVersion)); err != nil {
		t.Fatalf("setMetaValue() error = %v", err)
	}
	if _, err := SanitizeLegacyRows(context.Background()); err != nil {
		t.Fatalf("SanitizeLegacyRows() error = %v", err)
	}
	assertRows(t, "en", []string{"at&t"})
	if line, _ := CheckNotFound("aren't"); line == 0 {
		t.Fatal("not-found list lost aren't although the purge already ran")
	}
}

// 缓存文件都不存在时（新装用户）照常完成并记录版本号。
func TestSanitizeLegacyRowsPurgeWithoutCacheFiles(t *testing.T) {
	useTestDB(t)

	if _, err := SanitizeLegacyRows(context.Background()); err != nil {
		t.Fatalf("SanitizeLegacyRows() error = %v", err)
	}
	if version, _ := getMetaValue(sanitizeVersionKey); version != strconv.Itoa(sanitizeVersion) {
		t.Fatalf("recorded version = %q, want %d", version, sanitizeVersion)
	}
	if pkgPathExists(getNotFoundFilePath()) || pkgPathExists(LONG_TEXT_CACHE_FILE) {
		t.Fatal("the purge created cache files that did not exist")
	}
}

func assertRows(t *testing.T, table string, want []string) {
	t.Helper()
	rows, err := LiteDB.Query("SELECT query FROM " + table + " ORDER BY query")
	if err != nil {
		t.Fatalf("select %s error = %v", table, err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			t.Fatalf("scan %s error = %v", table, err)
		}
		got = append(got, q)
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows of %s = %q, want %q", table, got, want)
	}
}

func pkgPathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
