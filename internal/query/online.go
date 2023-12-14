package query

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/pkg"
	"github.com/Karmenzind/kd/pkg/str"
	"github.com/anaskhan96/soup"
	"go.uber.org/zap"
)

var ydCli *http.Client

func parseHtml(resp string, r *model.Result) (err error) {
	return
}

// "star star5"
func parseCollinsStar(v string) (star int) {
	if strings.HasPrefix(v, "star star") && len(v) == 10 {
		intChar := v[9]
		star, _ = strconv.Atoi(string(intChar))
	}
	return
}

// return html
func FetchOnline(r *model.Result) (err error) {
    req, err := pkg.BuildYoudaoRequest(r.Query)
    if err != nil {
        zap.S().Errorf("Failed to create request: %s", err)
        return err
    }
    resp, err := ydCli.Do(req)
    if err != nil {
        zap.S().Infof("[http] Failed to do request: %s", err)
        return err
    }

    defer resp.Body.Close()
    body, err := io.ReadAll(resp.Body)
    if err != nil  {
        zap.S().Infof("[http] Failed to read response: %s", err)
        return err
    }
	zap.S().Debugf("[http-get] query '%s' Resp len: %s", r.Query, len(body))
	// zap.S().Debugf("[http-get] query '%s' Resp : %s", r.Query, string(body))
	if config.Cfg.Debug {
		os.WriteFile(fmt.Sprintf("data/%s.html", r.Query), (body), 0666)
	}

	// body, err := os.ReadFile("data/rank.html")
	// resp := string(body)

	doc := soup.HTMLParse(string(body))

	// paraphrase
	// --------------------------------------------
	trans := doc.Find("div", "class", "trans-container")
	if trans.Error == nil {
		// XXX 此处可以输出warning
		if r.IsEN {
			for _, v := range trans.FindAll("li") {
				r.Paraphrase = append(r.Paraphrase, str.Simplify(v.Text()))
				zap.S().Debugf("Got para: %s\n", str.Simplify(v.Text()))
			}
		} else {
			for _, wg := range trans.FindAll("p", "class", "wordGroup") {
				r.Paraphrase = append(r.Paraphrase, str.Simplify(wg.FullText()))
				zap.S().Debugf("Got para: %s\n", str.Simplify(wg.FullText()))
			}
		}
	} else {
		zap.S().Debug("div trans-container not found\n")
	}
	if r.Paraphrase == nil || len(r.Paraphrase) == 0 {
        go cache.AppendNotFound(r.Query)
		r.Found = false
		return
	}

	// result keyword
	kwTag := doc.FindStrict("span", "class", "keyword")
	if kwTag.Error == nil {
		r.Keyword = kwTag.Text()
	}

	// pronounce
	// --------------------------------------------
	r.Pronounce = make(map[string]string)
	for _, pron := range doc.FindAll("span", "class", "pronounce") {
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

	// collins
	// --------------------------------------------
	collinsRoot := doc.Find("div", "id", "collinsResult")
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

	examplesRoot := doc.Find("div", "id", "examplesToggle")
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

	r.Found = true
    go cache.UpdateQueryCache(r)

	return
}

func init() {
	ydCli = pkg.CreateHTTPClient(5)
}
