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
    conn, err := net.Dial("tcp", addr)
    if err != nil {
        d.EchoFatal("与守护进程通信失败，请尝试执行`kd --daemon`，如果无法解决问题，请提交issue并上传日志")
        return err
    }
    q := model.TCPQuery{Action: "query", B: r.BaseResult}
    var j []byte
    j, err = json.Marshal(q)
    if err != nil {
        zap.S().Errorf("Failed to marshal msg (%s): %s", j, err)
        return err
    }
    zap.S().Debugf("Sending msg: %s\n", j)
    fmt.Fprint(conn, string(j)+"\n")

    message, _ := bufio.NewReader(conn).ReadBytes('\n')

    dr := r.ToDaemonResponse()
    err = json.Unmarshal(message, &dr)
    zap.S().Debugf("Message from server: %s", string(message))
    if err != nil {
        return fmt.Errorf("解析daemon返回结果失败: %s", err)
    }
    if dr.Error != "" {
        return fmt.Errorf(dr.Error)
    }
    dr.GetResult()
    return nil
}
