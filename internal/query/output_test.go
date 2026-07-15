package query

import (
	"strings"
	"testing"

	"github.com/Karmenzind/kd/internal/model"
	d "github.com/Karmenzind/kd/pkg/decorate"
)

func TestPrettyFormatStableSemantics(t *testing.T) {
	d.ApplyTheme("temp")
	tests := []struct {
		name       string
		result     *model.Result
		onlyEN     bool
		brief      bool
		contains   []string
		notContain []string
		ordered    []string
	}{
		{
			name: "word brief",
			result: &model.Result{
				BaseResult: &model.BaseResult{Query: "abandon", IsEN: true},
				Keyword:    "abandon",
				Paraphrase: []string{"v. 放弃", "", "to leave behind"},
				Examples:   map[string][][]string{"bi": {{"Never abandon hope.", "永远不要放弃希望。"}}},
			},
			brief:      true,
			contains:   []string{"abandon", "放弃", "leave behind"},
			notContain: []string{"Never abandon hope."},
			ordered:    []string{"abandon", "放弃", "leave behind"},
		},
		{
			name: "long text",
			result: &model.Result{BaseResult: &model.BaseResult{
				Query: "Hello 世界", IsLongText: true, MachineTrans: "你好，世界",
			}},
			contains: []string{"Hello 世界", "你好，世界"},
			ordered:  []string{"Hello 世界", "你好，世界"},
		},
		{
			name: "partial fields",
			result: &model.Result{
				BaseResult: &model.BaseResult{Query: "partial", IsEN: true},
				Examples: map[string][][]string{
					"bi": {nil, {"only one field"}},
				},
			},
			contains:   []string{"partial"},
			notContain: []string{"only one field"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrettyFormat(tt.result, tt.onlyEN, tt.brief)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("PrettyFormat() = %q, missing %q", got, want)
				}
			}
			for _, unwanted := range tt.notContain {
				if strings.Contains(got, unwanted) {
					t.Fatalf("PrettyFormat() = %q, unexpectedly contains %q", got, unwanted)
				}
			}
			last := -1
			for _, value := range tt.ordered {
				index := strings.Index(got, value)
				if index <= last {
					t.Fatalf("PrettyFormat() = %q, %q is out of order", got, value)
				}
				last = index
			}
		})
	}
}

func TestPrettyFormatUsesPreformattedOutput(t *testing.T) {
	d.ApplyTheme("temp")
	r := &model.Result{BaseResult: &model.BaseResult{Query: "ignored", Output: "cached output"}}
	if got := PrettyFormat(r, false, false); got != "cached output" {
		t.Fatalf("PrettyFormat() = %q, want cached output", got)
	}
}

func TestDisplayExampleAndCollinsSplit(t *testing.T) {
	d.ApplyTheme("temp")
	if got := displayExample(nil, "bi", false, true); got != "" {
		t.Fatalf("displayExample(nil) = %q, want empty", got)
	}
	if got := displayExample([]string{"English only"}, "bi", false, true); got != "" {
		t.Fatalf("displayExample(partial bilingual) = %q, want empty", got)
	}

	en, zh := cutCollinsTrans("to leave something 放弃某物")
	if en != "to leave something" || zh != "放弃某物" {
		t.Fatalf("cutCollinsTrans() = (%q, %q)", en, zh)
	}
}
