package pkg

import (
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

// Desktop browser User-Agents sent with Youdao requests, copied verbatim from
// https://deviceatlas.com/blog/list-of-user-agent-strings (17 Nov 2026).
// Refresh when this date is more than a month old; see AGENTS.md.
// last update 2026.11.22
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36 Edg/156.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0.0.0 Safari/537.36 Edg/155.0.0.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:159.0) Gecko/20100101 Firefox/159.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:157.0) Gecko/20100101 Firefox/157.0",
	"Mozilla/5.0 (X11; Linux x86_64; rv:157.0) Gecko/20100101 Firefox/157.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/27.0.1 Safari/605.1.15",
}

func GetRandomUA() string {
	var r = rand.New(rand.NewSource(time.Now().Unix()))
	idx := r.Intn(len(userAgents))
	return userAgents[idx]
}

func CreateHTTPClient(timeoutsec time.Duration) *http.Client {
	c := http.Client{Timeout: timeoutsec * time.Second}
	return &c
}

// func BuildYoudaoRequest(q string) (*http.Request, error) {
// 	// r, err := http.NewRequest("GET", fmt.Sprintf("http://dict.youdao.com/search?q=%s", q), nil)
// 	r, err := http.NewRequest("GET", fmt.Sprintf("http://dict.youdao.com/w/%s/#keyfrom=dict2.top", q), nil)
// 	if err != nil {
// 		return r, err
// 	}
// 	r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
// 	// r.Header.Set("Accept-Encoding", "gzip, deflate, br")
// 	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
// 	r.Header.Set("Connection", "keep-alive")
// 	r.Header.Set("Host", "dict.youdao.com")
// 	r.Header.Set("User-Agent", getRandomUA())
// 	// "User-Agent": Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36
// 	return r, err
// }

func getHTTPClient(url string, timeout time.Duration) *http.Client {
	var cli *http.Client
	if strings.HasPrefix(url, "https") {
		cli = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	}
	cli = &http.Client{}
	if timeout > 0 {
		cli.Timeout = timeout
	}
	return cli
}

// Disable timeout
func DownloadFile(filepath string, url string) (err error) {
	return DownloadFileWithTimeout(filepath, url, 0)
}

func DownloadFileWithTimeout(filepath string, url string, timeout time.Duration) (err error) {
	client := getHTTPClient(url, timeout)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func DownloadFileWithProgress(filepath string, url string) (err error) {
	client := getHTTPClient(url, 0)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	totalBytes := resp.ContentLength
	if totalBytes < 0 {
		return fmt.Errorf("content length is unknown")
	}

	// Create a buffer to hold the data
	buffer := make([]byte, 4096)
	var downloadedBytes int64

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			_, writeErr := out.Write(buffer[:n])
			if writeErr != nil {
				return writeErr
			}
			downloadedBytes += int64(n)

			progress := float64(downloadedBytes) / float64(totalBytes) * 100
			fmt.Printf("\rDownloading... %.2f%%", progress)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	fmt.Println("\nDownload completed.")
	return nil
}
