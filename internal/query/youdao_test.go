package query

import (
	"reflect"
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
