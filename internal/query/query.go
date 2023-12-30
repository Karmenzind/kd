package query

// query api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"go.uber.org/zap"
)

/*

TODO:
1. remove [] around pronounce
2. if not found
3.
*/

func FetchCached(r *model.Result) (err error) {
	err = cache.GetCachedQuery(r)
	if err == nil {
		r.Found = true
		return
	}
	r.Found = false
	return
}

func QueryDaemon(addr string, r *model.Result) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		d.EchoFatal("与守护进程通信失败，请尝试执行`kd --daemon`，如果无法解决问题，请提交issue并上传日志")
	}
	fmt.Fprint(conn, r.Query)

	message, _ := bufio.NewReader(conn).ReadBytes('\n')

	dr := model.DaemonResponse{R: r, IsLongText: r.IsLongText}
	err = json.Unmarshal(message, &dr)
	r.Found = dr.Found
	zap.S().Debugf("Message from server: %s", string(message))
	if err != nil {
		return fmt.Errorf("解析daemon返回结果失败: %s", err)
	}
	if dr.Error != "" {
		return fmt.Errorf(dr.Error)
	}
	return nil
}
