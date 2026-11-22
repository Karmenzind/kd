package query

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/anaskhan96/soup"
)

func TestYdResultParsesEnglishResponse(t *testing.T) {
	html := `
<html><body>
  <span class="keyword">abandon</span>
  <div class="trans-container"><ul>
    <li> v.  give up </li>
    <li> n. abandonment </li>
  </ul></div>
  <span class="pronounce">UK <span>[əˈbændən]</span></span>
  <div id="collinsResult">
    <span class="star star5"></span>
    <span class="via rank">1234</span>
    <span class="additional pattern">(abandoned, abandoning)</span>
    <ul class="ol"><li>
      <div class="collinsMajorTrans"><span class="additional">VERB</span><p>VERB to leave behind</p></div>
      <div class="exampleLists"><p>They abandoned the car.</p><p>他们弃车而去。</p></div>
    </li></ul>
  </div>
  <div id="examplesToggle">
    <div id="bilingual"><ul><li><p>Never abandon hope.</p><p>永远不要放弃希望。</p></li></ul></div>
    <div id="authority"><ul><li><p>Authoritative example.</p></li></ul></div>
  </div>
</body></html>`

	doc := soup.HTMLParse(html)
	result := &model.Result{BaseResult: &model.BaseResult{Query: "abandon", IsEN: true}}
	yd := YdResult{Result: result, Doc: &doc}
	yd.parseKeyword()
	yd.parseParaphrase()
	yd.parsePronounce()
	yd.parseCollins()
	yd.parseExamples()

	if result.Keyword != "abandon" {
		t.Fatalf("Keyword = %q, want abandon", result.Keyword)
	}
	if want := []string{"v. give up", "n. abandonment"}; !reflect.DeepEqual(result.Paraphrase, want) {
		t.Fatalf("Paraphrase = %#v, want %#v", result.Paraphrase, want)
	}
	if len(result.Pronounce) != 1 {
		t.Fatalf("Pronounce = %#v, want one entry", result.Pronounce)
	}
	if result.Collins.Star != 5 || result.Collins.ViaRank != "1234" || len(result.Collins.Items) != 1 {
		t.Fatalf("Collins = %+v", result.Collins)
	}
	if result.Collins.Items[0].MajorTrans != "to leave behind" {
		t.Fatalf("MajorTrans = %q", result.Collins.Items[0].MajorTrans)
	}
	if want := [][]string{{"Never abandon hope.", "永远不要放弃希望。"}}; !reflect.DeepEqual(result.Examples["bi"], want) {
		t.Fatalf("bilingual examples = %#v, want %#v", result.Examples["bi"], want)
	}
	if want := [][]string{{"Authoritative example."}}; !reflect.DeepEqual(result.Examples["au"], want) {
		t.Fatalf("authority examples = %#v, want %#v", result.Examples["au"], want)
	}
}

func TestYdResultParsesChineseAndMachineTranslation(t *testing.T) {
	doc := soup.HTMLParse(`
<div class="trans-container"><p class="wordGroup"> abandon: 放弃；抛弃 </p></div>
<div id="fanyiToggle"><p>source</p><p> translated text </p></div>`)
	result := &model.Result{BaseResult: &model.BaseResult{Query: "放弃", IsEN: false}}
	yd := YdResult{Result: result, Doc: &doc}
	yd.parseParaphrase()
	yd.parseMachineTrans()

	if want := []string{"abandon: 放弃；抛弃"}; !reflect.DeepEqual(result.Paraphrase, want) {
		t.Fatalf("Paraphrase = %#v, want %#v", result.Paraphrase, want)
	}
	if result.MachineTrans != "translated text" {
		t.Fatalf("MachineTrans = %q, want translated text", result.MachineTrans)
	}
}

