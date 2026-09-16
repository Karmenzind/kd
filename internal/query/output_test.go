package query

import (
	"strings"
	"testing"

	"github.com/Karmenzind/kd/internal/model"
	d "github.com/Karmenzind/kd/pkg/decorate"
	fc "github.com/fatih/color"
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

// 旧版缓存中的释义保存的是未清洗的HTML文本，含换行与大段缩进，
// 直接渲染会导致排版错乱，这里确认渲染前会被规整。
func TestPrettyFormatLegacyParaphrase(t *testing.T) {
	originalNoColor := fc.NoColor
	fc.NoColor = true
	t.Cleanup(func() { fc.NoColor = originalNoColor })

	d.ApplyTheme("temp")
	r := &model.Result{
		BaseResult: &model.BaseResult{Query: "lamb", IsEN: true},
		Keyword:    "lamb",
		Paraphrase: []string{
			"n.\n\n小羊；羔羊\n                        小羊肉；羔羊肉",
			"短语:\n\n\n                    in lamb\n                    怀着羔羊\n",
		},
	}

	got := PrettyFormat(r, false, true)
	lines := strings.Split(got, "\n")
	want := []string{"lamb", "n.", "   小羊；羔羊", "   小羊肉；羔羊肉", "短语:", "   in lamb", "   怀着羔羊"}
	if len(lines) != len(want) {
		t.Fatalf("PrettyFormat() = %q, want %d lines", got, len(want))
	}
	for i, wantLine := range want {
		if lines[i] != wantLine {
			t.Fatalf("PrettyFormat() line %d = %q, want %q", i, lines[i], wantLine)
		}
	}
}

// issue #86: 多词短语的释义是一整句话，没有词性前缀。以前无条件按首个空格切分，
// 会把空格之前的半句中文染成词性色，色块边界落在句子中间。
// 这些释义取自各短语的真实返回结果。
var phraseParaphrases = map[string]string{
	"notional word":      "实义词：在语言学中，指有具体意义的词汇，如名词、动词、形容词等。与虚词（function word）相对。",
	"pay attention":      "注意，留心：把注意力集中在某人或某事物上，常与介词 to 连用。",
	"uncountable noun":   "不可数名词：只有一种形式，没有复数形式，表示无法被数清的事物，如 water、information、furniture。",
	"modal verbs":        "情态动词：英语中一类助动词，用于修饰主动词，表达说话者对动作或状态的态度。如 can、could、may 等。",
	"conditional clause": "条件从句：在复合句中，表示条件的从句。通常由if, unless, provided that, as long as等引导。",
}

func paraphraseLine(t *testing.T, query string, paraphrase string) string {
	t.Helper()

	r := &model.Result{
		BaseResult: &model.BaseResult{Query: query, IsEN: true, Found: true},
		Keyword:    query,
		Paraphrase: []string{paraphrase},
	}
	lines := strings.Split(PrettyFormat(r, false, true), "\n")
	if len(lines) != 2 {
		t.Fatalf("PrettyFormat(%q) = %q, want a title line and a single paraphrase line", query, lines)
	}
	return lines[1]
}

// 颜色边界断言：整条释义必须落在释义色里，不能有任何一段被染成词性色。
func TestPrettyFormatPhraseParaphraseKeepsOneColor(t *testing.T) {
	withColor(t)

	for query, paraphrase := range phraseParaphrases {
		t.Run(query, func(t *testing.T) {
			got := paraphraseLine(t, query, paraphrase)
			if want := d.Para(paraphrase); got != want {
				t.Fatalf("paraphrase line = %q, want the whole definition in paraphrase color %q", got, want)
			}
		})
	}
}

// 单词查询的“词性 + 释义”着色必须保持不变。
func TestPrettyFormatWordParaphraseKeepsPropertyColor(t *testing.T) {
	withColor(t)

	for _, tt := range []struct {
		name     string
		line     string
		property string
		rest     string
	}{
		{name: "noun", line: "n. 羔羊，小羊；羊羔肉", property: "n.", rest: "羔羊，小羊；羊羔肉"},
		{name: "combined", line: "vi.,vt. 产羊羔", property: "vi.,vt.", rest: "产羊羔"},
		{name: "trailing comma", line: "vt., 打断", property: "vt.,", rest: "打断"},
		{name: "subject label", line: "[计] 计算机辅助设计", property: "[计]", rest: "计算机辅助设计"},
		{name: "cjk label", line: "【名】 （Lamb）（英）兰姆", property: "【名】", rest: "（Lamb）（英）兰姆"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := paraphraseLine(t, "lamb", tt.line)
			want := d.Property(tt.property) + " " + d.Para(tt.rest)
			if got != want {
				t.Fatalf("paraphrase line = %q, want %q", got, want)
			}
		})
	}
}

