package update

import (
	"crypto/tls"
	"encoding/json"
	"errors"
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

const LATEST_RELEASE_URL = "https://github.com/Karmenzind/kd/releases/latest/download/"
const TAGLIST_URL = "https://api.github.com/repos/Karmenzind/kd/tags"

type GithubTag struct {
	Name string
	// zipball_url
	// tarball_url
}

func CompareVersions(v1, v2 string) int {
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

var LATEST_TAG_FILE = filepath.Join(cache.CACHE_ROOT_PATH, "latest_tag")

func GetLatestTag() (tag string, err error) {
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

	if err = json.Unmarshal(body, &tags); err == nil {
		if len(tags) > 0 {
			tag = tags[0].Name
			p := LATEST_TAG_FILE
			writeErr := os.WriteFile(p, []byte(tag), os.ModePerm)
			if writeErr != nil {
				zap.S().Warnf("Failed to save latest_tag: %s", writeErr)
			}
		} else {
			err = errors.New("empty response list")
		}
	}
	return
}

func GetCachedLatestTag() (tag string) {
	if !pkg.IsPathExists(LATEST_TAG_FILE) {
		return
	}
	if b, err := os.ReadFile(LATEST_TAG_FILE); err == nil {
		tag = string(b)
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
	latestTag, err := GetLatestTag()
	if err != nil {
		return
	}
	if CompareVersions(latestTag, currentTag) < 1 {
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
	err = pkg.DownloadFileWithProgress(tmpPath, url)
	if err != nil {
		zap.S().Errorf("Failed to download binary file: %s", err)
	}
	d.EchoOkay("已下载完成")

	err = moveFile(tmpPath, exepath)
	if err != nil {
		return
	} else {
		d.EchoOkay("已成功替换旧版本，更新完成")
	}
	if runtime.GOOS != "windows" {
		err = pkg.AddExecutablePermission(exepath)
		if err != nil {
			d.EchoWrong(fmt.Sprintf("修改权限失败，请手动执行`chmod +x %s`", exepath))
		}
	}
	return
}

// try sudo if needed
func moveFile(src, tgt string) (err error) {
	err = os.Rename(src, tgt)
	if err == nil {
		return
	}
	zap.S().Infof("Failed to rename binary file: %s", err)
	d.EchoWarn("更改文件名失败（%s），将尝试其他方式", err)

	if runtime.GOOS == "windows" {
		// return fmt.Errorf("文件覆盖失败，遇到权限问题（%s），请到release页面下载", err)
		// d.EchoRun("尝试覆盖源文件")
		err = replaceExecutable(tgt, src)
	} else {
		cmd := exec.Command("sudo", "mv", src, tgt)
		cmd.Stdin = os.Stdin
		d.EchoRun("覆盖文件需root权限，请输入密码")
		err = cmd.Run()
	}
	// if linkErr, ok := err.(*os.LinkError); ok {
	// 	if os.IsPermission(linkErr.Err) {
	// 		zap.S().Infof("Permission denied. Please make sure you have write access to the destination directory.")
	// 		if runtime.GOOS == "windows" {
	// 			return errors.New("文件覆盖失败，遇到权限问题，请到release页面下载")
	// 		}
	// 		cmd := exec.Command("sudo", "mv", src, tgt)
	// 		cmd.Stdin = os.Stdin
	// 		d.EchoRun("覆盖文件需root权限，请输入密码")
	// 		err = cmd.Run()
	// 	}
	// }
	return
}

func replaceExecutable(oldPath, newPath string) error {
	// 修改时改cron
	backupPath := oldPath + ".update_backup"
	err := os.Rename(oldPath, backupPath)
	if err != nil {
		return err
	}

	err = copyFile(newPath, oldPath)
	if err != nil {
		// XXX (k): <2024-01-01> 再挪回来？
		return err
	}

	removeErr := os.Remove(backupPath)
	if removeErr != nil {
		zap.S().Warnf("Failed to remove old file (%s): %s", backupPath, removeErr)
	}

	return nil
}

func copyFile(src, dest string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
