package internal

import "testing"

func TestNormalizeQuery(t *testing.T) {
	for _, tt := range []struct {
		name     string
		input    string
		longText bool
		want     string
	}{
		{name: "word lowercased", input: "  Abandon  ", want: "abandon"},
		{name: "phrase whitespace", input: "  Leave\t me \n alone  ", want: "leave me alone"},
		{name: "Chinese", input: "  放弃  ", want: "放弃"},
		{name: "Unicode punctuation", input: "。你好？", longText: true, want: "。你好？"},
		{name: "long text preserves case", input: "  Keep This Case  ", longText: true, want: "Keep This Case"},
		{name: "empty", input: " \t\n ", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeQuery(tt.input, tt.longText); got != tt.want {
				t.Fatalf("normalizeQuery(%q, %v) = %q, want %q", tt.input, tt.longText, got, tt.want)
			}
		})
	}
}

func TestBuildResult(t *testing.T) {
	r := buildResult("你好", true)
	if r.BaseResult == nil || r.Query != "你好" || !r.IsLongText {
		t.Fatalf("buildResult() = %+v", r)
	}
}
