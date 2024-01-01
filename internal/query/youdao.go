package query

import (
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
	trans := r.Doc.Find("div", "class", "trans-container")
	if trans.Error == nil {
		// XXX 此处可以输出warning
		var para string
		if r.IsEN {
			for _, v := range trans.FindAll("li") {
				para = str.Simplify(v.Text())
				if para != "" {
					r.Paraphrase = append(r.Paraphrase, para)
					zap.S().Debugf("Got para: %s\n", para)
				}
			}
		} else {
			for _, wg := range trans.FindAll("p", "class", "wordGroup") {
				para = str.Simplify(wg.FullText())
				if para != "" {
					r.Paraphrase = append(r.Paraphrase, para)
					zap.S().Debugf("Got para: %s\n", para)
				}
			}
		}
	} else {
		zap.S().Debug("div trans-container not found\n")
	}
}

func (r *YdResult) parseKeyword() {
	kwTag := r.Doc.FindStrict("span", "class", "keyword")
	if kwTag.Error == nil {
		r.Keyword = kwTag.Text()
	}
}

func (r *YdResult) parsePronounce() {
	r.Pronounce = make(map[string]string)
	for _, pron := range r.Doc.FindAll("span", "class", "pronounce") {
		if pron.Error != nil {
			continue
		}

		phoneticTag := pron.Find("span")
		if phoneticTag.Error != nil {
			continue
		}
		nation := strings.Trim(pron.Text(), " \n")
		phonetic := strings.Trim(pron.Find("span").Text(), "[]")
		r.Pronounce[nation] = phonetic
	}
}

func (r *YdResult) parseCollins() {
	collinsRoot := r.Doc.Find("div", "id", "collinsResult")
	if collinsRoot.Error == nil {
		star := collinsRoot.Find("span", "class", "star")
		if star.Error == nil {
			if starVal, ok := star.Attrs()["class"]; ok {
				r.Collins.Star = parseCollinsStar(starVal)
			}
		}

		viaRank := collinsRoot.FindStrict("span", "class", "via rank")
		if viaRank.Error == nil {
			r.Collins.ViaRank = viaRank.Text()
		}

		ap := collinsRoot.FindStrict("span", "class", "additional pattern")
		if ap.Error == nil {
			apText := ap.Text()
			if apText != "" {
				apText = strings.ReplaceAll(apText, "\n", "")
				apText = regexp.MustCompile("[ \t]+").ReplaceAllString(apText, "")
				apText = strings.Trim(apText, "()")
				r.Collins.AdditionalPattern = apText
			}
		}

		olRoot := collinsRoot.Find("ul", "class", "ol")
		if olRoot.Error == nil {
			for _, liTag := range olRoot.FindAll("li") {
				cTrans := liTag.Find("div", "class", "collinsMajorTrans")
				if cTrans.Error != nil {
					continue
				}

				adtTag := cTrans.Find("span", "class", "additional")

				transTag := cTrans.Find("p")
				if adtTag.Error != nil || transTag.Error != nil {
					continue
				}
				adtStr := adtTag.Text()
				transStr := str.Simplify(transTag.FullText())

				if adtStr != "" {
					transStr = transStr[len(adtStr)+1:]
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
					if len(ps) > 0 {
						exampleEn := str.Simplify(ps[0].FullText())
						exampleSlice := []string{exampleEn}
						if len(ps) > 1 {
							exampleCh := str.Simplify(ps[1].FullText())
							exampleSlice = append(exampleSlice, exampleCh)
						}
						i.ExampleLists = append(i.ExampleLists, exampleSlice)
					}
				}
			}
		}
	}
}

func (r *YdResult) parseExamples() {
	examplesRoot := r.Doc.Find("div", "id", "examplesToggle")
	if examplesRoot.Error == nil {
		r.Examples = make(map[string][][]string)
		for _, tab := range []string{"bilingual", "authority", "originalSound"} {
			egTabDiv := examplesRoot.Find("div", "id", tab)
			if egTabDiv.Error != nil {
				continue
			}
			lis := egTabDiv.FindAll("li")
			if len(lis) == 0 {
				continue
			}
			egKey := tab[:2]
			r.Examples[egKey] = make([][]string, 0, len(lis))
			for _, li := range lis {
				pTags := li.FindAll("p")
				example := make([]string, 0, 3)
				for idx, ptag := range pTags {
					if idx > 3 {
						break
					}
					example = append(example, str.Simplify(ptag.FullText()))
				}

				if tab == "bilingual" {
					if len(example) < 2 {
						continue
					}
					if !r.IsEN {
						example[0], example[1] = example[1], example[0]
					}
				}
				zap.S().Debug("Got example", example)
				r.Examples[egKey] = append(r.Examples[egKey], example)
			}
		}
	}
}

func (r *YdResult) parseMachineTrans() {
	if tcRoot := r.Doc.FindStrict("div", "class", "trans-container"); tcRoot.Error == nil {
		if prev := tcRoot.FindPrevElementSibling(); prev.Error == nil && prev.Attrs()["class"] == "wordbook-js" {
			r.MachineTrans = str.Simplify(tcRoot.FullText())
			if r.MachineTrans != "" {
				zap.S().Debug("Got Machine trans from top area: ", r.MachineTrans)
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
				zap.S().Debug("Got Machine trans from fanyiToggle: ", r.MachineTrans)
				return
			}
		}
	}

	if tWebRoot := r.Doc.FindStrict("div", "id", "tWebTrans"); tWebRoot.Error == nil {
		title := tWebRoot.FindStrict("div", "class", "title")
		if title.Error == nil {
			r.MachineTrans = str.Simplify(title.FullText())
			if r.MachineTrans != "" {
				zap.S().Debug("Got Machine trans from tWebTrans: ", r.MachineTrans)
				return
			}
		}
	}

}

func (r *YdResult) isNotFound() bool {
	if r.Paraphrase == nil || len(r.Paraphrase) == 0 {
		return true
	}
	return false
}
