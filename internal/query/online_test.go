package query

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Karmenzind/kd/internal/model"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestFetchOnlineWithOutcomes(t *testing.T) {
	primaryErr := errors.New("preferred backend failed")
	fallbackErr := errors.New("fallback backend failed")
	for _, tt := range []struct {
		name         string
		query        string
		longText     bool
		primary      fetchOutcome
		fallback     fetchOutcome
		order        []string
		found        bool
		wantErr      error
		saveFound    int
		saveNotFound int
	}{
		{
			name: "legacy word found", query: "word",
			primary: fetchFound, order: []string{"youdao"}, found: true, saveFound: 1,
		},
		{
			name: "legacy word not found stops lookup", query: "missing",
			primary: fetchNotFound, order: []string{"youdao"}, saveNotFound: 1,
		},
		{
			name: "legacy long text not found stops lookup", query: "missing text", longText: true,
			primary: fetchNotFound, order: []string{"youdao"},
		},
		{
			name: "apostrophe in word prefers jsonapi_s", query: "aren't",
			primary: fetchFound, order: []string{"jsonapi_s"}, found: true, saveFound: 1,
		},
		{
			name: "ampersand in word prefers jsonapi_s", query: "at&t",
			primary: fetchNotFound, order: []string{"jsonapi_s"}, saveNotFound: 1,
		},
		{
			name: "apostrophe in long text prefers jsonapi_s", query: "don't go", longText: true,
			primary: fetchUnknown, fallback: fetchFound, order: []string{"jsonapi_s", "youdao"},
			found: true, saveFound: 1,
		},
		{
			name: "ampersand in long text prefers jsonapi_s", query: "salt & pepper", longText: true,
			primary: fetchFound, order: []string{"jsonapi_s"}, found: true, saveFound: 1,
		},
		{
			name: "word fallback found after partial result", query: "word",
			primary: fetchUnknown, fallback: fetchFound, order: []string{"youdao", "jsonapi_s"},
			found: true, saveFound: 1,
		},
		{
			name: "word fallback not found", query: "missing",
			primary: fetchUnknown, fallback: fetchNotFound, order: []string{"youdao", "jsonapi_s"},
			saveNotFound: 1,
		},
		{
			name: "long text fallback not found", query: "missing text", longText: true,
			primary: fetchUnknown, fallback: fetchNotFound, order: []string{"youdao", "jsonapi_s"},
		},
		{
			name: "both fail with jsonapi_s preferred", query: "at&t",
			primary: fetchUnknown, fallback: fetchUnknown, order: []string{"jsonapi_s", "youdao"},
			wantErr: primaryErr,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := model.BaseResult{
				Query: tt.query, IsEN: true, IsLongText: tt.longText, IsPhrase: tt.longText,
				Found: !tt.found, Prompt: "prompt", Output: "output", History: make(chan int),
			}
			r := &model.Result{BaseResult: &base}
			initialBase := base
			wantBase := base
			wantBase.Found = tt.found
			wantResult := &model.Result{BaseResult: &wantBase}
			if tt.found {
				if tt.longText {
					wantResult.MachineTrans = "translation"
				} else {
					wantResult.Paraphrase = []string{"definition"}
				}
			}

			var order []string
			saveFoundCalls, saveNotFoundCalls := 0, 0
			backend := func(name string) func(context.Context, *model.Result) (fetchOutcome, error) {
				return func(_ context.Context, result *model.Result) (fetchOutcome, error) {
					order = append(order, name)
					outcome, err := tt.primary, primaryErr
					if name != tt.order[0] {
						outcome, err = tt.fallback, fallbackErr
						if !reflect.DeepEqual(result, &model.Result{BaseResult: &initialBase}) {
							t.Errorf("fallback received content from preferred backend: %+v, base %+v", result, result.BaseResult)
						}
					}
					switch outcome {
					case fetchFound:
						if result.IsLongText {
							result.MachineTrans = "translation"
						} else {
							result.Paraphrase = []string{"definition"}
						}
					case fetchUnknown:
						result.Keyword = "partial keyword"
						result.Pronounce = map[string]string{"US": "partial pronunciation"}
						result.Paraphrase = []string{"partial definition"}
						result.Examples = map[string][][]string{"bi": {{"partial example"}}}
						result.Collins.Star = 5
						result.Collins.ViaRank = "partial rank"
						result.Collins.AdditionalPattern = "partial pattern"
						result.Collins.Items = []*model.CollinsItem{{MajorTrans: "partial Collins translation"}}
						result.MachineTrans = "partial translation"
						return outcome, err
					}
					return outcome, nil
				}
			}
			deps := onlineDeps{
				legacy:  backend("youdao"),
				jsonapi: backend("jsonapi_s"),
				saveFound: func(saved *model.Result) {
					saveFoundCalls++
					if saved != r || !reflect.DeepEqual(saved, wantResult) {
						t.Errorf("saveFound received %+v, base %+v, want %+v, base %+v",
							saved, saved.BaseResult, wantResult, wantBase)
					}
				},
				saveNotFound: func(query string) {
					saveNotFoundCalls++
					if query != tt.query || r.Found {
						t.Errorf("saveNotFound received query %q with Found = %v", query, r.Found)
					}
				},
				total: time.Second,
			}
			if err := fetchOnlineWith(r, deps); err != tt.wantErr {
				t.Errorf("fetchOnlineWith() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(order, tt.order) {
				t.Errorf("backend calls = %v, want %v", order, tt.order)
			}
			if saveFoundCalls != tt.saveFound || saveNotFoundCalls != tt.saveNotFound {
				t.Errorf("cache calls (found, not found) = (%d, %d), want (%d, %d)",
					saveFoundCalls, saveNotFoundCalls, tt.saveFound, tt.saveNotFound)
			}
			if r.BaseResult != &base || !reflect.DeepEqual(r, wantResult) {
				t.Errorf("result = %+v, base %+v, want %+v, base %+v", r, r.BaseResult, wantResult, wantBase)
			}
		})
	}
}

