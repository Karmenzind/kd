package config

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/pkg"
	"github.com/jinzhu/configor"
)

var CONFIG_PATH string

type LoggerConfig struct {
	Enable           bool   `default:"true" toml:"enable"`
	Path             string `toml:"path"`
	Level            string `default:"warn" toml:"level"`
	Stderr           bool   `default:"false" toml:"stderr"`
	RedirectToStream bool   `default:"false" toml:"redirect_to_stream"`
}

type Config struct {
	// FIXME (k): <2025-06-12 01:08>
	Debug bool `default:"false" toml:"debug"`

	// Modules      []string
	EnableEmoji         bool   `default:"true" toml:"enable_emoji"`
	Paging              bool   `default:"true" toml:"paging"`
	PagerCommand        string `toml:"pager_command"`
	EnglishOnly         bool   `default:"false" toml:"english_only"`
	Theme               string `default:"temp" toml:"theme"`
	HTTPProxy           string `toml:"http_proxy"`
	ClearScreen         bool   `toml:"clear_screen" default:"false"`
	FreqAlert           bool   `toml:"freq_alert" default:"false"`
	Brief               bool   `toml:"brief" default:"false"`
	AudioCacheMaxSizeMB uint64 `toml:"audio_cache_max_size_mb" default:"2048"`
	// MaxCached    uint   `default:"10000" toml:"max_cached"`

	Logging LoggerConfig `toml:"logging"`

	FileExists bool  `toml:"-"`
	ModTime    int64 `toml:"-"`
}

func (c *Config) CheckAndApply() (err error) {
	const maxAudioCacheSizeMB = uint64(^uint64(0)>>1) / (1 << 20)
	if c.AudioCacheMaxSizeMB > maxAudioCacheSizeMB {
		return fmt.Errorf("[audio_cache_max_size_mb] 音频缓存上限过大：%d", c.AudioCacheMaxSizeMB)
	}
	if c.HTTPProxy != "" {
		proxyRegex := `^(https?:\/\/)(?:[\w\-\.]+(?::[\w\-\.]*)?@)?(?:\d{1,3}(?:\.\d{1,3}){3}|\[[^\]]+\]|[a-zA-Z0-9\-\.]+):\d{1,5}$`
		re := regexp.MustCompile(proxyRegex)
		if !re.MatchString(c.HTTPProxy) {
			return errors.New(`[http_proxy] 代理地址格式不合法，请参考以下格式：  
  - http://127.0.0.1:8080
  - http://example.com:80
  - https://username:password@192.168.1.1:3128
  - https://[2001:db8::1]:443`)
		}
		// if !strings.HasPrefix(c.HTTPProxy, "http:") && !
		proxyUrl, err := url.Parse(c.HTTPProxy)
		if err != nil {
			return fmt.Errorf("[http_proxy] 代理地址格式不合法（%s）", err)
		}
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}

	if c.Logging.Level != "" {
		c.Logging.Level = strings.ToLower(c.Logging.Level)
		if c.Logging.Level == "warning" {
			c.Logging.Level = "warn"
		} else if !slices.Contains([]string{"debug", "info", "warn", "panic", "fatal"}, c.Logging.Level) {
			return fmt.Errorf("[logging.level] 不支持的日志等级：%s", c.Logging.Level)
		}
	}

	if pkg.HasAnyFlag("log-to-stream") {
		c.Logging.Enable = true
		c.Logging.RedirectToStream = true
	}
	return
}

var Cfg = Config{}

func configPathFor(goos, home string) string {
	if goos == "windows" {
		return filepath.Join(home, "kd.toml")
	}
	return filepath.Join(home, ".config", "kd.toml")
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	return configPathFor(runtime.GOOS, home)
}

// func getDaemonCreatetime() int64 {
// }

func parseConfig() (err error) {
	p := CONFIG_PATH
	if fileinfo, fileErr := os.Stat(p); fileErr == nil {
		Cfg.FileExists = true
		Cfg.ModTime = fileinfo.ModTime().Unix()
		err = configor.New(&configor.Config{ErrorOnUnmatchedKeys: false}).Load(&Cfg, p)
	} else {
		err = configor.New(&configor.Config{ErrorOnUnmatchedKeys: false}).Load(&Cfg)
		switch runtime.GOOS {
		// case "darwin": // MacOS
		//     Cfg.Paging = false
		case "linux":
			if run.Info.GetOSInfo().IsDebianBased {
				Cfg.Paging = false
			}
		}
	}
	return err
}

func InitConfig() error {
	CONFIG_PATH = getConfigPath()
	err := parseConfig()
	if err != nil {
		if strings.HasPrefix(err.Error(), "toml") {
			return fmt.Errorf("解析配置文件失败，请检查toml文件语法（%s）", err)
		}
		return fmt.Errorf("解析配置文件失败（%s）", err)
	}
	return Cfg.CheckAndApply()
}