// 英文释义整句、以及单独成行的词性，行为都不能变。
func TestPrettyFormatParaphraseEdgeCases(t *testing.T) {
	withColor(t)

	if got, want := paraphraseLine(t, "lamb", "A lamb is a young sheep."), d.Para("A lamb is a young sheep."); got != want {
		t.Fatalf("english sentence = %q, want %q", got, want)
	}
	if got, want := paraphraseLine(t, "lamb", "n."), d.Property("n."); got != want {
		t.Fatalf("bare property = %q, want %q", got, want)
	}
}

// 换主题、关颜色都不能让结构跑偏：文本内容必须始终完整保留。
func TestPrettyFormatPhraseParaphraseAcrossThemes(t *testing.T) {
	originalNoColor := fc.NoColor
	t.Cleanup(func() {
		fc.NoColor = originalNoColor
		d.ApplyTheme("temp")
	})

	for _, theme := range []string{"temp", "wudao", "canvas"} {
		for _, noColor := range []bool{false, true} {
			fc.NoColor = noColor
			d.ApplyTheme(theme)
			for query, paraphrase := range phraseParaphrases {
				got := paraphraseLine(t, query, paraphrase)
				if want := d.Para(paraphrase); got != want {
					t.Fatalf("theme %s (NoColor=%v) %q: line = %q, want %q", theme, noColor, query, got, want)
				}
				if noColor && got != paraphrase {
					t.Fatalf("theme %s with color disabled: line = %q, want the plain definition %q", theme, got, paraphrase)
				}
			}
		}
	}
}

// brief与only-English两种模式下，短语释义同样不能被切开。
func TestPrettyFormatPhraseParaphraseModes(t *testing.T) {
	withColor(t)

	paraphrase := phraseParaphrases["pay attention"]
	for _, tt := range []struct {
		name   string
		onlyEN bool
		brief  bool
		isEN   bool
	}{
		{name: "full", isEN: true},
		{name: "brief", brief: true, isEN: true},
		// onlyEN对英文词条会整段跳过释义，这里用中文词条覆盖onlyEN路径
		{name: "only english, chinese entry", onlyEN: true, isEN: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &model.Result{
				BaseResult: &model.BaseResult{Query: "pay attention", IsEN: tt.isEN, Found: true},
				Keyword:    "pay attention",
				Paraphrase: []string{paraphrase},
			}
			got := PrettyFormat(r, tt.onlyEN, tt.brief)
			if !strings.Contains(got, d.Para(paraphrase)) {
				t.Fatalf("PrettyFormat(onlyEN=%v, brief=%v) = %q, want it to contain %q",
					tt.onlyEN, tt.brief, got, d.Para(paraphrase))
			}
		})
	}
}

func withColor(t *testing.T) {
	t.Helper()

	originalNoColor := fc.NoColor
	fc.NoColor = false
	d.ApplyTheme("temp")
	t.Cleanup(func() {
		fc.NoColor = originalNoColor
		d.ApplyTheme("temp")
	})
}
