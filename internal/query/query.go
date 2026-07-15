package query

// query api

import (
	"errors"
	"fmt"
	"net"
	"time"

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

func QueryDaemon(addr string, r *model.Result) error {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("连接daemon失败: %w", err)
	}
	defer conn.Close()
	q := model.TCPQuery{Action: "query", B: r.BaseResult}
	if err := model.WriteProtocolMessage(conn, q); err != nil {
		return fmt.Errorf("发送daemon请求失败: %w", err)
	}

	var dr model.DaemonResponse
	if err = model.NewProtocolReader(conn).Read(&dr); err != nil {
		return fmt.Errorf("解析daemon返回结果失败: %s", err)
	}
	if dr.Error != "" {
		return errors.New(dr.Error)
	}
	if dr.R == nil || dr.Base == nil {
		return errors.New("解析daemon返回结果失败: 响应缺少结果字段")
	}
	*r = *dr.GetResult()
	return nil
}
