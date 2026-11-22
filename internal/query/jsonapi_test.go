package query

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/internal/run"
)

const jsonapiDictsValue = `{"count":99,"dicts":[["ec","ce","collins","blng_sents_part","auth_sents_part","media_sents_part","fanyi","typos","simple"]]}`

func TestJSONAPIClientUsesDefaultTLS(t *testing.T) {
	if jsonapiClient.Timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", jsonapiClient.Timeout)
	}
	if jsonapiClient.Transport != nil {
		t.Fatal("jsonapi client must use the default transport")
	}
	if jsonapiClient == ydCli || jsonapiClient == ydCliLegacy {
		t.Fatal("jsonapi client aliases a legacy client")
	}
}

func TestFetchJSONAPIRequest(t *testing.T) {
	var (
		method, accept, lang, ua, upgrade string
		query                             = map[string]string{}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		accept = r.Header.Get("Accept")
		lang = r.Header.Get("Accept-Language")
		ua = r.Header.Get("User-Agent")
		upgrade = r.Header.Get("Upgrade-Insecure-Requests")
		for key, values := range r.URL.Query() {
			query[key] = strings.Join(values, ",")
		}
		_, _ = w.Write([]byte(`{"meta":{}}`))
	}))
	t.Cleanup(srv.Close)

	r := jsonapiResult("rock 'n' roll & 你好", true, true, false)
	if _, err := fetchJSONAPIWith(context.Background(), srv.Client(), srv.URL+"/", r); err != nil && !strings.Contains(err.Error(), "jsonapi_s") {
		t.Fatalf("fetchJSONAPIWith() error = %v", err)
	}
	if method != http.MethodGet {
		t.Fatalf("method = %s, want GET", method)
	}
	if accept != "application/json, text/plain, */*" {
		t.Fatalf("Accept = %q", accept)
	}
	if lang != "zh-CN,zh;q=0.9,en;q=0.8" {
		t.Fatalf("Accept-Language = %q", lang)
	}
	if ua == "" {
		t.Fatal("User-Agent is empty")
	}
	if upgrade != "" {
		t.Fatalf("Upgrade-Insecure-Requests = %q", upgrade)
	}
	for _, key := range []string{"doctype", "jsonversion", "le", "q", "dicts"} {
		if _, ok := query[key]; !ok {
			t.Fatalf("query missing %s: %#v", key, query)
		}
	}
	if query["doctype"] != "json" || query["jsonversion"] != "4" || query["le"] != "en" {
		t.Fatalf("fixed query = %#v", query)
	}
	if query["q"] != "rock 'n' roll & 你好" {
		t.Fatalf("q = %q", query["q"])
	}
	if query["dicts"] != jsonapiDictsValue {
		t.Fatalf("dicts = %q", query["dicts"])
	}
}

func TestFetchJSONAPITransportErrors(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusBadGateway)
		}))
		t.Cleanup(srv.Close)
		r := jsonapiResult("take", true, false, false)
		outcome, err := fetchJSONAPIWith(context.Background(), srv.Client(), srv.URL, r)
		assertUnknown(t, outcome, err)
		if !strings.Contains(err.Error(), "502") {
			t.Fatalf("error = %v, want status", err)
		}
		assertContentEmpty(t, r)
	})

	t.Run("empty", func(t *testing.T) {
		outcome, err := fetchJSONAPIBody(t, nil, jsonapiResult("take", true, false, false))
		assertUnknown(t, outcome, err)
		if !strings.Contains(err.Error(), "empty") {
			t.Fatalf("error = %v, want empty response", err)
		}
	})

	t.Run("canceled", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		t.Cleanup(srv.Close)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := jsonapiResult("secret-query", true, false, false)
		outcome, err := fetchJSONAPIWith(ctx, srv.Client(), srv.URL, r)
		assertUnknown(t, outcome, err)
		if !strings.Contains(err.Error(), "request failed") || strings.Contains(err.Error(), "secret-query") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		t.Cleanup(srv.Close)
		client := srv.Client()
		client.Timeout = 20 * time.Millisecond
		r := jsonapiResult("secret-query", true, false, false)
		outcome, err := fetchJSONAPIWith(context.Background(), client, srv.URL, r)
		assertUnknown(t, outcome, err)
		if !strings.Contains(err.Error(), "request failed") || strings.Contains(err.Error(), "secret-query") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("connection", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close()
		r := jsonapiResult("secret-query", true, false, false)
		outcome, err := fetchJSONAPIWith(context.Background(), &http.Client{Timeout: time.Second}, url, r)
		assertUnknown(t, outcome, err)
		if !strings.Contains(err.Error(), "request failed") || strings.Contains(err.Error(), "secret-query") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestFetchJSONAPIRejectsUnusableJSON(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "broken", body: `{`, want: "parse"},
		{name: "array", body: `[1]`, want: "parse"},
		{name: "null", body: `null`, want: "parse"},
		{name: "no meta", body: `{"ec":{"word":{"trs":[{"tran":"词"}]}}}`, want: "meta"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := jsonapiResult("take", true, false, false)
			outcome, err := fetchJSONAPIBody(t, []byte(tt.body), r)
			assertUnknown(t, outcome, err)
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
			assertContentEmpty(t, r)
		})
	}

	t.Run("null meta is not unknown", func(t *testing.T) {
		r := jsonapiResult("qwzxvkj", true, false, false)
		outcome, err := fetchJSONAPIBody(t, []byte(`{"meta":null}`), r)
		assertNotFound(t, outcome, err)
		assertContentEmpty(t, r)
	})
}

