package query

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/pkg"
	"github.com/anaskhan96/soup"
	"go.uber.org/zap"
)

var ydCliLegacy = &http.Client{Timeout: 5 * time.Second}
var ydCli = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: 5 * time.Second}
var youdaoLegacyBaseURL = "http://dict.youdao.com/w"

func requestYoudaoWith(ctx context.Context, cli *http.Client, baseURL string, r *model.Result) ([]byte, error) {
	// Escape the entire query as one path segment to preserve characters such
	// as percent signs and spaces (issue #82).
	q := neturl.PathEscape(r.Query)
	reqURL := fmt.Sprintf("%s/%s/#keyfrom=dict2.top", strings.TrimRight(baseURL, "/"), q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, errors.New("youdao: invalid request URL")
	}
	if r.IsLongText {
		req.Header.Set("Upgrade-Insecure-Requests", "1")
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", "dict.youdao.com")
	req.Header.Set("User-Agent", pkg.GetRandomUA())

	resp, err := cli.Do(req)
	if err != nil {
		// The client's URL error includes the query in the request path.
		if urlErr, ok := err.(*neturl.Error); ok {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("youdao: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("youdao: HTTP status %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("youdao: read response: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("youdao: request failed: %w", err)
	}
	if len(body) == 0 {
		return nil, errors.New("youdao: empty response")
	}
	if config.Cfg.Debug {
		htmlDir := filepath.Join(run.CACHE_ROOT_PATH, "html")
		errW := os.MkdirAll(htmlDir, os.ModePerm)
		if errW == nil {
			htmlPath := filepath.Join(htmlDir, fmt.Sprintf("%s.html", r.Query))
			errW = os.WriteFile(htmlPath, body, 0666)
		}
		if errW == nil {
			zap.S().Debug("Saved Youdao HTML response")
		} else {
			zap.S().Warn("Failed to save Youdao HTML response")
		}
	}
	return body, nil
}

func parseHtml(resp string, r *model.Result) (err error) {
	return
}

// "star star5"
func parseCollinsStar(v string) (star int) {
	if strings.HasPrefix(v, "star star") && len(v) == 10 {
		intChar := v[9]
		star, _ = strconv.Atoi(string(intChar))
	}
	return
}

func FetchOnline(r *model.Result) error {
	return fetchOnlineWith(r, onlineDeps{
		legacy:  fetchLegacy,
		jsonapi: fetchJSONAPI,
		saveFound: func(r *model.Result) {
			if r.IsLongText {
				go cache.UpdateLongTextCache(r)
			} else {
				go cache.UpdateQueryCache(r)
			}
		},
		saveNotFound: func(query string) {
			go cache.AppendNotFound(query)
		},
		total: 8 * time.Second,
	})
}

// fetchLegacy queries the legacy Youdao page.
func fetchLegacy(ctx context.Context, r *model.Result) (fetchOutcome, error) {
	return fetchLegacyWith(ctx, ydCliLegacy, youdaoLegacyBaseURL, r)
}

// fetchLegacyWith is fetchLegacy with a replaceable client and base URL.
func fetchLegacyWith(ctx context.Context, cli *http.Client, baseURL string, r *model.Result) (fetchOutcome, error) {
	body, err := requestYoudaoWith(ctx, cli, baseURL, r)
	if err != nil {
		return fetchUnknown, err
	}

	doc := soup.HTMLParse(string(body))
	if doc.Error != nil {
		return fetchUnknown, fmt.Errorf("youdao: parse HTML: %w", doc.Error)
	}
	resetContent(r)
	yr := YdResult{Result: r, Doc: &doc}
	if r.IsLongText {
		yr.parseMachineTrans()
		if r.MachineTrans != "" {
			return fetchFound, nil
		}
	} else {
		yr.parseParaphrase()
		if !yr.isNotFound() {
			yr.parseKeyword()
			yr.parsePronounce()
			yr.parseCollins()
			yr.parseExamples()
			return fetchFound, nil
		}
	}

	if doc.FindStrict("", "id", "results-contents").Error == nil {
		return fetchNotFound, nil
	}
	return fetchUnknown, errors.New("youdao: unrecognized response without results-contents")
}

// func init() {
// 	ydCli = pkg.CreateHTTPClient(5)
// }

func (r *YdResult) isNotFound() bool {
	return len(r.Paraphrase) == 0
}

// fetchOnlineWith picks the preferred backend and retries the other one once.
func fetchOnlineWith(r *model.Result, deps onlineDeps) error {
	ctx, cancel := context.WithTimeout(context.Background(), deps.total)
	defer cancel()

	primary, fallback := deps.legacy, deps.jsonapi
	fallbackName := "jsonapi_s"
	if preferJSONAPI(r.Query) {
		primary, fallback = fallback, primary
		fallbackName = "youdao"
	}

	outcome, errPrimary := primary(ctx, r)
	if outcome == fetchUnknown {
		resetContent(r)
		var errFallback error
		outcome, errFallback = fallback(ctx, r)
		if outcome == fetchUnknown {
			resetContent(r)
			r.Found = false
			zap.S().Infof("%s fallback failed: %s", fallbackName, redactQuery(errFallback.Error(), r.Query))
			return errPrimary
		}
	}

	r.Found = outcome == fetchFound
	if r.Found {
		deps.saveFound(r)
	} else if !r.IsLongText {
		deps.saveNotFound(r.Query)
	}
	return nil
}

func redactQuery(message, query string) string {
	if query == "" {
		return message
	}
	return strings.NewReplacer(
		neturl.PathEscape(query), "[redacted]",
		neturl.QueryEscape(query), "[redacted]",
		query, "[redacted]",
	).Replace(message)
}