func TestParseCollinsStar(t *testing.T) {
	for _, tt := range []struct {
		class string
		want  int
	}{
		{class: "star star1", want: 1},
		{class: "star star5", want: 5},
		{class: "star", want: 0},
		{class: "star star12", want: 0},
	} {
		t.Run(tt.class, func(t *testing.T) {
			if got := parseCollinsStar(tt.class); got != tt.want {
				t.Fatalf("parseCollinsStar(%q) = %d, want %d", tt.class, got, tt.want)
			}
		})
	}
}

func TestYdResultHandlesMissingFields(t *testing.T) {
	doc := soup.HTMLParse(`<html><body><div id="examplesToggle"><div id="bilingual"><li><p>only one field</p></li></div></div></body></html>`)
	result := &model.Result{BaseResult: &model.BaseResult{Query: "missing", IsEN: true}}
	yd := YdResult{Result: result, Doc: &doc}

	yd.parseKeyword()
	yd.parseParaphrase()
	yd.parsePronounce()
	yd.parseCollins()
	yd.parseExamples()
	yd.parseMachineTrans()

	if !yd.isNotFound() {
		t.Fatalf("isNotFound() = false for response without paraphrases: %+v", result)
	}
	if result.Keyword != "" || len(result.Pronounce) != 0 || len(result.Collins.Items) != 0 || len(result.Examples["bi"]) != 0 {
		t.Fatalf("partial response produced unexpected data: %+v", result)
	}
}

// issue #85: 词组/语法词条的双语例句里，有道会返回<p>为空的条目，
// 句子只存在于发音链接的data-rel属性中，译文则整条缺失。
// 以下HTML结构取自 modal verb / present tense / possessive noun 的真实响应。
const audioOnlyExample = `<li>
    <p>
        <a class="sp dictvoice voice-js log-js" title="点击发音" href="#" data-rel="%s&le=eng"></a>
    </p>
    <p>
    </p>
    <p class="example-via"><a target=_blank rel="nofollow">youdao</a></p>
</li>`

func pairedExample(en string, ch string) string {
	return `<li>
    <p><span>` + en + `</span>
        <a class="sp dictvoice voice-js log-js" title="点击发音" href="#" data-rel="ignored&le=eng"></a>
    </p>
    <p><span>` + ch + `</span></p>
    <p class="example-via"><a target=_blank rel="nofollow">youdao</a></p>
</li>`
}

func parseExamplesOf(t *testing.T, query string, isEN bool, body string) map[string][][]string {
	t.Helper()

	doc := soup.HTMLParse("<html><body><div id=\"examplesToggle\">" + body + "</div></body></html>")
	result := &model.Result{BaseResult: &model.BaseResult{Query: query, IsEN: isEN}}
	yd := YdResult{Result: result, Doc: &doc}
	yd.parseExamples()
	return result.Examples
}

// modal verb / possessive noun：部分条目只有data-rel，必须还原出句子，
// 并且不能留下空例句。
func TestParseExamplesRecoversSentenceFromAudioLink(t *testing.T) {
	for _, tt := range []struct {
		query string
		body  string
		want  [][]string
	}{
		{
			query: "modal verb",
			body: `<div id="bilingual"><ul>` +
				fmt.Sprintf(audioOnlyExample, "Let%27s+look+at+this+simple+example+by+using+the+modal+verb.") +
				fmt.Sprintf(audioOnlyExample, "Sufficient+research+on+modal+verbs+will+help.") +
				pairedExample("The semantics of the English modal verb are very complicated.", "英语情态动词的语义十分复杂。") +
				`</ul></div>`,
			want: [][]string{
				{"Let's look at this simple example by using the modal verb.", "", "youdao"},
				{"Sufficient research on modal verbs will help.", "", "youdao"},
				{"The semantics of the English modal verb are very complicated.", "英语情态动词的语义十分复杂。", "youdao"},
			},
		},
		{
			query: "possessive noun",
			body: `<div id="bilingual"><ul>` +
				pairedExample("A possessive noun is formed by adding apostrophe 's' to a noun.", "所有格名词是表示拥有权的名词。") +
				fmt.Sprintf(audioOnlyExample, "In+a+sentence%2C+a+possessive+adjective+is+always+used+before+a+noun.") +
				`</ul></div>`,
			want: [][]string{
				{"A possessive noun is formed by adding apostrophe 's' to a noun.", "所有格名词是表示拥有权的名词。", "youdao"},
				{"In a sentence, a possessive adjective is always used before a noun.", "", "youdao"},
			},
		},
	} {
		t.Run(tt.query, func(t *testing.T) {
			examples := parseExamplesOf(t, tt.query, true, tt.body)
			if !reflect.DeepEqual(examples["bi"], tt.want) {
				t.Fatalf("bilingual examples = %#v, want %#v", examples["bi"], tt.want)
			}
			for _, e := range examples["bi"] {
				if e[0] == "" {
					t.Fatalf("bilingual examples = %#v, contain an entry without a sentence", examples["bi"])
				}
			}
		})
	}
}

