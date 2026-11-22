package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/pkg"
	"github.com/Karmenzind/kd/pkg/str"
	"go.uber.org/zap"
)

const jsonapiBaseURL = "https://dict.youdao.com/jsonapi_s"

// jsonapiDicts limits the response to the dictionaries kd reads.
const jsonapiDicts = `{"count":99,"dicts":[["ec","ce","collins","blng_sents_part","auth_sents_part","media_sents_part","fanyi","typos","simple"]]}`

// jsonapiClient verifies TLS. It is separate from ydCli, which skips verification.
var jsonapiClient = &http.Client{Timeout: 5 * time.Second}

// Tags removed before str.Simplify. Other angle brackets, such as <非正式>, stay.
var jsonapiInlineTag = regexp.MustCompile(`(?i)</?b>|<i>|</i>|<br\s*/?>`)

// fetchJSONAPI queries Youdao's jsonapi_s endpoint.
func fetchJSONAPI(ctx context.Context, r *model.Result) (fetchOutcome, error) {
	return fetchJSONAPIWith(ctx, jsonapiClient, jsonapiBaseURL, r)
}

// fetchJSONAPIWith is fetchJSONAPI with a replaceable client and base URL.
func fetchJSONAPIWith(ctx context.Context, cli *http.Client, baseURL string, r *model.Result) (fetchOutcome, error) {
	if cli == nil {
		cli = jsonapiClient
	}
	if baseURL == "" {
		baseURL = jsonapiBaseURL
	}
	body, err := requestJSONAPI(ctx, cli, baseURL, r.Query)
	if err != nil {
		return fetchUnknown, err
	}
	saveJSONAPIDebug(r.Query, body)
	return applyJSONAPI(body, r)
}

func requestJSONAPI(ctx context.Context, cli *http.Client, baseURL, query string) ([]byte, error) {
	params := url.Values{}
	params.Set("doctype", "json")
	params.Set("jsonversion", "4")
	params.Set("le", "en")
	params.Set("q", query)
	params.Set("dicts", jsonapiDicts)
	endpoint := strings.TrimRight(baseURL, "/") + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("jsonapi_s: request failed: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("User-Agent", pkg.GetRandomUA())

	resp, err := cli.Do(req)
	if err != nil {
		return nil, wrapJSONAPIRequestErr(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jsonapi_s: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jsonapi_s: returned %s", resp.Status)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("jsonapi_s: empty response")
	}
	return body, nil
}

// wrapJSONAPIRequestErr drops the request URL so the query word stays out of the error.
func wrapJSONAPIRequestErr(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		err = uerr.Err
	}
	return fmt.Errorf("jsonapi_s: request failed: %w", err)
}

func saveJSONAPIDebug(query string, body []byte) {
	if !config.Cfg.Debug {
		return
	}
	dir := filepath.Join(run.CACHE_ROOT_PATH, "html")
	err := os.MkdirAll(dir, os.ModePerm)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, query+".json"), body, 0666)
	}
	if err != nil {
		zap.S().Warn("Failed to save jsonapi_s response")
		return
	}
	zap.S().Debug("Saved jsonapi_s response")
}

func applyJSONAPI(body []byte, r *model.Result) (fetchOutcome, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return fetchUnknown, fmt.Errorf("jsonapi_s: parse response: %w", err)
	}
	if doc == nil {
		return fetchUnknown, fmt.Errorf("jsonapi_s: parse response: empty document")
	}
	if _, ok := doc["meta"]; !ok {
		return fetchUnknown, fmt.Errorf("jsonapi_s: response has no meta")
	}
	// jsonapi_s sometimes answers with the full entry of an unrelated word
	// (seen for 银行, 汽车, 学校), meta included; only input gives it away.
	input, ok := doc["input"].(string)
	if !ok {
		input, ok = jsonObject(doc["meta"])["input"].(string)
	}
	if ok && !strings.EqualFold(str.Simplify(input), str.Simplify(r.Query)) {
		return fetchUnknown, errors.New("jsonapi_s: response is for a different query")
	}
	if r.IsLongText {
		return applyJSONAPILongText(doc, r)
	}
	return applyJSONAPIWord(doc, r)
}

