# Youdao test fixtures

Captured on 2026-11-23 with curl 8.21.0 (Git for Windows) and the User-Agent
`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36`.

## jsonapi_s/

Request, with `<q>` URL-encoded:

```
https://dict.youdao.com/jsonapi_s?doctype=json&jsonversion=4&le=en&q=<q>&dicts={"count":99,"dicts":[["ec","ce","collins","blng_sents_part","auth_sents_part","media_sents_part","fanyi","typos","simple"]]}
```

The responses were pretty-printed with jq and trimmed of audio and alignment
fields kd never reads (`aligned-words`, `speech-size`, `sentence-speech`,
`sentence-translation-speech`, `speech`, `ukspeech`, `usspeech`, `voice`,
`collegeExamVoice`, `win8`, `swf`, `streamUrl`). Everything else is verbatim.

| File | Query | Notes |
|---|---|---|
| `take.json` | `take` | polysemous word, two Collins groups, `cet` only in the first |
| `nasa.json` | `NASA` | abbreviation |
| `arent.json` | `aren't` | the legacy page reports it as not found |
| `look_forward_to.json` | `look forward to` | phrase: `trs` entries have no `pos` |
| `nihao.json` | `你好` | Chinese word, `ce`; `<非正式>` inside `#tran` |
| `da.json` | `打` | Chinese word |
| `long_sentence.json` | `I don't know what you're talking about` | long text with `fanyi` |
| `hello_world.json` | `hello world` | short phrase: `ec` only, no `fanyi` |
| `qwzxvkj.json` | `qwzxvkj` | not found: only `input`, `lang`, `le`, `meta` |

## legacy/

`http://dict.youdao.com/w/<q>/`, saved verbatim.

| File | Query | Notes |
|---|---|---|
| `qwzxvkj.html` | `qwzxvkj` | not-found page: has `id="results-contents"`, no `trans-container` |
| `hello_world.html` | `hello world` | long-text page |
