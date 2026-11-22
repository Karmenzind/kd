package query

import (
	"reflect"
	"testing"

	"github.com/Karmenzind/kd/internal/model"
)

func TestPreferJSONAPI(t *testing.T) {
	for _, tt := range []struct {
		query string
		want  bool
	}{
		{"take", false},
		{"look forward to", false},
		{"你好", false},
		{"aren't", true},
		{"at&t", true},
		{"rock 'n' roll", true},
		// 弯引号不在规则内：旧网页对它的表现没有实测过
		{"aren’t", false},
	} {
		if got := preferJSONAPI(tt.query); got != tt.want {
			t.Errorf("preferJSONAPI(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestResetContentKeepsQueryParameters(t *testing.T) {
	r := &model.Result{BaseResult: &model.BaseResult{
		Query:        "take",
		IsEN:         true,
		IsPhrase:     false,
		IsLongText:   true,
		Prompt:       "p",
		MachineTrans: "partial",
	}}
	r.Keyword = "take"
	r.Pronounce = map[string]string{"英": "teɪk"}
	r.Paraphrase = []string{"v. 拿"}
	r.Examples = map[string][][]string{"bi": {{"a", "b"}}}
	r.Collins.Star = 5
	r.Collins.ViaRank = "CET4"
	r.Collins.AdditionalPattern = "took"
	r.Collins.Items = []*model.CollinsItem{{MajorTrans: "x"}}

	resetContent(r)

	want := &model.Result{BaseResult: &model.BaseResult{
		Query:      "take",
		IsEN:       true,
		IsLongText: true,
		Prompt:     "p",
	}}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("resetContent() = %+v / %+v, want %+v / %+v", r, r.BaseResult, want, want.BaseResult)
	}
}

func TestResetContentWithoutBaseResult(t *testing.T) {
	r := &model.Result{Keyword: "take"}
	resetContent(r)
	if r.Keyword != "" {
		t.Fatalf("resetContent() left Keyword = %q", r.Keyword)
	}
}
