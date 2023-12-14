package config

import (
	"bytes"
	"strings"

	"github.com/BurntSushi/toml"
)

func GenerateDefaultConfig() (string, error) {
	var buf = new(bytes.Buffer)
	var err error
	encoder := toml.NewEncoder(buf)
	err = encoder.Encode(Cfg)
	if err != nil {
		return "", err
	}
	var validline int
	lines := strings.Split(buf.String(), "\n")
	for _, line := range lines {
		// prefix := strings.SplitN(line, " ", 2)[0]
		// switch prefix {
		// case "debug", "english_only", "":
		// }
		if strings.HasPrefix(line, "debug =") ||
			strings.HasPrefix(line, "enable_emoji =") {
			continue
		}
		lines[validline] = line
		validline++
	}
	return strings.Join(lines[:validline], "\n"), err
}
