package query

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/pkg/str"
	"github.com/anaskhan96/soup"
	"go.uber.org/zap"
)

type YdResult struct {
	*model.Result
	Doc *soup.Root
}

func (r *YdResult) parseParaphrase() {
	if trans := r.Doc.Find("div", "class", "trans-container"); trans.Error == nil {
		// XXX 此处可以输出warning
		var para string
		if r.IsEN {
			for _, v := range trans.FindAll("li") {
				para = str.Simplify(v.Text())
				if para != "" {
					r.Paraphrase = append(r.Paraphrase, para)
				}
			}
		} else {
			for _, wg := range trans.FindAll("p", "class", "wordGroup") {
				para = str.Simplify(wg.FullText())
				if para != "" {
					r.Paraphrase = append(r.Paraphrase, para)
				}
			}
		}
	} else {
		zap.S().Debug("div trans-container not found")
	}
}

func (r *YdResult) parseKeyword() {
	if kwTag := r.Doc.FindStrict("span", "class", "keyword"); kwTag.Error == nil {
		r.Keyword = kwTag.Text()
	}
}

func (r *YdResult) parsePronounce() {
	r.Pronounce = make(map[string]string)
	for _, pron := range r.Doc.FindAll("span", "class", "pronounce") {
		if pron.Error != nil {
			continue
		}

		if phoneticTag := pron.Find("span"); phoneticTag.Error != nil {
			continue
		}
		nation := strings.Trim(pron.Text(), " \n")
		phonetic := strings.Trim(pron.Find("span").Text(), "[]")
		r.Pronounce[nation] = phonetic
	}
}

func (r *YdResult) parseCollins() {
	if collinsRoot := r.Doc.Find("div", "id", "collinsResult"); collinsRoot.Error == nil {
		if star := collinsRoot.Find("span", "class", "star"); star.Error == nil {
			if starVal, ok := star.Attrs()["class"]; ok {
				r.Collins.Star = parseCollinsStar(starVal)
			}
		}

		if viaRank := collinsRoot.FindStrict("span", "class", "via rank"); viaRank.Error == nil {
			r.Collins.ViaRank = viaRank.Text()
		}

		if ap := collinsRoot.FindStrict("span", "class", "additional pattern"); ap.Error == nil {
			r.Collins.AdditionalPattern = normalizeCollinsPattern(ap.Text())
		}

		// 多义词的柯林斯释义分成好几组（如 VERB USES / NOUN USES），每组又可能
		// 有多个列表，只取第一个列表会丢掉大部分释义
		for _, olRoot := range collinsRoot.FindAll("ul", "class", "ol") {
			for _, liTag := range olRoot.FindAll("li") {
				cTrans := liTag.Find("div", "class", "collinsMajorTrans")
				if cTrans.Error != nil {
					continue
				}

				transTag := cTrans.Find("p")
				if transTag.Error != nil {
					continue
				}
				transStr := str.Simplify(transTag.FullText())
				if transStr == "" {
					continue
				}
				// 词性等附加说明不是每条都有（如 is 的第一条释义），缺失时照常保留释义
				var adtStr string
				if adtTag := cTrans.Find("span", "class", "additional"); adtTag.Error == nil {
					adtStr = adtTag.Text()
				}

				if adtStr != "" {
					if transStr == adtStr {
						continue
					} else if len(transStr) > len(adtStr) && strings.HasPrefix(transStr, adtStr) {
						transStr = transStr[len(adtStr)+1:]
					}
				}
				// TODO (k): <2023-11-16> 此处如果分割中文，猜测
				// - 找到第一个中文char的index
				// - 用 /[a-zA-Z]. / 分割
				// fmt.Println(idx+1, adtStr)
				// fmt.Println(transStr)

				cExamples := liTag.FindAll("div", "class", "exampleLists")
				i := &model.CollinsItem{
					Additional:   adtStr,
					MajorTrans:   transStr,
					ExampleLists: make([][]string, 0, len(cExamples)),
				}
				r.Collins.Items = append(r.Collins.Items, i)

				for _, example := range cExamples {
					if example.Error != nil {
						continue
					}
					ps := example.FindAll("p")
					if len(ps) == 0 {
						continue
					}
					exampleEn := str.Simplify(ps[0].FullText())
					if exampleEn == "" {
						continue
					}
					exampleSlice := []string{exampleEn}
					// 部分例句没有译文，不存空串，否则渲染时末尾会多出分隔空格
					if len(ps) > 1 {
						if exampleCh := str.Simplify(ps[1].FullText()); exampleCh != "" {
							exampleSlice = append(exampleSlice, exampleCh)
						}
					}
					i.ExampleLists = append(i.ExampleLists, exampleSlice)
				}
			}
		}
	}
}

