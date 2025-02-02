package ip

import (
	"encoding/json"
	"net/http"
	"strings"
)

type IPInfo struct {
	// IP       string `json:"ip"`
	// Hostname string `json:"hostname"`
	// City     string `json:"city"`
	// Region   string `json:"region"`
	Country string `json:"country"`
	// Loc      string `json:"loc"`
	// Org      string `json:"org"`
	// Postal   string `json:"postal"`
	// Timezone string `json:"timezone"`
}

// check if is China Mainland
func (i *IPInfo) IsCN() bool {
	return strings.ToUpper(i.Country) == "CN"
}

// public IP information
func getIPInfo() (*IPInfo, error) {
	resp, err := http.Get("https://ipinfo.io/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var info IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}
