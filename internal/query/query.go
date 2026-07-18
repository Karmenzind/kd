package query

// query api

import (
	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
	"go.uber.org/zap"
)

/*

TODO:
1. remove [] around pronounce
2. if not found
3.
*/

func FetchCached(r *model.Result) (err error) {
	if r.IsLongText {
		err = cache.GetLongTextCache(r)
	} else {
		err = cache.GetCachedQuery(r)
	}
	if err == nil {
		r.Found = true
		return
	}
	zap.S().Debugf("[cache] Query error: %s", err)
	r.Found = false
	return
}