func TestFetchJSONAPIFixtures(t *testing.T) {
	t.Run("take", func(t *testing.T) {
		r := jsonapiResult("take", true, false, false)
		outcome, err := fetchJSONAPIFile(t, "take.json", r)
		assertFound(t, outcome, err)
		assertParamsKept(t, r, "take", true, false, false)
		if r.Keyword != "take" {
			t.Fatalf("Keyword = %q", r.Keyword)
		}
		if !reflect.DeepEqual(r.Pronounce, map[string]string{"英": "teɪk", "美": "teɪk"}) {
			t.Fatalf("Pronounce = %#v", r.Pronounce)
		}
		wantPara := []string{
			"v. 携带，拿走；带去，引领；使达到，提升；拿，取；移走，拿开；偷走，误拿；取材于，收集；攻占，控制；选中，买下；订阅（报纸等）；吃，服用；减去；记录，摘录；照相，摄影；量取，测定；就（座）；以…...为例；接受，收取；接纳，接待（顾客、患者等）；遭受，经受；忍受，容忍；（以某种方式）对待，处理；理解，考虑；误以为；赢得（比赛、竞赛等）；产生（感情），持有（看法）；采取（措施），采用（方法）；做，拥有；采用（形式），就任（职位）；花费，占用（时间）； 需要，要求；使用；穿（特定尺码的鞋或衣物）；容纳；授课；学习，选修（课程）；参加（考试或测验）；走（路线），乘坐（交通工具）；跨过，跳过；踢，掷；举行投票，进行民意调查；成功，奏效；（语法）需带有（某种结构）",
			"n. （一次拍摄的）镜头，场景；收入量；看法，态度；（印刷）一次排版量",
		}
		if !reflect.DeepEqual(r.Paraphrase, wantPara) {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
		if r.Collins.Star != 5 || r.Collins.ViaRank != "CET4 TEM4" || r.Collins.AdditionalPattern != "taking,took,taken,takes" {
			t.Fatalf("Collins header = star %d, rank %q, pattern %q", r.Collins.Star, r.Collins.ViaRank, r.Collins.AdditionalPattern)
		}
		if strings.Contains(r.Collins.ViaRank, "初中") {
			t.Fatalf("ViaRank used exam_type: %q", r.Collins.ViaRank)
		}
		if len(r.Collins.Items) != 45 {
			t.Fatalf("Collins items = %d, want 45 (7 cross references skipped)", len(r.Collins.Items))
		}
		for _, head := range []string{"to be taken aback", "to take up arms", "to take the cake", "to take your hat off to someone", "to be taken for a ride", "to take someone by surprise", "take my word for it"} {
			for _, item := range r.Collins.Items {
				if item.MajorTrans == head {
					t.Fatalf("kept cross reference %q", head)
				}
			}
		}
		wantFirst := colItem(
			"V-T",
			`You can use take followed by a noun to talk about an action or event, when it would also be possible to use the verb that is related to that noun. For example, you can say "she took a shower" instead of "she showered." 后接名词描述动作或事件`,
			[]string{"She was too tired to take a shower.", "她累得不想淋浴了。"},
			[]string{"Betty took a photograph of us.", "贝蒂给我们拍了一张照片。"},
		)
		if !reflect.DeepEqual(r.Collins.Items[0], wantFirst) {
			t.Fatalf("first Collins item = %#v", r.Collins.Items[0])
		}
		if strings.Contains(r.Collins.Items[0].MajorTrans, "<b>") {
			t.Fatalf("MajorTrans kept a tag: %q", r.Collins.Items[0].MajorTrans)
		}
		wantThird := colItem("V-T", "If you take something, you reach out for it and hold it. 拿",
			[]string{"Here, let me take your coat.", "来，我帮你拿大衣。"},
			[]string{"Colette took her by the shoulders and shook her.", "科利特抓住她的双肩，摇晃着她。"},
		)
		if !reflect.DeepEqual(r.Collins.Items[2], wantThird) {
			t.Fatalf("Collins item from the second group = %#v", r.Collins.Items[2])
		}
		wantExamples := map[string][][]string{
			"bi": {
				{"Let's take the kids to the fair.", "咱们带孩子们到游乐场吧。", "《牛津词典》"},
				{"Let me take your coat.", "让我帮您拿外套吧。", "《柯林斯英汉双解大词典》"},
				{"We take pride in offering you the highest standards.", "我们以向您提供最高标准的服务而自豪。", "《柯林斯英汉双解大词典》"},
			},
			"au": {
				{"The troops' mandate was to protect observers who would take notes on ceasefire violations.", "BBC: [an error occurred while processing this directive]"},
				{"My own take on it is that the music comes from a lot of different places.", "NPR: James Taylor Gets Ready for Summer Tour"},
				{"We take off in pursuit, doing a fast hike straight up the steep ridge.", "FORBES: Hat Trick at High Lonesome"},
			},
			"or": {
				{`"I will take the children on an outing as well. I will take them to the zoo or to the park.`, "", "VOA: special.2009.08.10"},
				{"And, by all means, if you are scheduled to attend class at 11:00, please take the test at 11:00.", "", "固态化学导论课程节选: 麻省理工公开课"},
				{"But this year, it's my senior year, so I'm just trying to take it easy and... take my classes.", "", "SpeakingMax英语口语达人: 写作课程比较难"},
				{"Studies have shown that the fewer medicines a person has to take the more likely he or she will take them.", "", "VOA: special.2009.04.08"},
				{"The farmer who had promised to take care of the farm while her husband was away had been lazy and dishonest.", "", "VOA: special.2009.12.12"},
			},
		}
		if !reflect.DeepEqual(r.Examples, wantExamples) {
			t.Fatalf("Examples = %#v", r.Examples)
		}
		if r.MachineTrans != "" {
			t.Fatalf("MachineTrans = %q", r.MachineTrans)
		}
	})

	t.Run("nasa", func(t *testing.T) {
		r := jsonapiResult("NASA", true, false, false)
		outcome, err := fetchJSONAPIFile(t, "nasa.json", r)
		assertFound(t, outcome, err)
		assertParamsKept(t, r, "NASA", true, false, false)
		if r.Keyword != "NASA" || !reflect.DeepEqual(r.Pronounce, map[string]string{"英": "ˈnæsə", "美": "ˈnæsə"}) {
			t.Fatalf("keyword/pronounce = %q %#v", r.Keyword, r.Pronounce)
		}
		if !reflect.DeepEqual(r.Paraphrase, []string{"abbr. 美国国家航空和宇宙航行局（National Aeronautics and Space Administration）"}) {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
		if r.Collins.Star != 2 || r.Collins.ViaRank != "" || r.Collins.AdditionalPattern != "" || len(r.Collins.Items) != 1 {
			t.Fatalf("Collins header = %+v", r.Collins)
		}
		wantItem := colItem("N-PROPER", `NASA is a U.S. government organization concerned with spacecraft and space travel. NASA is an abbreviation for "National Aeronautics and Space Administration." 美国国家航空航天局`)
		if !reflect.DeepEqual(r.Collins.Items[0], wantItem) {
			t.Fatalf("Collins item = %#v", r.Collins.Items[0])
		}
		wantExamples := map[string][][]string{
			"bi": {
				{"NASA found that certain plants could remove chemicals from the air, and even today, garden centers recommend the plants for air-cleaning properties.", "美国国家航空航天局发现某些植物可以去除空气中的化学物质，即使在今天，园艺中心也因这些植物的空气净化特性而推荐它们。"},
				{"Companies like SpaceX could help NASA and its partners when it comes to resupplying the ISS, as it suggests it can reduce launch costs by two-thirds.", "在为国际空间站提供补给方面，像 SpaceX 这样的公司可以帮到 NASA 及其合作伙伴，因为该公司表示这样可以将发射成本降低三分之二。"},
				{"Since NASA published a paper in 1989 claiming that houseplants can soak up pollution and toxic chemicals, businesses and homeowners have increasingly invested in greenery to help clean their air.", "自从1989年美国国家航空航天局发表论文声称室内植物可以吸收污染物和有毒化学物质以来，企业和房主越来越多地投资绿植以帮助净化空气。"},
			},
			"au": {
				{"Sunday's supermoon will technically be a perigee full moon, the closest full moon of the year, NASA says.", "CNN: Rare supermoon eclipse comes Sunday"},
				{"This month, the White House ordered a sweeping independent review of Nasa's manned spaceflight strategy.", "BBC: Obama names US space agency chief"},
				{"That engineer told me something that would later become symbolic of NASA's Challenger failures.", "NPR: Challenger: Reporting a Disaster's Cold, Hard Facts"},
			},
			"or": {
				{"NASA plans to send the Mars Science Laboratory to the red planet in the autumn of two thousand eleven.", "", "VOA: special.2009.12.30"},
				{"In September, NASA scientists had announced the discovery of water molecules mainly in the moon's extreme northern and southern areas.", "", "VOA: special.2009.12.30"},
				{"The NASA team still is not sure how much of what they have found is water and how much is hydroxyl.", "", "VOA: special.2009.09.30"},
			},
		}
		if !reflect.DeepEqual(r.Examples, wantExamples) {
			t.Fatalf("Examples = %#v", r.Examples)
		}
	})

	t.Run("aren't", func(t *testing.T) {
		r := jsonapiResult("aren't", true, false, false)
		outcome, err := fetchJSONAPIFile(t, "arent.json", r)
		assertFound(t, outcome, err)
		assertParamsKept(t, r, "aren't", true, false, false)
		if r.Keyword != "aren't" || !reflect.DeepEqual(r.Pronounce, map[string]string{"英": "ɑːnt", "美": "ɑːrnt"}) {
			t.Fatalf("keyword/pronounce = %q %#v", r.Keyword, r.Pronounce)
		}
		if !reflect.DeepEqual(r.Paraphrase, []string{"abbr. are not 的缩略形式，表示“不是”；口语疑问句或附加疑问句中可代替 am not（如 Aren't I?）"}) {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
		if r.Collins.Star != 4 || r.Collins.ViaRank != "" || r.Collins.AdditionalPattern != "" || len(r.Collins.Items) != 2 {
			t.Fatalf("Collins header = %+v, items %d", r.Collins, len(r.Collins.Items))
		}
		wantItems := []*model.CollinsItem{
			colItem("", `Aren't is the usual spoken form of "are not." (are not)的常用口语形式`),
			colItem("", `Aren't is the form of "am not" that is used in questions or tags in spoken English. 口语中疑问句或附加疑问句里(am not)的缩略形式`),
		}
		if !reflect.DeepEqual(r.Collins.Items, wantItems) {
			t.Fatalf("Collins items = %#v", r.Collins.Items)
		}
		if !reflect.DeepEqual(r.Examples["au"][0], []string{
			"Second, BAE's owners, as opposed to its board, aren't weeping about the collapse of the deal.",
			"BBC: How Germany killed the merger of BAE and EADS",
		}) {
			t.Fatalf("au[0] = %#v", r.Examples["au"][0])
		}
		if r.Examples["bi"][0][0] != "No, silly, those aren't your shoes!" || len(r.Examples["bi"]) != 3 || len(r.Examples["au"]) != 3 || len(r.Examples["or"]) != 5 {
			t.Fatalf("Examples = %#v", r.Examples)
		}
		if strings.Contains(r.Examples["au"][0][1], "<i>") || strings.Contains(r.Examples["bi"][0][0], "<b>") {
			t.Fatalf("example kept a tag: %#v", r.Examples)
		}
	})

	t.Run("look forward to", func(t *testing.T) {
		r := jsonapiResult("look forward to", true, true, false)
		outcome, err := fetchJSONAPIFile(t, "look_forward_to.json", r)
		assertFound(t, outcome, err)
		assertParamsKept(t, r, "look forward to", true, true, false)
		if r.Keyword != "look forward to" || r.Pronounce != nil {
			t.Fatalf("keyword/pronounce = %q %#v", r.Keyword, r.Pronounce)
		}
		wantPara := []string{
			"期待：指对未来的某事或某人充满期待和期望。",
			"期望：指对未来的某事或某人抱有希望和期望。",
		}
		if !reflect.DeepEqual(r.Paraphrase, wantPara) {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
		for _, para := range r.Paraphrase {
			if strings.Contains(para, "meeting you tomorrow") {
				t.Fatalf("paraphrase included trs sentence: %q", para)
			}
		}
		if r.Collins.Star != 0 || r.Collins.ViaRank != "" || r.Collins.AdditionalPattern != "looking,looked,looks" || len(r.Collins.Items) != 2 {
			t.Fatalf("Collins = star %d rank %q pattern %q items %d", r.Collins.Star, r.Collins.ViaRank, r.Collins.AdditionalPattern, len(r.Collins.Items))
		}
		wantItems := []*model.CollinsItem{
			colItem("PHRASAL VERB", "If you look forward to something that is going to happen, you want it to happen because you think you will enjoy it. 期待", []string{"He was looking forward to working with the new manager.", "他很期待和新经理一起工作。"}),
			colItem("PHRASAL VERB", "If you say that someone is looking forward to something useful or positive, you mean they expect it to happen. 盼望", []string{"He now says that he's looking forward to increased trade after the war.", "他现在说他正期盼着战后的贸易增长。"}),
		}
		if !reflect.DeepEqual(r.Collins.Items, wantItems) {
			t.Fatalf("Collins items = %#v", r.Collins.Items)
		}
		if r.Examples["bi"][0][0] != "I look forward to hearing from you." || len(r.Examples["bi"]) != 3 || len(r.Examples["au"]) != 3 || len(r.Examples["or"]) != 5 {
			t.Fatalf("Examples = %#v", r.Examples)
		}
	})

	t.Run("你好", func(t *testing.T) {
		r := jsonapiResult("你好", false, false, false)
		outcome, err := fetchJSONAPIFile(t, "nihao.json", r)
		assertFound(t, outcome, err)
		assertParamsKept(t, r, "你好", false, false, false)
		if r.Keyword != "你好" || r.Pronounce != nil {
			t.Fatalf("keyword/pronounce = %q %#v", r.Keyword, r.Pronounce)
		}
		if len(r.Paraphrase) != 3 || !strings.HasPrefix(r.Paraphrase[0], "hello ") || r.Paraphrase[2] != "how do you do 你好；" {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
		if !strings.Contains(r.Paraphrase[0], "<非正式>") || !strings.Contains(r.Paraphrase[0], "<英，旧>") {
			t.Fatalf("angle-bracket gloss was stripped: %q", r.Paraphrase[0])
		}
		if r.Collins.Star != 0 || r.Collins.ViaRank != "" || r.Collins.AdditionalPattern != "" || r.Collins.Items != nil {
			t.Fatalf("Collins = %+v", r.Collins)
		}
		wantBI := [][]string{
			{"Hello Simon! What brings you here?", "你好，西蒙！什么风把你吹来了？", "《牛津词典》"},
			{"'Hello, Paul,' they chorused.", "“你好，保罗。”他们齐声问候道。", "《牛津词典》"},
			{"I hope to God they are paying you well.", "我真心希望他们给你好的报酬。", "《柯林斯英汉双解大词典》"},
		}
		if !reflect.DeepEqual(r.Examples["bi"], wantBI) {
			t.Fatalf("bi = %#v", r.Examples["bi"])
		}
		if _, ok := r.Examples["au"]; ok || len(r.Examples["or"]) != 3 {
			t.Fatalf("Examples = %#v", r.Examples)
		}
		if r.Examples["or"][0][1] != "" || r.Examples["or"][0][2] != "1945年后的美国小说课程节选: 耶鲁公开课" {
			t.Fatalf("or[0] = %#v", r.Examples["or"][0])
		}
	})

	t.Run("打", func(t *testing.T) {
		r := jsonapiResult("打", false, false, false)
		outcome, err := fetchJSONAPIFile(t, "da.json", r)
		assertFound(t, outcome, err)
		assertParamsKept(t, r, "打", false, false, false)
		if r.Keyword != "打" || r.Pronounce != nil || len(r.Paraphrase) != 4 {
			t.Fatalf("keyword %q pronounce %#v paras %d", r.Keyword, r.Pronounce, len(r.Paraphrase))
		}
		if !strings.HasPrefix(r.Paraphrase[0], "hit ") || !strings.Contains(r.Paraphrase[0], "<非正式>") || !strings.Contains(r.Paraphrase[0], "<美，非正式>") {
			t.Fatalf("Paraphrase[0] = %q", r.Paraphrase[0])
		}
		if !strings.HasPrefix(r.Paraphrase[1], "fight ") || !strings.HasPrefix(r.Paraphrase[2], "strike ") || r.Paraphrase[3] != "dozen 一打，十二个；大量，许多；十多个；" {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
		if r.Examples["bi"][0][0] != "He liked to play a few holes after work." || r.Examples["bi"][0][1] != "他下班后喜欢打几杆高尔夫球。" {
			t.Fatalf("chinese example was not swapped: %#v", r.Examples["bi"][0])
		}
		if _, ok := r.Examples["au"]; ok || len(r.Examples["bi"]) != 3 || len(r.Examples["or"]) != 3 {
			t.Fatalf("Examples = %#v", r.Examples)
		}
	})
}

func TestFetchJSONAPILongTextAndNotFound(t *testing.T) {
	t.Run("long sentence", func(t *testing.T) {
		r := jsonapiResult("I don't know what you're talking about", true, false, true)
		outcome, err := fetchJSONAPIFile(t, "long_sentence.json", r)
		assertFound(t, outcome, err)
		assertParamsKept(t, r, "I don't know what you're talking about", true, false, true)
		if r.MachineTrans != "我不知道你在说什么" {
			t.Fatalf("MachineTrans = %q", r.MachineTrans)
		}
		assertWordFieldsEmpty(t, r)
	})

	t.Run("long text ignores language flag", func(t *testing.T) {
		r := jsonapiResult("I don't know what you're talking about", false, false, true)
		outcome, err := fetchJSONAPIFile(t, "long_sentence.json", r)
		assertFound(t, outcome, err)
		if r.MachineTrans != "我不知道你在说什么" || r.IsEN {
			t.Fatalf("MachineTrans = %q, IsEN = %v", r.MachineTrans, r.IsEN)
		}
	})

	t.Run("hello world long text has no translation", func(t *testing.T) {
		r := jsonapiResult("hello world", true, true, true)
		outcome, err := fetchJSONAPIFile(t, "hello_world.json", r)
		assertUnknown(t, outcome, err)
		if !strings.Contains(err.Error(), "translation") {
			t.Fatalf("error = %v, want a missing-translation error", err)
		}
		assertContentEmpty(t, r)
		assertParamsKept(t, r, "hello world", true, true, true)
	})

	t.Run("qwzxvkj", func(t *testing.T) {
		r := jsonapiResult("qwzxvkj", true, false, false)
		outcome, err := fetchJSONAPIFile(t, "qwzxvkj.json", r)
		assertNotFound(t, outcome, err)
		assertContentEmpty(t, r)
		assertParamsKept(t, r, "qwzxvkj", true, false, false)
	})
}

func TestFetchJSONAPIUsesCallerLanguage(t *testing.T) {
	t.Run("english flag does not read ce", func(t *testing.T) {
		r := jsonapiResult("你好", true, false, false)
		outcome, err := fetchJSONAPIFile(t, "nihao.json", r)
		assertNotFound(t, outcome, err)
		assertContentEmpty(t, r)
		if r.IsEN != true {
			t.Fatal("IsEN was changed")
		}
	})

	t.Run("chinese flag does not read ec", func(t *testing.T) {
		r := jsonapiResult("hello world", false, true, false)
		outcome, err := fetchJSONAPIFile(t, "hello_world.json", r)
		assertNotFound(t, outcome, err)
		assertContentEmpty(t, r)
	})
}

func TestFetchJSONAPIFieldRules(t *testing.T) {
	t.Run("cleanup and one-sided paraphrase", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ce": {"word": {"return-phrase": " 边 ", "phone": "biān", "trs": [
				{"#text": "<b>only</b>", "#tran": ""},
				{"#text": "", "#tran": "只有<br />中文 <B>X</B> <非正式>"},
				{"#text": "<b></b>", "#tran": "   "},
				{"#tran": "缺了英文"}
			]}}
		}`)
		r := jsonapiResult("边", false, false, false)
		r.Found = false
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if r.Found {
			t.Fatal("Found was set")
		}
		if r.Keyword != "边" || r.Pronounce != nil {
			t.Fatalf("keyword/pronounce = %q %#v", r.Keyword, r.Pronounce)
		}
		want := []string{"only", "只有中文 X <非正式>", "缺了英文"}
		if !reflect.DeepEqual(r.Paraphrase, want) {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
	})

	t.Run("english tags and blank phone", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"word": {
				"return-phrase": "edge",
				"ukphone": "  edʒ  ",
				"usphone": "",
				"trs": [{"pos": "v.", "tran": "hello<br>world <b>X</b> <br /> <非正式>"}]
			}}
		}`)
		r := jsonapiResult("edge", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if !reflect.DeepEqual(r.Pronounce, map[string]string{"英": "edʒ"}) {
			t.Fatalf("Pronounce = %#v", r.Pronounce)
		}
		if !reflect.DeepEqual(r.Paraphrase, []string{"v. helloworld X <非正式>"}) {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
	})

	t.Run("cet in a later group", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"exam_type": ["初中", "CET4"], "word": {"trs": [{"tran": "词"}]}},
			"collins": {"collins_entries": [
				{"star": "", "basic_entries": {"basic_entry": [{"headword": "a"}]}},
				{"star": "4", "basic_entries": {"basic_entry": [{"cet": ""}, {"cet": "CET6"}, {"cet": "TEM8"}]}}
			]}
		}`)
		r := jsonapiResult("word", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if r.Collins.Star != 4 || r.Collins.ViaRank != "CET6" {
			t.Fatalf("star %d rank %q", r.Collins.Star, r.Collins.ViaRank)
		}
	})

	t.Run("star parse failure stops the search", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"word": {"trs": [{"tran": "词"}]}},
			"collins": {"collins_entries": [
				{"star": "five"},
				{"star": "3", "basic_entries": {"basic_entry": [{"cet": "CET6"}]}}
			]}
		}`)
		r := jsonapiResult("word", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if r.Collins.Star != 0 || r.Collins.ViaRank != "CET6" {
			t.Fatalf("star %d rank %q", r.Collins.Star, r.Collins.ViaRank)
		}
	})

	t.Run("numeric star is ignored", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"word": {"trs": [{"tran": "词"}]}},
			"collins": {"collins_entries": [
				{"star": 5},
				{"star": "3"}
			]}
		}`)
		r := jsonapiResult("word", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if r.Collins.Star != 3 {
			t.Fatalf("star = %d", r.Collins.Star)
		}
	})

	t.Run("exam type fallback", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"exam_type": ["初中", 4, null, "", "考研"], "word": {"trs": [{"tran": "词"}]}}
		}`)
		r := jsonapiResult("word", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if r.Collins.ViaRank != "初中 考研" {
			t.Fatalf("ViaRank = %q", r.Collins.ViaRank)
		}
	})

	t.Run("chinese ignores exam type and phone", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"exam_type": ["CET4"], "word": {"ukphone": "x", "trs": [{"tran": "should-not-use"}]}},
			"ce": {"word": {"return-phrase": "你好", "phone": "nǐ hǎo", "trs": [{"#text": "hello", "#tran": "<非正式>嗨"}]}}
		}`)
		r := jsonapiResult("你好", false, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if r.Keyword != "你好" || r.Pronounce != nil || r.Collins.ViaRank != "" {
			t.Fatalf("keyword %q pron %#v rank %q", r.Keyword, r.Pronounce, r.Collins.ViaRank)
		}
		if !reflect.DeepEqual(r.Paraphrase, []string{"hello <非正式>嗨"}) {
			t.Fatalf("Paraphrase = %#v", r.Paraphrase)
		}
	})

	t.Run("type mismatch is not unknown", func(t *testing.T) {
		r := jsonapiResult("take", true, false, false)
		outcome, err := fetchJSONAPIBody(t, []byte(`{"meta":{},"ec":{"word":{"return-phrase":"take","trs":"not-an-array"}}}`), r)
		assertNotFound(t, outcome, err)
		assertContentEmpty(t, r)

		body := []byte(`{
			"meta": {},
			"ec": {"word": {"trs": ["bad", {"tran": ["also-bad"]}, {"pos": "n.", "tran": "ok"}]}},
			"collins": "not-an-object",
			"blng_sents_part": {"sentence-pair": {"sentence": "nope"}}
		}`)
		r = jsonapiResult("take", true, false, false)
		outcome, err = fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if !reflect.DeepEqual(r.Paraphrase, []string{"n. ok"}) || r.Collins.Items != nil || r.Examples != nil {
			t.Fatalf("result = para %#v collins %#v examples %#v", r.Paraphrase, r.Collins, r.Examples)
		}
	})

	t.Run("wordforms object is skipped", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"word": {"trs": [{"tran": "词"}]}},
			"collins": {"collins_entries": [{"basic_entries": {"basic_entry": [
				{"wordforms": {"wordform": {"word": "edges"}}},
				{"wordforms": {"wordform": [{"word": "took"}, {"word": ""}, {"word": "taken"}]}}
			]}}]}
		}`)
		r := jsonapiResult("take", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if r.Collins.AdditionalPattern != "took,taken" {
			t.Fatalf("pattern = %q", r.Collins.AdditionalPattern)
		}
	})

	t.Run("collins examples and empty tran", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"word": {"trs": [{"tran": "词"}]}},
			"collins": {"collins_entries": [{"entries": {"entry": [{"tran_entry": [
				{"headword": "see also"},
				{"tran": "<b></b>", "pos_entry": {"pos": "V"}},
				{"tran": "keep <b>me</b>", "pos_entry": {"pos": "N"}, "exam_sents": {"sent": [
					{"eng_sent": "", "chn_sent": "空"},
					{"eng_sent": "English only"},
					{"eng_sent": "Both <b>en</b>", "chn_sent": "中文"}
				]}}
			]}]}}]}
		}`)
		r := jsonapiResult("take", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		want := []*model.CollinsItem{colItem("N", "keep me", []string{"English only"}, []string{"Both en", "中文"})}
		if !reflect.DeepEqual(r.Collins.Items, want) {
			t.Fatalf("items = %#v", r.Collins.Items)
		}
	})

	t.Run("example row shapes", func(t *testing.T) {
		body := []byte(`{
			"meta": {},
			"ec": {"word": {"trs": [{"tran": "词"}]}},
			"blng_sents_part": {"sentence-pair": [
				{"sentence": "plain", "sentence-eng": "<b>other</b>", "sentence-translation": "中文", "source": "<i>src</i>"},
				{"sentence": "", "sentence-translation": "丢", "source": "x"},
				{"sentence": "nosrc", "sentence-translation": ""}
			]},
			"auth_sents_part": {"sent": [
				{"foreign": "<b></b>", "source": "x"},
				{"foreign": "keep", "source": ""},
				{"foreign": "both", "source": "<i>S</i>"}
			]},
			"media_sents_part": {"sent": [
				{"eng": "", "snippets": {"snippet": [{"source": "VOA", "name": "n"}]}},
				{"eng": "only", "snippets": {"snippet": [{"source": "VOA"}]}},
				{"eng": "name-only", "snippets": {"snippet": [{"name": "clip"}]}},
				{"eng": "both", "snippets": {"snippet": [{"source": "VOA", "name": "clip"}]}},
				{"eng": "none"},
				{"eng": "object", "snippets": {"snippet": {"source": "VOA", "name": "clip"}}}
			]}
		}`)
		r := jsonapiResult("word", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		want := map[string][][]string{
			"bi": {{"plain", "中文", "src"}, {"nosrc", ""}},
			"au": {{"keep"}, {"both", "S"}},
			"or": {
				{"only", "", "VOA"},
				{"name-only", "", "clip"},
				{"both", "", "VOA: clip"},
				{"none", ""},
				{"object", ""},
			},
		}
		if !reflect.DeepEqual(r.Examples, want) {
			t.Fatalf("Examples = %#v", r.Examples)
		}
	})

	t.Run("long text strips tags and ignores the word entry", func(t *testing.T) {
		body := []byte(`{"meta":{},"fanyi":{"tran":"你<br>好 <b>世界</b>"},"ec":{"word":{"return-phrase":"nope","trs":[{"tran":"词"}]}}}`)
		r := jsonapiResult("你好世界", true, false, true)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if r.MachineTrans != "你好 世界" || r.Keyword != "" || r.Paraphrase != nil {
			t.Fatalf("machine %q keyword %q para %#v", r.MachineTrans, r.Keyword, r.Paraphrase)
		}
	})

	t.Run("blank long text translation is unknown", func(t *testing.T) {
		r := jsonapiResult("x", true, false, true)
		outcome, err := fetchJSONAPIBody(t, []byte(`{"meta":{},"fanyi":{"tran":" <b></b> "}}`), r)
		assertUnknown(t, outcome, err)
		if !strings.Contains(err.Error(), "translation") {
			t.Fatalf("error = %v, want a missing-translation error", err)
		}
		assertContentEmpty(t, r)
	})
}

func TestFetchJSONAPIDebugSavesBody(t *testing.T) {
	origDebug := config.Cfg.Debug
	origRoot := run.CACHE_ROOT_PATH
	t.Cleanup(func() {
		config.Cfg.Debug = origDebug
		run.CACHE_ROOT_PATH = origRoot
	})
	root := t.TempDir()
	run.CACHE_ROOT_PATH = root
	body := []byte(`{"meta":{},"ec":{"word":{"return-phrase":"take","trs":[{"tran":"拿"}]}}}`)

	t.Run("off", func(t *testing.T) {
		config.Cfg.Debug = false
		r := jsonapiResult("take", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		if _, err := os.Stat(filepath.Join(root, "html")); !os.IsNotExist(err) {
			t.Fatalf("debug off created html dir: %v", err)
		}
	})

	t.Run("on", func(t *testing.T) {
		config.Cfg.Debug = true
		r := jsonapiResult("take", true, false, false)
		outcome, err := fetchJSONAPIBody(t, body, r)
		assertFound(t, outcome, err)
		got, err := os.ReadFile(filepath.Join(root, "html", "take.json"))
		if err != nil {
			t.Fatalf("read saved json: %v", err)
		}
		if string(got) != string(body) {
			t.Fatalf("saved body = %q", got)
		}
	})
}

func jsonapiResult(query string, isEN, phrase, long bool) *model.Result {
	return &model.Result{BaseResult: &model.BaseResult{
		Query:      query,
		IsEN:       isEN,
		IsPhrase:   phrase,
		IsLongText: long,
		Prompt:     "keep-prompt",
		Output:     "keep-output",
		Found:      true,
	}}
}

func fetchJSONAPIBody(t *testing.T, body []byte, r *model.Result) (fetchOutcome, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return fetchJSONAPIWith(context.Background(), srv.Client(), srv.URL, r)
}

func fetchJSONAPIFile(t *testing.T, name string, r *model.Result) (fetchOutcome, error) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "jsonapi_s", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return fetchJSONAPIBody(t, body, r)
}

func colItem(pos, major string, examples ...[]string) *model.CollinsItem {
	item := &model.CollinsItem{Additional: pos, MajorTrans: major}
	if len(examples) > 0 {
		item.ExampleLists = examples
	}
	return item
}

func assertFound(t *testing.T, outcome fetchOutcome, err error) {
	t.Helper()
	if outcome != fetchFound || err != nil {
		t.Fatalf("outcome = %s, err = %v, want found", outcome, err)
	}
}

func assertNotFound(t *testing.T, outcome fetchOutcome, err error) {
	t.Helper()
	if outcome != fetchNotFound || err != nil {
		t.Fatalf("outcome = %s, err = %v, want not found", outcome, err)
	}
}

func assertUnknown(t *testing.T, outcome fetchOutcome, err error) {
	t.Helper()
	if outcome != fetchUnknown || err == nil || !strings.Contains(err.Error(), "jsonapi_s") {
		t.Fatalf("outcome = %s, err = %v, want unknown jsonapi_s error", outcome, err)
	}
}

func assertParamsKept(t *testing.T, r *model.Result, query string, isEN, phrase, long bool) {
	t.Helper()
	b := r.BaseResult
	if b.Query != query || b.IsEN != isEN || b.IsPhrase != phrase || b.IsLongText != long || b.Prompt != "keep-prompt" || b.Output != "keep-output" || !b.Found || b.History != nil {
		t.Fatalf("query parameters changed: %+v", b)
	}
}

func assertContentEmpty(t *testing.T, r *model.Result) {
	t.Helper()
	assertWordFieldsEmpty(t, r)
	if r.MachineTrans != "" {
		t.Fatalf("MachineTrans = %q", r.MachineTrans)
	}
}

func assertWordFieldsEmpty(t *testing.T, r *model.Result) {
	t.Helper()
	if r.Keyword != "" || r.Pronounce != nil || r.Paraphrase != nil || r.Examples != nil || r.Collins.Star != 0 || r.Collins.ViaRank != "" || r.Collins.AdditionalPattern != "" || r.Collins.Items != nil {
		t.Fatalf("word fields = keyword %q pron %#v para %#v examples %#v collins %+v", r.Keyword, r.Pronounce, r.Paraphrase, r.Examples, r.Collins)
	}
}

// be 的词形里有 "was, were" 这种合在一起的值，整理后要和旧网页一样没有空格。
func TestFetchJSONAPINormalizesCombinedWordForms(t *testing.T) {
	body := []byte(`{"meta":{},"ec":{"word":{"trs":[{"pos":"v.","tran":"是"}]}},"collins":{"collins_entries":[{"basic_entries":{"basic_entry":[{"wordforms":{"wordform":[{"word":"being"},{"word":"was, were"},{"word":"been"},{"word":"am, are, is"}]}}]}}]}}`)
	r := jsonapiResult("be", true, false, false)
	outcome, err := fetchJSONAPIBody(t, body, r)
	assertFound(t, outcome, err)
	if got, want := r.Collins.AdditionalPattern, "being,was,were,been,am,are,is"; got != want {
		t.Fatalf("AdditionalPattern = %q, want %q", got, want)
	}
}

// 有道偶尔把别的词的完整数据当成本次查询的结果返回，只有 input 能看出来。
func TestFetchJSONAPIRejectsResponseForAnotherQuery(t *testing.T) {
	entry := `"ec":{"word":{"return-phrase":"salient","trs":[{"pos":"adj.","tran":"显著的"}]}},"ce":{"word":{"return-phrase":"x","trs":[{"#text":"x","#tran":"y"}]}}`
	for _, tt := range []struct {
		name, query, body string
		isEN, long        bool
		want              fetchOutcome
	}{
		{"top-level input differs", "银行", `{"input":"salient","meta":{"input":"salient"},` + entry + `}`, false, false, fetchUnknown},
		{"meta input differs", "学校", `{"meta":{"input":"Nebuchadnezzar"},` + entry + `}`, false, false, fetchUnknown},
		{"English input differs", "take", `{"input":"salient","meta":{},` + entry + `}`, true, false, fetchUnknown},
		{"long text input differs", "good morning", `{"input":"helix","meta":{},"fanyi":{"tran":"螺旋"}}`, true, true, fetchUnknown},
		{"case and spaces are ignored", "nasa", `{"input":" NASA ","meta":{},` + entry + `}`, true, false, fetchFound},
		{"missing input is not checked", "take", `{"meta":{},` + entry + `}`, true, false, fetchFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := jsonapiResult(tt.query, tt.isEN, false, tt.long)
			outcome, err := fetchJSONAPIBody(t, []byte(tt.body), r)
			if outcome != tt.want {
				t.Fatalf("outcome = %v (err %v), want %v", outcome, err, tt.want)
			}
			if tt.want == fetchUnknown {
				if err == nil {
					t.Fatal("fetchUnknown without an error")
				}
				if strings.Contains(err.Error(), tt.query) {
					t.Fatalf("error %q contains the query", err)
				}
			}
		})
	}
}
