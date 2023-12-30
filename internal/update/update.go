package update

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/Karmenzind/kd/internal/cache"
	d "github.com/Karmenzind/kd/pkg/decorate"

	"github.com/Karmenzind/kd/pkg"
	"go.uber.org/zap"
)

// var LATEST_RELEASE_URL = "http://localhost:8901/"
var LATEST_RELEASE_URL = "https://github.com/Karmenzind/kd/releases/latest/download/"
var TAGLIST_URL = "https://api.github.com/repos/Karmenzind/kd/tags"

type GithubTag struct {
	Name string
	// zipball_url
	// tarball_url
}

func compareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	pattern := regexp.MustCompile(`(\d+).(\d+).(\d+)`)
	matches1 := pattern.FindStringSubmatch(v1)
	matches2 := pattern.FindStringSubmatch(v2)

	version1 := make([]int, 3)
	version2 := make([]int, 3)

	for i := 1; i <= 3; i++ {
		version1[i-1], _ = strconv.Atoi(matches1[i])
		version2[i-1], _ = strconv.Atoi(matches2[i])
	}

	for i := 0; i < 3; i++ {
		if version1[i] > version2[i] {
			return 1
		} else if version1[i] < version2[i] {
			return -1
		}
	}
	return 0
}

func getLatestTag() (tag string, err error) {
	req, err := http.NewRequest("GET", TAGLIST_URL, nil)
	if err != nil {
		zap.S().Infof("Error creating request: %v\n", err)
		return
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}

	resp, err := client.Do(req)
	if err != nil {
		zap.S().Infof("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		zap.S().Infof("Error reading response body: %v\n", err)
		return
	}
	zap.S().Debugf("Response Status: %s Body: %s\n", resp.Status, string(body))

	tags := []*GithubTag{}
	err = json.Unmarshal(body, &tags)
	if err == nil {
		if len(tags) > 0 {
			tag = tags[0].Name
		} else {
			err = fmt.Errorf("empty response list")
		}
	}
	return
}

func getBinaryURL() string {
	var os, arch string
	if runtime.GOOS == "darwin" {
		os = "macos"
	} else {
		os = runtime.GOOS
	}
	arch = runtime.GOARCH
	url := fmt.Sprintf("%skd_%s_%s", LATEST_RELEASE_URL, os, arch)
	if os == "windows" {
		url += ".exe"
	}
	return url
}

func GetNewerVersion(currentTag string) (tag string, err error) {
	latestTag, err := getLatestTag()
	if err != nil {
		return
	}
	if compareVersions(latestTag, currentTag) < 1 {
		zap.S().Infof("Current tag %s latest tag: %s. No need to update.", currentTag, latestTag)
		return
	}
	return latestTag, nil
}

func UpdateBinary(currentTag string) (err error) {
	_ = currentTag
	// emoji.Println(":eyes: 不好意思更新功能没写好，请手动到release下载")
	tmpPath := filepath.Join(cache.CACHE_ROOT_PATH, "kd.temp")
	url := getBinaryURL()

	var exepath string
	exepath, err = pkg.GetExecutablePath()
	if err != nil {
		return err
	}
	if strings.Contains(exepath, "go-build") {
		fmt.Println("非binary，已忽略")
		return nil
	}

	d.EchoRun(fmt.Sprintf("Start downloading: %s", url))
	// TODO (k): <2023-12-31> 调用curl
	err = pkg.DownloadFile(tmpPath, url)
	if err != nil {
		zap.S().Errorf("Failed to download binary file: %s", err)
	}
	d.EchoRun("已下载完成")
	err = moveFile(tmpPath, exepath)
	if err != nil {
		return
	} else {
		d.EchoRun("已成功覆盖")
	}
	if runtime.GOOS != "windows" {
		err = pkg.AddExecutablePermission(exepath)
		if err != nil {
			d.EchoWrong(fmt.Sprintf("修改权限失败，请手动执行`chmod +x %s`", exepath))
		}
	}
	return
	// emoji.Println(":lightning: Now we start updating the binary")
	// emoji.Println(":lightning: updating...")
	// emoji.Println(":beer: DONE :)")
}

// try sudo if needed
func moveFile(src, tgt string) (err error) {
	err = os.Rename(src, tgt)
	if err == nil {
		return
	}
	zap.S().Infof("Permission denied. Please make sure you have write access to the destination directory.")
	if runtime.GOOS == "windows" {
		return fmt.Errorf("文件覆盖失败，遇到权限问题，请到release页面下载")
	}

	cmd := exec.Command("sudo", "mv", src, tgt)
	cmd.Stdin = os.Stdin
	d.EchoRun("覆盖文件需root权限，请输入密码")
	err = cmd.Run()
	// if linkErr, ok := err.(*os.LinkError); ok {
	// 	if os.IsPermission(linkErr.Err) {
	// 		zap.S().Infof("Permission denied. Please make sure you have write access to the destination directory.")
	// 		if runtime.GOOS == "windows" {
	// 			return fmt.Errorf("文件覆盖失败，遇到权限问题，请到release页面下载")
	// 		}

	// 		cmd := exec.Command("sudo", "mv", src, tgt)
	// 		cmd.Stdin = os.Stdin
	// 		d.EchoRun("覆盖文件需root权限，请输入密码")
	// 		err = cmd.Run()
	// 	}
	// }
	return
}