func TestFetchOnlineWithSharesDeadline(t *testing.T) {
	const total = 20 * time.Millisecond
	const allowance = 250 * time.Millisecond
	abort := make(chan struct{})
	defer close(abort)
	var preferredCtx, fallbackCtx context.Context
	var preferredStarted time.Time
	var fallbackErr error
	saveCalls := 0
	deps := onlineDeps{
		legacy: func(ctx context.Context, _ *model.Result) (fetchOutcome, error) {
			preferredCtx = ctx
			preferredStarted = time.Now()
			select {
			case <-ctx.Done():
				return fetchUnknown, ctx.Err()
			case <-abort:
				return fetchUnknown, errors.New("test deadline exceeded")
			}
		},
		jsonapi: func(ctx context.Context, _ *model.Result) (fetchOutcome, error) {
			fallbackCtx, fallbackErr = ctx, ctx.Err()
			return fetchUnknown, errors.New("backup failed")
		},
		saveFound:    func(*model.Result) { saveCalls++ },
		saveNotFound: func(string) { saveCalls++ },
		total:        total,
	}
	r := &model.Result{BaseResult: &model.BaseResult{Query: "deadline", Found: true}}
	done := make(chan error, 1)
	started := time.Now()
	timer := time.NewTimer(total + allowance)
	defer timer.Stop()
	go func() { done <- fetchOnlineWith(r, deps) }()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("fetchOnlineWith() error = %v, want context.DeadlineExceeded", err)
		}
		if preferredCtx == nil {
			t.Fatal("preferred backend was never called")
		}
		if deadline, ok := preferredCtx.Deadline(); !ok || deadline.Before(started.Add(total)) || deadline.After(preferredStarted.Add(total)) {
			t.Errorf("preferred deadline = %v, want a deadline set %v from the start of the lookup", deadline, total)
		}
		if fallbackCtx == nil || fallbackCtx != preferredCtx || !errors.Is(fallbackErr, context.DeadlineExceeded) {
			t.Error("fallback did not receive the same expired context as the preferred backend")
		}
		if r.Found || saveCalls != 0 {
			t.Errorf("Found = %v, cache calls = %d, want false and zero", r.Found, saveCalls)
		}
	case <-timer.C:
		t.Fatalf("fetchOnlineWith() did not return within %v", total+allowance)
	}
}

func TestFetchOnlineWithRedactsFallbackError(t *testing.T) {
	const query = "private word / text"
	var logs bytes.Buffer
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()), zapcore.AddSync(&logs), zap.InfoLevel)
	t.Cleanup(zap.ReplaceGlobals(zap.New(core)))
	primaryErr := errors.New("youdao: preferred request failed")
	fallbackErr := errors.New("jsonapi_s: invalid response for " + query + " " + url.PathEscape(query) + " " + url.QueryEscape(query))
	deps := onlineDeps{
		legacy: func(context.Context, *model.Result) (fetchOutcome, error) {
			return fetchUnknown, primaryErr
		},
		jsonapi: func(context.Context, *model.Result) (fetchOutcome, error) {
			return fetchUnknown, fallbackErr
		},
		saveFound:    func(*model.Result) { t.Error("unexpected saveFound call") },
		saveNotFound: func(string) { t.Error("unexpected saveNotFound call") },
		total:        time.Second,
	}
	r := &model.Result{BaseResult: &model.BaseResult{Query: query}}
	if err := fetchOnlineWith(r, deps); err != primaryErr {
		t.Fatalf("fetchOnlineWith() error = %v, want %v", err, primaryErr)
	}
	message := strings.TrimSpace(logs.String())
	if strings.Contains(message, "\n") || !strings.Contains(message, "INFO") {
		t.Fatalf("logs = %q, want one Info entry", message)
	}
	if !strings.Contains(message, "jsonapi_s: invalid response") {
		t.Errorf("log message = %q, want fallback error", message)
	}
	for _, raw := range []string{query, url.PathEscape(query), url.QueryEscape(query)} {
		if strings.Contains(message, raw) {
			t.Errorf("log message = %q, contains query %q", message, raw)
		}
	}
}