// present tense：双语例句整个tab都没有译文，此时不能占住例句位置，
// 否则渲染出一片空行，真正有译文的原声例句反而显示不出来。
func TestParseExamplesSkipsBilingualTabWithoutTranslations(t *testing.T) {
	body := `<div id="bilingual"><ul>` +
		fmt.Sprintf(audioOnlyExample, "It+helps+children+acquire+and+practice+simple+present+tense.") +
		fmt.Sprintf(audioOnlyExample, "We+use+the+simple+present+tense+to+talk+about+facts+and+states.") +
		`</ul></div>` +
		`<div id="originalSound"><ul>` +
		pairedExample("They put it in the present tense.", "他们用的是现在时。") +
		`</ul></div>`

	examples := parseExamplesOf(t, "present tense", true, body)
	if _, ok := examples["bi"]; ok {
		t.Fatalf("bilingual tab = %#v, want it left out so the other tabs can be used", examples["bi"])
	}
	want := [][]string{{"They put it in the present tense.", "他们用的是现在时。", "youdao"}}
	if !reflect.DeepEqual(examples["or"], want) {
		t.Fatalf("originalSound examples = %#v, want %#v", examples["or"], want)
	}
}

// 空条目不能进入结果，普通单词的完整例句不受影响。
func TestParseExamplesDropsEmptyEntries(t *testing.T) {
	body := `<div id="bilingual"><ul>` +
		pairedExample("Never abandon hope.", "永远不要放弃希望。") +
		`<li><p></p><p></p><p class="example-via"><a>youdao</a></p></li>` +
		`</ul></div>`

	examples := parseExamplesOf(t, "abandon", true, body)
	want := [][]string{{"Never abandon hope.", "永远不要放弃希望。", "youdao"}}
	if !reflect.DeepEqual(examples["bi"], want) {
		t.Fatalf("bilingual examples = %#v, want %#v", examples["bi"], want)
	}
}

// 中文词条的原文/译文顺序与英文相反，交换逻辑不能被本次改动影响。
func TestParseExamplesKeepsChineseEntryOrder(t *testing.T) {
	body := `<div id="bilingual"><ul>` +
		pairedExample("放弃希望", "Never abandon hope.") +
		`</ul></div>`

	examples := parseExamplesOf(t, "放弃", false, body)
	want := [][]string{{"Never abandon hope.", "放弃希望", "youdao"}}
	if !reflect.DeepEqual(examples["bi"], want) {
		t.Fatalf("bilingual examples = %#v, want %#v", examples["bi"], want)
	}
}

