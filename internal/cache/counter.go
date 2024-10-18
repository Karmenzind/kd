package cache

import (
    "fmt"
    "path/filepath"
    "time"

    "github.com/Karmenzind/kd/internal/core"
    "github.com/Karmenzind/kd/pkg"
    "go.uber.org/zap"
)

type MonthCounter map[string]int

// XXX 后续考虑二级映射

func CounterIncr(query string, history chan int) {
    defer func() {
        if len(history) == 0 {
            history <- 0
        }
        core.WG.Done()
    }()

    n := time.Now()
    c := make(MonthCounter)
    counterPath := filepath.Join(CACHE_STAT_DIR_PATH, fmt.Sprintf("counter-%d%02d.json", n.Year(), int(n.Month())))
    if pkg.IsPathExists(counterPath) {
        err := pkg.LoadJson(counterPath, &c)
        if err != nil {
            zap.S().Warnf("Failed to load counter")
            return
        }
        // zap.S().Debugf("Loaded counter: %+v", c)
    }
    c[query] += 1
    history <- c[query]
    pkg.SaveJson(counterPath, &c)
}
