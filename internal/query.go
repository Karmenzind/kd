package internal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/core"
	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/model"
	q "github.com/Karmenzind/kd/internal/query"
	"github.com/Karmenzind/kd/internal/ui"
	"github.com/Karmenzind/kd/pkg/str"
	"go.uber.org/zap"
)

// pre-compile
// shortRegex: base regex
// longRegex: allows Chinese punctuation marks as leading characters
var (
	shortRegex = regexp.MustCompile("^[a-zA-Z0-9\u4e00-\u9fa5]")
	longRegex  = regexp.MustCompile("^[a-zA-Z0-9\u4e00-\u9fa5\u3000-\u303F]")
)

func normalizeQuery(query string, longText bool) string {
	query = str.Simplify(query)
	if !longText {
		query = strings.ToLower(query)
	}
	return query
}

func Query(query string, noCache bool, longText bool) (r *model.Result, err error) {
	return QueryWithProgress(query, noCache, longText, ui.NopProgress())
}

func QueryWithProgress(query string, noCache bool, longText bool, progress ui.Progress) (r *model.Result, err error) {
	if progress == nil {
		progress = ui.NopProgress()
	}
	// TODO (k): <2024-01-02> regexp
	query = normalizeQuery(query, longText)

	r = buildResult(query, longText)
	r.History = make(chan int, 1)

	daemonReady := make(chan error, 1)
	go func() {
		daemonReady <- daemon.EnsureReady(func() {
			progress.Update(ui.State{Query: query, Phase: ui.PhaseStarting})
		})
	}()
	progress.Update(ui.State{Query: query, Phase: ui.PhaseLocal})

	if !longText {
		core.WG.Add(1)
		go cache.CounterIncr(query, r.History)
	}

	regex := shortRegex
	prompt := "请以字母、数字或汉字开头"
	if longText {
		regex = longRegex
		prompt = "请以字母、数字、汉字或标点符号开头"
	}

	// Validate input characters
	if !regex.MatchString(query) {
		r.Found = false
		r.Prompt = prompt
		return
	}

	var inNotFound bool
	var line int
	if !longText {
		line, err = cache.CheckNotFound(r.Query)
		if err != nil {
			zap.S().Warnf("[cache] check not found error: %s", err)
		} else if line > 0 {
			if !noCache {
				r.Found = false
				zap.S().Debugf("`%s` is in not-found-list", r.Query)
				return
			}
			inNotFound = true
		}
		r.Initialize()
	}

	if !noCache {
		if cacheErr := q.FetchCached(r); cacheErr == nil && r.Found {
			return
		}
	}

	progress.Update(ui.State{Query: query, Phase: ui.PhaseRemote})
	if err = <-daemonReady; err != nil {
		return r, fmt.Errorf("守护进程启动失败: %w", err)
	}
	progress.Update(ui.State{Query: query, Phase: ui.PhaseRemote})
	err = daemon.QueryDaemon(daemon.DefaultAddress(), r)
	if err == nil && r.Found && inNotFound {
		go cache.RemoveNotFound(r.Query)
	}

	return r, err
}