func TestSentenceFromDataRel(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{in: "Let%27s+go.&le=eng", want: "Let's go."},
		{in: "Even+so%2C+we%27ll+stick+to+it.&le=eng", want: "Even so, we'll stick to it."},
		{in: "", want: ""},
		{in: "%zz&le=eng", want: ""},
	} {
		if got := sentenceFromDataRel(tt.in); got != tt.want {
			t.Fatalf("sentenceFromDataRel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// 有道对部分柯林斯释义不提供词性等附加说明（如 is 的第一条），
// 这类释义以前会被整条丢掉。HTML 取自 is 的真实页面并做了删减。
func TestParseCollinsKeepsItemsWithoutAdditional(t *testing.T) {
	doc := soup.HTMLParse(`
<div id="collinsResult"><ul class="ol">
<li><div class="collinsMajorTrans">
  <span class="collinsOrder">1.&nbsp;</span>
  <p>
  <b>Is</b> is the third person singular of the present tense of . <b>Is</b> is often added to other words and shortened to <b>-'s</b>. (be)的第三人称单数现在时
  </p></div>
</li>
<li><div class="collinsMajorTrans">
  <span class="collinsOrder">2.&nbsp;</span>
  <p>
  <span class="additional">the INTERNET DOMAIN NAME for</span>
  Iceland 冰岛
  </p></div>
</li>
<li><div class="collinsMajorTrans"><p>   </p></div></li>
</ul></div>`)
	result := &model.Result{BaseResult: &model.BaseResult{Query: "is", IsEN: true}}
	yd := YdResult{Result: result, Doc: &doc}
	yd.parseCollins()

	got := make([][2]string, 0, len(result.Collins.Items))
	for _, item := range result.Collins.Items {
		got = append(got, [2]string{item.Additional, item.MajorTrans})
	}
	want := [][2]string{
		{"", "Is is the third person singular of the present tense of . Is is often added to other words and shortened to -'s. (be)的第三人称单数现在时"},
		{"the INTERNET DOMAIN NAME for", "Iceland 冰岛"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Collins items = %q, want %q", got, want)
	}
}

// 多义词的柯林斯释义分成好几组，每组又可能有多个列表（第一个列表常常只有
// 词性标题，没有释义）。以前只读第一个列表，take 的 52 条释义只剩 2 条。
// HTML 取自 take 的真实页面并做了删减。
func TestParseCollinsReadsEveryGroupAndList(t *testing.T) {
	doc := soup.HTMLParse(`
<div id="collinsResult">
<div class="wt-container">
  <h4><span class="title">take</span><span class="star star5"></span>
  <span class="via rank">CET4 TEM4</span>
  <span class="additional pattern">(
    taking,
    took,
    taken,
    takes
    )</span></h4>
  <ul class="ol"><li><div class="collinsMajorTrans"><p>
    <span class="additional">V-T</span> You can use take followed by a noun to talk about an action. 进行
  </p></div></li></ul>
</div>
<div class="wt-container wt-collapse">
  <ul class="ol"><li><p><span class="additional">V-T</span></p></li></ul>
  <ul class="ol">
  <li><div class="collinsMajorTrans"><p>
    <span class="additional">V-T</span> If you take something, you reach out for it and hold it. 拿
  </p></div>
  <div class="exampleLists"><div class="examples"><p> Here, let me take your coat. </p><p>来，我帮你拿大衣。</p></div></div>
  <div class="exampleLists"><div class="examples"><p> Take it or leave it. </p><p>  </p></div></div>
  </li>
  </ul>
</div>
</div>`)
	result := &model.Result{BaseResult: &model.BaseResult{Query: "take", IsEN: true}}
	yd := YdResult{Result: result, Doc: &doc}
	yd.parseCollins()

	if result.Collins.Star != 5 || result.Collins.ViaRank != "CET4 TEM4" || result.Collins.AdditionalPattern != "taking,took,taken,takes" {
		t.Fatalf("Collins header = star %d, rank %q, pattern %q", result.Collins.Star, result.Collins.ViaRank, result.Collins.AdditionalPattern)
	}
	var got []string
	for _, item := range result.Collins.Items {
		got = append(got, item.MajorTrans)
	}
	want := []string{
		"You can use take followed by a noun to talk about an action. 进行",
		"If you take something, you reach out for it and hold it. 拿",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Collins items = %q, want %q", got, want)
	}
	// 没有译文的例句只存原句，不存空串
	if examples, want := result.Collins.Items[1].ExampleLists, [][]string{
		{"Here, let me take your coat.", "来，我帮你拿大衣。"},
		{"Take it or leave it."},
	}; !reflect.DeepEqual(examples, want) {
		t.Fatalf("Collins examples = %q, want %q", examples, want)
	}
}

func TestNormalizeCollinsPattern(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"(\n  taking,\n  took,\n  taken,\n  takes\n  )", "taking,took,taken,takes"},
		{"(also an)", "also an"},
		{"( plural mice )", "plural mice"},
		{"", ""},
	} {
		if got := normalizeCollinsPattern(tt.in); got != tt.want {
			t.Errorf("normalizeCollinsPattern(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFetchLegacyWithOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name        string
		query       string
		fixture     string
		body        string
		status      int
		isEN        bool
		isLongText  bool
		want        fetchOutcome
		paraphrase  []string
		translation string
	}{
		{
			name: "word found without results container", query: "abandon", isEN: true,
			body: `<div class="trans-container"><ul><li> v. give up </li></ul></div>`,
			want: fetchFound, paraphrase: []string{"v. give up"},
		},
		{
			name: "Chinese word found", query: "放弃",
			body: `<div class="trans-container"><p class="wordGroup"> abandon: 放弃 </p></div>`,
			want: fetchFound, paraphrase: []string{"abandon: 放弃"},
		},
		{
			name: "word not found", query: "qwzxvkj", isEN: true,
			fixture: "qwzxvkj.html", want: fetchNotFound,
		},
		{
			name: "word has only empty definitions", query: "blank", isEN: true,
			body: `<div id="results-contents"><div class="trans-container"><ul><li> </li></ul></div></div>`,
			want: fetchNotFound,
		},
		{
			name: "word has no results container", query: "blocked", isEN: true,
			body: `<html><body>Verify that you are human</body></html>`, want: fetchUnknown,
		},
		{
			name: "long text found", query: "hello world", isEN: true, isLongText: true,
			fixture: "hello_world.html", want: fetchFound,
			translation: "你好，世界：一个用于展示编程语言基本语法和结构的示例程序。",
		},
		{
			name: "long text not found", query: "qwzxvkj", isEN: true, isLongText: true,
			fixture: "qwzxvkj.html", want: fetchNotFound,
		},
		{
			name: "long text has no results container", query: "blocked", isEN: true, isLongText: true,
			body: `<html><body>Verify that you are human</body></html>`, want: fetchUnknown,
		},
		{
			name: "non-2xx overrides recognizable response", query: "qwzxvkj",
			fixture: "qwzxvkj.html", status: http.StatusBadGateway, want: fetchUnknown,
		},
		{name: "empty response", query: "blank", want: fetchUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			if tt.fixture != "" {
				var err error
				body, err = os.ReadFile(filepath.Join("testdata", "legacy", tt.fixture))
				if err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				_, _ = w.Write(body)
			}))
			t.Cleanup(server.Close)

			base := model.BaseResult{
				Query: tt.query, IsEN: tt.isEN, IsLongText: tt.isLongText, Found: tt.want != fetchFound,
				Prompt: "prompt", Output: "output", IsPhrase: true, History: make(chan int),
			}
			wantBase := base
			wantBase.MachineTrans = tt.translation
			r := &model.Result{BaseResult: &base}
			got, err := fetchLegacyWith(t.Context(), server.Client(), server.URL, r)

			if got != tt.want {
				t.Fatalf("fetchLegacyWith() = (%s, %v), want %s", got, err, tt.want)
			}
			if tt.want == fetchUnknown {
				if err == nil || !strings.HasPrefix(err.Error(), "youdao:") {
					t.Fatalf("error = %v, want Youdao context", err)
				}
			} else if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if !reflect.DeepEqual(r.Paraphrase, tt.paraphrase) {
				t.Errorf("Paraphrase = %q, want %q", r.Paraphrase, tt.paraphrase)
			}
			if r.BaseResult != &base || *r.BaseResult != wantBase {
				t.Errorf("BaseResult = %+v, want %+v", r.BaseResult, wantBase)
			}
		})
	}
}
