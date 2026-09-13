package model

import "testing"

func TestSanitizeIsIdempotent(t *testing.T) {
	r := &Result{
		BaseResult: &BaseResult{Query: "lamb", MachineTrans: "  多行\n\n  机器翻译 \n"},
		Pronounce:  map[string]string{"英": " [læm]\n"},
		Paraphrase: []string{
			"n.\n\n小羊；羔羊\n                        小羊肉",
			"   \n\t\n",
			"短语:\n\n\n        in lamb\n        怀着羔羊\n",
		},
		Examples: map[string][][]string{"bi": {{" a\tb \n", "中文 "}}},
	}
	r.Collins.Items = []*CollinsItem{{
		Additional:   " [N-COUNT]\n",
		MajorTrans:   "A lamb is a\n young sheep.",
		ExampleLists: [][]string{{"  eg\n", " 例句 "}},
	}}

	r.Sanitize()
	first := r.Output
	firstPara := append([]string(nil), r.Paraphrase...)
	firstTrans := r.MachineTrans
	firstMajor := r.Collins.Items[0].MajorTrans

	r.Sanitize()
	if r.Output != first {
		t.Fatalf("Output changed on second Sanitize(): %q -> %q", first, r.Output)
	}
	if len(r.Paraphrase) != len(firstPara) {
		t.Fatalf("Paraphrase length changed: %d -> %d", len(firstPara), len(r.Paraphrase))
	}
	for i := range firstPara {
		if r.Paraphrase[i] != firstPara[i] {
			t.Fatalf("Paraphrase[%d] changed: %q -> %q", i, firstPara[i], r.Paraphrase[i])
		}
	}
	if r.MachineTrans != firstTrans {
		t.Fatalf("MachineTrans changed: %q -> %q", firstTrans, r.MachineTrans)
	}
	if r.Collins.Items[0].MajorTrans != firstMajor {
		t.Fatalf("MajorTrans changed: %q -> %q", firstMajor, r.Collins.Items[0].MajorTrans)
	}
	t.Logf("stable paraphrase: %q", r.Paraphrase)
}

// 单行字段里的换行要折成空格，直接删除会把两个单词粘在一起。
func TestSanitizeKeepsWordBoundaries(t *testing.T) {
	r := &Result{
		BaseResult: &BaseResult{Query: "lamb"},
		Examples:   map[string][][]string{"bi": {{"A lamb is a young\nsheep.", "羔羊是小羊。"}}},
	}
	r.Collins.Items = []*CollinsItem{{
		MajorTrans:   "A lamb is a young\nsheep.",
		ExampleLists: [][]string{{"the leg of\nlamb"}},
	}}

	r.Sanitize()

	if got, want := r.Examples["bi"][0][0], "A lamb is a young sheep."; got != want {
		t.Fatalf("example = %q, want %q", got, want)
	}
	if got, want := r.Collins.Items[0].MajorTrans, "A lamb is a young sheep."; got != want {
		t.Fatalf("collins trans = %q, want %q", got, want)
	}
	if got, want := r.Collins.Items[0].ExampleLists[0][0], "the leg of lamb"; got != want {
		t.Fatalf("collins example = %q, want %q", got, want)
	}
}
