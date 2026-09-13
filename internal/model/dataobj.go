package model

import (
	"regexp"
	"strings"

	"github.com/Karmenzind/kd/pkg/str"
	"go.uber.org/zap"
)

type CollinsItem struct {
	Additional   string     `json:"a"`
	MajorTrans   string     `json:"maj"`
	ExampleLists [][]string `json:"eg"`
	// MajorTransCh string // 备用
}

type BaseResult struct {
	Query        string
	Prompt       string
	IsEN         bool
	IsPhrase     bool
	Output       string
	Found        bool
	IsLongText   bool
	MachineTrans string
	History      chan int `json:"-"`
}

type Result struct {
	*BaseResult `json:"-"`

	Keyword    string                `json:"k"`
	Pronounce  map[string]string     `json:"pron"`
	Paraphrase []string              `json:"para"`
	Examples   map[string][][]string `json:"eg"`
	Collins    struct {              // XXX (k): <2023-11-15> 直接提到第一级
		Star              int    `json:"star"`
		ViaRank           string `json:"rank"`
		AdditionalPattern string `json:"pat"`

		Items []*CollinsItem `json:"li"`
	} `json:"co"`
}

func (r *Result) ToDaemonResponse() *DaemonResponse {
	return &DaemonResponse{
		R:    r,
		Base: r.BaseResult,
	}
}

func (r *Result) Initialize() {
	if m, e := regexp.MatchString("^[A-Za-z0-9 -.?]+$", r.Query); e == nil && m {
		r.IsEN = true
		if strings.Contains(r.Query, " ") {
			r.IsPhrase = true
		}
		zap.S().Debugf("Query: isEn: %v isPhrase: %v", r.IsEN, r.IsPhrase)
	}
}

// Sanitize 清洗释义等文本中残留的HTML空白（换行、大段缩进、不可见空格）。
// 早期版本写入的缓存条目保存的是未经清洗的原始文本，直接渲染会导致排版错乱。
func (r *Result) Sanitize() {
	if r == nil {
		return
	}

	paras := make([]string, 0, len(r.Paraphrase))
	for _, para := range r.Paraphrase {
		if lines := str.SimplifyLines(para); len(lines) > 0 {
			paras = append(paras, strings.Join(lines, "\n"))
		}
	}
	r.Paraphrase = paras

	for nation, phonetic := range r.Pronounce {
		r.Pronounce[nation] = str.Simplify(phonetic)
	}

	for _, item := range r.Collins.Items {
		if item == nil {
			continue
		}
		item.Additional = str.Simplify(item.Additional)
		item.MajorTrans = str.Simplify(item.MajorTrans)
		for _, examples := range item.ExampleLists {
			for i, e := range examples {
				examples[i] = str.Simplify(e)
			}
		}
	}

	for _, examples := range r.Examples {
		for _, example := range examples {
			for i, e := range example {
				example[i] = str.Simplify(e)
			}
		}
	}

	if r.BaseResult != nil {
		r.MachineTrans = strings.Join(str.SimplifyLines(r.MachineTrans), "\n")
	}
}
