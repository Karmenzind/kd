package cache

import (
	"context"
	"encoding/json"
	"strconv"
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