func applyJSONAPILongText(doc map[string]any, r *model.Result) (fetchOutcome, error) {
	tran := cleanJSONAPIText(jsonObject(doc["fanyi"])["tran"])
	if tran == "" {
		return fetchUnknown, fmt.Errorf("jsonapi_s: long text has no translation")
	}
	r.MachineTrans = tran
	return fetchFound, nil
}

func applyJSONAPIWord(doc map[string]any, r *model.Result) (fetchOutcome, error) {
	dictKey := "ec"
	if !r.IsEN {
		dictKey = "ce"
	}
	word := jsonObject(jsonObject(doc[dictKey])["word"])
	paras := jsonapiParaphrases(word["trs"], r.IsEN)
	if len(paras) == 0 {
		return fetchNotFound, nil
	}

	r.Paraphrase = paras
	r.Keyword = cleanJSONAPIText(word["return-phrase"])
	if r.IsEN {
		if pron := jsonapiPronounce(word); len(pron) > 0 {
			r.Pronounce = pron
		}
	}
	applyJSONAPICollins(doc, r)
	if examples := jsonapiExamples(doc, r.IsEN); len(examples) > 0 {
		r.Examples = examples
	}
	return fetchFound, nil
}

func jsonapiParaphrases(raw any, isEN bool) []string {
	var out []string
	for _, item := range jsonArray(raw) {
		obj := jsonObject(item)
		if obj == nil {
			continue
		}
		var line string
		if isEN {
			line = joinJSONAPIParts(cleanJSONAPIText(obj["pos"]), cleanJSONAPIText(obj["tran"]))
		} else {
			line = joinJSONAPIParts(cleanJSONAPIText(obj["#text"]), cleanJSONAPIText(obj["#tran"]))
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func jsonapiPronounce(word map[string]any) map[string]string {
	pron := make(map[string]string, 2)
	if uk := cleanJSONAPIText(word["ukphone"]); uk != "" {
		pron["英"] = uk
	}
	if us := cleanJSONAPIText(word["usphone"]); us != "" {
		pron["美"] = us
	}
	if len(pron) == 0 {
		return nil
	}
	return pron
}

func applyJSONAPICollins(doc map[string]any, r *model.Result) {
	var (
		star     int
		starSeen bool
		rank     string
		pattern  string
		items    []*model.CollinsItem
	)
	for _, raw := range jsonArray(jsonObject(doc["collins"])["collins_entries"]) {
		entry := jsonObject(raw)
		if entry == nil {
			continue
		}
		if !starSeen {
			if n, ok := jsonapiStar(entry["star"]); ok {
				star = n
				starSeen = true
			}
		}
		for _, braw := range jsonArray(jsonObject(entry["basic_entries"])["basic_entry"]) {
			basic := jsonObject(braw)
			if basic == nil {
				continue
			}
			if rank == "" {
				if cet := cleanJSONAPIText(basic["cet"]); cet != "" {
					rank = cet
				}
			}
			if pattern == "" {
				if pat, ok := jsonapiPattern(basic); ok {
					pattern = pat
				}
			}
		}
		for _, eraw := range jsonArray(jsonObject(entry["entries"])["entry"]) {
			for _, traw := range jsonArray(jsonObject(eraw)["tran_entry"]) {
				item := jsonObject(traw)
				if item == nil {
					continue
				}
				major := cleanJSONAPIText(item["tran"])
				if major == "" {
					continue
				}
				items = append(items, &model.CollinsItem{
					Additional:   cleanJSONAPIText(jsonObject(item["pos_entry"])["pos"]),
					MajorTrans:   major,
					ExampleLists: jsonapiExamSents(item["exam_sents"]),
				})
			}
		}
	}
	if rank == "" && r.IsEN {
		rank = jsonapiExamType(doc)
	}
	if !starSeen && rank == "" && pattern == "" && len(items) == 0 {
		return
	}
	r.Collins.Star = star
	r.Collins.ViaRank = rank
	r.Collins.AdditionalPattern = pattern
	r.Collins.Items = items
}

// jsonapiStar reads the first non-empty star string. A value that is not a
// string is ignored. A non-empty string that is not an integer stops the search.
func jsonapiStar(v any) (int, bool) {
	s := cleanJSONAPIText(v)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, true
	}
	return n, true
}

func jsonapiPattern(basic map[string]any) (string, bool) {
	var words []string
	for _, raw := range jsonArray(jsonObject(basic["wordforms"])["wordform"]) {
		if word := cleanJSONAPIText(jsonObject(raw)["word"]); word != "" {
			words = append(words, word)
		}
	}
	if len(words) == 0 {
		return "", false
	}
	// Some forms come combined, such as "was, were"; normalize like the legacy page.
	return normalizeCollinsPattern(strings.Join(words, ",")), true
}

func jsonapiExamType(doc map[string]any) string {
	var parts []string
	for _, raw := range jsonArray(jsonObject(doc["ec"])["exam_type"]) {
		if part := cleanJSONAPIText(raw); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
}

func jsonapiExamSents(raw any) [][]string {
	var out [][]string
	for _, item := range jsonArray(jsonObject(raw)["sent"]) {
		sent := jsonObject(item)
		if sent == nil {
			continue
		}
		eng := cleanJSONAPIText(sent["eng_sent"])
		if eng == "" {
			continue
		}
		row := []string{eng}
		if chn := cleanJSONAPIText(sent["chn_sent"]); chn != "" {
			row = append(row, chn)
		}
		out = append(out, row)
	}
	return out
}

func jsonapiExamples(doc map[string]any, isEN bool) map[string][][]string {
	out := make(map[string][][]string)
	if rows := jsonapiBilingual(jsonObject(doc["blng_sents_part"])["sentence-pair"], isEN); len(rows) > 0 {
		out["bi"] = rows
	}
	if rows := jsonapiAuthority(jsonObject(doc["auth_sents_part"])["sent"]); len(rows) > 0 {
		out["au"] = rows
	}
	if rows := jsonapiMedia(jsonObject(doc["media_sents_part"])["sent"]); len(rows) > 0 {
		out["or"] = rows
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func jsonapiBilingual(raw any, isEN bool) [][]string {
	var out [][]string
	for _, item := range jsonArray(raw) {
		pair := jsonObject(item)
		if pair == nil {
			continue
		}
		first := cleanJSONAPIText(pair["sentence"])
		second := cleanJSONAPIText(pair["sentence-translation"])
		if !isEN {
			first, second = second, first
		}
		if first == "" {
			continue
		}
		row := []string{first, second}
		if src := cleanJSONAPIText(pair["source"]); src != "" {
			row = append(row, src)
		}
		out = append(out, row)
	}
	return out
}

func jsonapiAuthority(raw any) [][]string {
	var out [][]string
	for _, item := range jsonArray(raw) {
		sent := jsonObject(item)
		if sent == nil {
			continue
		}
		foreign := cleanJSONAPIText(sent["foreign"])
		if foreign == "" {
			continue
		}
		row := []string{foreign}
		if src := cleanJSONAPIText(sent["source"]); src != "" {
			row = append(row, src)
		}
		out = append(out, row)
	}
	return out
}

func jsonapiMedia(raw any) [][]string {
	var out [][]string
	for _, item := range jsonArray(raw) {
		sent := jsonObject(item)
		if sent == nil {
			continue
		}
		eng := cleanJSONAPIText(sent["eng"])
		if eng == "" {
			continue
		}
		row := []string{eng, ""}
		if src := jsonapiMediaSource(sent); src != "" {
			row = append(row, src)
		}
		out = append(out, row)
	}
	return out
}

func jsonapiMediaSource(sent map[string]any) string {
	snippets := jsonArray(jsonObject(sent["snippets"])["snippet"])
	if len(snippets) == 0 {
		return ""
	}
	snip := jsonObject(snippets[0])
	source := cleanJSONAPIText(snip["source"])
	name := cleanJSONAPIText(snip["name"])
	switch {
	case source != "" && name != "":
		return source + ": " + name
	case source != "":
		return source
	default:
		return name
	}
}

func joinJSONAPIParts(a, b string) string {
	switch {
	case a != "" && b != "":
		return a + " " + b
	case a != "":
		return a
	default:
		return b
	}
}

func cleanJSONAPIText(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return str.Simplify(jsonapiInlineTag.ReplaceAllString(s, ""))
}

func jsonObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func jsonArray(v any) []any {
	a, _ := v.([]any)
	return a
}
