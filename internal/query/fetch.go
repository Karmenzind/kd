package query

import (
	"context"
	"strings"
	"time"

	"github.com/Karmenzind/kd/internal/model"
)

// Youdao has two backends behind one query path: the legacy HTML page and the
// jsonapi_s JSON endpoint. Users never choose between them; fetchOnlineWith
// picks a preferred backend and retries the other one once on an unknown error.

type fetchOutcome int

const (
	fetchFound    fetchOutcome = iota // content fields of r are filled, err == nil
	fetchNotFound                     // a recognizable "not found", err == nil
	fetchUnknown                      // network error, bad status, unrecognized body; err != nil
)

func (o fetchOutcome) String() string {
	switch o {
	case fetchFound:
		return "found"
	case fetchNotFound:
		return "not found"
	default:
		return "unknown"
	}
}

// onlineDeps holds what fetchOnlineWith depends on, so tests can replace the
// backends and the cache writers.
type onlineDeps struct {
	legacy       func(ctx context.Context, r *model.Result) (fetchOutcome, error)
	jsonapi      func(ctx context.Context, r *model.Result) (fetchOutcome, error)
	saveFound    func(r *model.Result) // writes the word or long-text cache
	saveNotFound func(query string)    // appends a word to the not-found list
	total        time.Duration         // deadline shared by both attempts
}

// preferJSONAPI reports whether jsonapi_s should be tried first. The legacy
// page escapes ' and & twice, so it finds nothing for aren't and returns the
// entry of "at" for at&t.
func preferJSONAPI(query string) bool {
	return strings.ContainsAny(query, "'&")
}

// resetContent clears what a backend may have filled before failing, so the
// other backend starts from an empty result. Query parameters are kept.
func resetContent(r *model.Result) {
	r.Keyword = ""
	r.Pronounce = nil
	r.Paraphrase = nil
	r.Examples = nil
	r.Collins = model.Result{}.Collins
	if r.BaseResult != nil {
		r.MachineTrans = ""
	}
}
