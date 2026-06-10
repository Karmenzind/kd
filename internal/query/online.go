package query

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
	"github.com/Karmenzind/kd/pkg"
	"github.com/anaskhan96/soup"
	"go.uber.org/zap"
)

var ydCliLegacy = &http.Client{Timeout: 5 * time.Second}
var ydCli = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: 5 * time.Second}
var llm_client = &http.Client{Timeout: 30 * time.Second}

func requestYoudao(r *model.Result) (body []byte, err error) {
	var req *http.Request
	var url string
	var cli *http.Client
	useNewApi := false
	q := strings.ReplaceAll(r.Query, " ", "%20")
	if useNewApi {
		cli = ydCli
		url = fmt.Sprintf("https://dict.youdao.com/result?word=%s&lang=en", q)
	} else {
		cli = ydCliLegacy
		url = fmt.Sprintf("http://dict.youdao.com/w/%s/#keyfrom=dict2.top", q)
		// url = fmt.Sprintf("http://dict.youdao.com/search?q=%s", q)
	}
	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		zap.S().Errorf("Failed to create request: %s", err)
		return
	}
	if r.IsLongText {
		req.Header.Set("Upgrade-Insecure-Requests", "1")
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", "dict.youdao.com")
	req.Header.Set("User-Agent", pkg.GetRandomUA())

	resp, err := cli.Do(req)
	if err != nil {
		zap.S().Infof("[http] Failed to do request: %s", err)
		return
	}

	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		zap.S().Infof("[http] Failed to read response: %s", err)
		return
	}
	zap.S().Debugf("[http-get] query '%s' Resp len: %d Status: %v", url, len(body), resp.Status)
	if resp.StatusCode != 200 {
		zap.S().Debugf("[http-get] detail: header %+v", url, len(body), resp.Header)
	}
	if config.Cfg.Debug {
		htmlDir := filepath.Join(run.CACHE_ROOT_PATH, "html")
		errW := os.MkdirAll(htmlDir, os.ModePerm)
		if errW == nil {
			htmlPath := filepath.Join(htmlDir, fmt.Sprintf("%s.html", r.Query))
			errW = os.WriteFile(htmlPath, body, 0666)
		}
		if errW == nil {
			zap.S().Debugf("Saved '%s.html'", r.Query)
		} else {
			zap.S().Warnf("Failed to save html data for '%s': %s", r.Query, errW)
		}
	}
	return
}

func parseHtml(resp string, r *model.Result) (err error) {
	return
}

// "star star5"
func parseCollinsStar(v string) (star int) {
	if strings.HasPrefix(v, "star star") && len(v) == 10 {
		intChar := v[9]
		star, _ = strconv.Atoi(string(intChar))
	}
	return
}

// return html
func FetchOnline(r *model.Result) (err error) {
	body, err := requestYoudao(r)
	if err != nil {
		zap.S().Infof("[http-youdao] Failed to request: %s", err)
		return
	}

	doc := soup.HTMLParse(string(body))
	yr := YdResult{r, &doc}

	if r.IsLongText {
		yr.parseMachineTrans()
		if r.MachineTrans != "" {
			r.Found = true
			go cache.UpdateLongTextCache(r)
		}
		return
	}

	yr.parseParaphrase()
	if yr.isNotFound() {
		go cache.AppendNotFound(r.Query)
		return
	}

	// XXX (k): <2024-01-02> long text?
	yr.parseKeyword()
	yr.parsePronounce()
	yr.parseCollins()
	yr.parseExamples()

	r.Found = true
	go cache.UpdateQueryCache(r)
	return
}

const LLM_PROMPT_TEXT = "请将用户输入的内容翻译为中文，除了翻译结果以外不要带任何解释或多余内容"
const LLM_PROMPT_WORD = `请将用户输入的内容翻译为中文，输出格式必须严格遵守要求，不得包含多余内容：
第一行为用户想要查询的词的原文（注意大小写，例如monday应该变为Monday）
第二行为发音音标，如果英式美式发音不同，以/分隔，英式在前。除了中间的分隔符外不能有多余的/。例(please): pliːz/pliz
第三行及之后每行为单词释义，每行一种词性（n./adj./vt./vi./etc），如果两个相同词性的释义意义差别大，可以拆为多条
如果无法确定发音，此行留空；如果发音和意义都无法确定，第三行写Not found`

func FetchLLM(r *model.Result) (err error) {
	// 检查配置情况
	cfg := config.Cfg
	if cfg.LLM.Model == "" || cfg.LLM.BaseUrl == "" || cfg.LLM.ApiKey == "" {
		err_msg := "未配置"
		if cfg.LLM.BaseUrl == "" {
			err_msg += " llm.base_url"
		}
		if cfg.LLM.Model == "" {
			err_msg += " llm.model"
		}
		if cfg.LLM.ApiKey == "" {
			err_msg += " llm.api_key"
		}
		return fmt.Errorf(err_msg)
	}
	url := cfg.LLM.BaseUrl + "/chat/completions"
	model := cfg.LLM.Model
	var prompt string
	if r.IsLongText {
		prompt = LLM_PROMPT_TEXT
	} else {
		prompt = LLM_PROMPT_WORD
	}
	// 格式化消息体
	body := map[string]any{
		"model":       model,
		"stream":      false,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": prompt},
			{"role": "user", "content": r.Query},
		},
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		zap.S().Errorf("Failed to build request body: %s", err)
		return
	}
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		zap.S().Errorf("Failed to create request: %s", err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.LLM.ApiKey))

	response, err := llm_client.Do(request)
	if err != nil {
		zap.S().Errorf("[http] Failed to request: %s", err)
		return
	}
	defer response.Body.Close()

	// if response.StatusCode != http.StatusOK {
	// 	zap.S().Errorf("[http] Status: %s", response.Status)
	// }
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		zap.S().Errorf("[http] Failed to read response: %s", err)
		return
	}
	// 提取内容
	var llm_res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.Unmarshal(respBody, &llm_res); err != nil || len(llm_res.Choices) == 0 {
		var llm_err struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if e := json.Unmarshal(respBody, &llm_err); e == nil {
			zap.S().Errorf("Api response error: %s", llm_err.Error.Message)
			err = fmt.Errorf("LLM调用错误: %s", llm_err.Error.Message)
		} else {
			zap.S().Errorf("Api response error")
			err = fmt.Errorf("LLM请求返回了无法解析的数据: %s", string(respBody))
		}
		return
	}
	zap.S().Debugf("Raw LLM response: %s\n", llm_res.Choices[0].Message.Content)
	// 解析输出
	llm_output := llm_res.Choices[0].Message.Content
	if r.IsLongText {
		r.Keyword = r.Query
		r.MachineTrans = llm_output
	} else {
		lines := strings.Split(llm_output, "\n")
		for i, s := range lines {
			s = strings.TrimSpace(s)
			switch i {
			case 0:
				r.Keyword = s
			case 1:
				if len(s) != 0 {
					prons := strings.Split(s, "/")
					if len(prons) == 1 {
						r.Pronounce = map[string]string{"英": prons[0], "美": prons[0]}
					}
					if len(prons) == 2 {
						r.Pronounce = map[string]string{"英": prons[0], "美": prons[1]}
					}
				}
			default:
				r.Paraphrase = append(r.Paraphrase, s)
			}
		}
	}
	r.Found = true
	return
}

// func init() {
// 	ydCli = pkg.CreateHTTPClient(5)
// }
