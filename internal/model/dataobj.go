package model

import (
	"regexp"
	"strings"

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

	// 入库
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
		zap.S().Debugf("Query: isEn: %v isPhrase: %v\n", r.IsEN, r.IsPhrase)
	}
}
