package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Karmenzind/kd/pkg/str"
	"github.com/jinzhu/configor"
)

var CONFIG_PATH string

type LoggerConfig struct {
	Enable bool   `default:"true" toml:"enable"`
	Path   string `toml:"path"`
	Level  string `default:"warn" toml:"level"`
	Stderr bool   `default:"false" toml:"stderr"`
}

type Config struct {
	// FIXME
	Debug bool `default:"false" toml:"debug"`

	// Modules      []string
	EnableEmoji  bool   `default:"true" toml:"enable_emoji"`
	Paging       bool   `default:"true" toml:"paging"`
	PagerCommand string `toml:"pager_command"`
	AutoClear    bool   `default:"false" toml:"auto_clear"`
	MaxCached    uint   `default:"10000" toml:"max_cached"`
	EnglishOnly  bool   `default:"false" toml:"english_only"`
	Theme        string `default:"temp" toml:"theme"`
	HTTPProxy    string `toml:"http_proxy"`
	ClearScreen  bool   `toml:"clear_screen" default:"false"`
	FreqAlert    bool   `toml:"freq_alert" default:"true"`

	Logging LoggerConfig `toml:"logging"`

	FileExists bool  `toml:"-"`
	ModTime    int64 `toml:"-"`
}

func (c *Config) CheckAndApply() (err error) {
	if c.HTTPProxy != "" {
		proxyUrl, err := url.Parse(c.HTTPProxy)
		if err != nil {
			return fmt.Errorf("[http_proxy] 代理地址格式不合法")
		}
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}
	if c.Logging.Level != "" {
		c.Logging.Level = strings.ToLower(c.Logging.Level)
		if c.Logging.Level == "warning" {
			c.Logging.Level = "warn"
		} else if !str.InSlice(c.Logging.Level, []string{"debug", "info", "panic", "fatal"}) {
			return fmt.Errorf("[logging.level] 不支持的日志等级：%s", c.Logging.Level)

		}
	}
	return
}

var Cfg = Config{}

func getConfigPath() string {
	var p string
	dirname, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		p = filepath.Join(dirname, "kd.toml")
	} else {
		p = filepath.Join(dirname, ".config", "kd.toml")
	}
	return p
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
		// 配有配置文件，部分默认值处理
		err = configor.New(&configor.Config{ErrorOnUnmatchedKeys: false}).Load(&Cfg)
		switch runtime.GOOS {
		case "darwin": //MacOS
			Cfg.Paging = false
		}
	}
	return err
}

func InitConfig() error {
	CONFIG_PATH = getConfigPath()
	err := parseConfig()
	if err != nil {
		panic(fmt.Sprintf("Failed to parse configuration file: %s", err))
	}

	err = Cfg.CheckAndApply()
	if err != nil {
		fmt.Println(err)
        os.Exit(1)
	}
	return err
}