var collinsPatternComma = regexp.MustCompile(`\s*,\s*`)

// normalizeCollinsPattern 整理柯林斯的词形变化，如`(\n taking,\n took\n)`
// 整理为`taking,took`。只去掉逗号两侧的空白，`(also an)`这类说明里的空格要保留
func normalizeCollinsPattern(text string) string {
	text = strings.TrimSpace(strings.Trim(str.Simplify(text), "()"))
	return collinsPatternComma.ReplaceAllString(text, ",")
}

func (r *YdResult) parseExamples() {
	examplesRoot := r.Doc.Find("div", "id", "examplesToggle")
	if examplesRoot.Error == nil {
		r.Examples = make(map[string][][]string)
		var pendingBilingual [][]string
		for _, tab := range []string{"bilingual", "authority", "originalSound"} {
			egTabDiv := examplesRoot.Find("div", "id", tab)
			if egTabDiv.Error != nil {
				continue
			}
			lis := egTabDiv.FindAll("li")
			if len(lis) == 0 {
				continue
			}
			examples := make([][]string, 0, len(lis))
			var pairedFound bool
			for _, li := range lis {
				pTags := li.FindAll("p")
				example := make([]string, 0, 3)
				for idx, ptag := range pTags {
					if idx > 3 {
						break
					}
					example = append(example, exampleText(ptag))
				}

				// 没有句子文本的条目直接丢弃，否则会渲染出空例句
				if len(example) == 0 || example[0] == "" {
					continue
				}

				if tab == "bilingual" {
					if len(example) < 2 {
						// 结构不完整，渲染不出任何东西
						continue
					}
					paired := example[1] != ""
					if !paired && !r.IsEN {
						// 中文词条无从判断单语例句属于原文还是译文，跳过
						continue
					}
					if paired && !r.IsEN {
						example[0], example[1] = example[1], example[0]
					}
					pairedFound = pairedFound || paired
				}
				examples = append(examples, example)
			}

			// 只有原句、没有译文的双语例句先搁置：其他来源有例句时让位给它们，
			// 但如果整条词都没有别的例句，有原句也比没有强（issue #85）
			if len(examples) == 0 {
				continue
			}
			if tab == "bilingual" && !pairedFound {
				pendingBilingual = examples
				continue
			}
			r.Examples[tab[:2]] = examples
		}

		if len(r.Examples) == 0 && len(pendingBilingual) > 0 {
			r.Examples["bi"] = pendingBilingual
		}
	}
}

// exampleText 取出例句文本。有道对部分词组/语法词条只在发音链接的data-rel里
// 给出句子，<p>标签本身是空的，此时从data-rel还原（issue #85）
func exampleText(ptag soup.Root) string {
	if text := str.Simplify(ptag.FullText()); text != "" {
		return text
	}
	audio := ptag.Find("a")
	if audio.Error != nil {
		return ""
	}
	return sentenceFromDataRel(audio.Attrs()["data-rel"])
}

// sentenceFromDataRel 解析形如`Let%27s+go.&le=eng`的data-rel，取出其中的句子
func sentenceFromDataRel(dataRel string) string {
	if dataRel == "" {
		return ""
	}
	encoded, _, _ := strings.Cut(dataRel, "&")
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		zap.S().Debug("Failed to decode example audio link")
		return ""
	}
	return str.Simplify(decoded)
}

func (r *YdResult) parseMachineTrans() {
	if tcRoot := r.Doc.FindStrict("div", "class", "trans-container"); tcRoot.Error == nil {
		if prev := tcRoot.FindPrevElementSibling(); prev.Error == nil && prev.Attrs()["class"] == "wordbook-js" {
			r.MachineTrans = str.Simplify(tcRoot.FullText())
			if r.MachineTrans != "" {
				return
			}
		}
		// fmt.Printf("[Prev] Error: %v Attrs: %+v\n", prev.Error, prev.Attrs())
		// fmt.Printf("[Prev] HTML: %+v\n", prev.HTML())
	}

	if fanyiRoot := r.Doc.FindStrict("div", "id", "fanyiToggle"); fanyiRoot.Error == nil {
		ps := fanyiRoot.FindAll("p")
		if len(ps) >= 2 {
			r.MachineTrans = str.Simplify(ps[1].FullText())

			if r.MachineTrans != "" {
				return
			}
		}
	}

	if tWebRoot := r.Doc.FindStrict("div", "id", "tWebTrans"); tWebRoot.Error == nil {
		if title := tWebRoot.FindStrict("div", "class", "title"); title.Error == nil {
			r.MachineTrans = str.Simplify(title.FullText())
			if r.MachineTrans != "" {
				return
			}
		}
	}

}
