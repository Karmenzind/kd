package str

import (
	"reflect"
	"testing"
)

func TestSimplify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "trim and collapse", input: "  hello\t  world  ", want: "hello world"},
		{name: "fold newlines into spaces", input: "hello\nworld", want: "hello world"},
		{name: "trim non-breaking space", input: "\u00a0中文 text\u00a0", want: "中文 text"},
		{name: "fold carriage returns", input: "one\r\ntwo", want: "one two"},
		{name: "collapse wide spaces", input: "a　 b", want: "a b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Simplify(tt.input); got != tt.want {
				t.Fatalf("Simplify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSplitIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "plain lowercase", input: "hello", want: []string{"hello"}},
		{name: "capitalized word", input: "Hello", want: []string{"Hello"}},
		{name: "all caps word", input: "HTTP", want: []string{"HTTP"}},
		{name: "camelCase", input: "helloWorld", want: []string{"hello", "World"}},
		{name: "PascalCase", input: "HelloWorld", want: []string{"Hello", "World"}},
		{name: "snake_case", input: "hello_world", want: []string{"hello", "world"}},
		{name: "acronym prefix", input: "HTTPServer", want: []string{"HTTP", "Server"}},
		{name: "acronym in middle", input: "getHTTPResponse", want: []string{"get", "HTTP", "Response"}},
		{name: "single letters kept", input: "getUserByID", want: []string{"get", "User", "By", "ID"}},
		{name: "leading underscore", input: "_private", want: []string{"private"}},
		{name: "trailing underscores", input: "trailing__", want: []string{"trailing"}},
		{name: "digit stays with previous", input: "user2FA", want: []string{"user2", "FA"}},
		{name: "digit prefix", input: "v2Api", want: []string{"v2", "Api"}},
		{name: "mixed snake and camel", input: "user_nameTag", want: []string{"user", "name", "Tag"}},
		{name: "phrase untouched", input: "leave me alone", want: []string{"leave me alone"}},
		{name: "only underscores", input: "___", want: []string{}},
		{name: "empty", input: "", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitIdentifier(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SplitIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIdentifierPhrase(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantPhrase string
		wantOK     bool
	}{
		{name: "camelCase", input: "helloWorld", wantPhrase: "hello world", wantOK: true},
		{name: "PascalCase", input: "HelloWorld", wantPhrase: "hello world", wantOK: true},
		{name: "snake_case", input: "hello_world", wantPhrase: "hello world", wantOK: true},
		{name: "leading underscore", input: "_private", wantPhrase: "private", wantOK: true},
		{name: "acronym", input: "getHTTPResponse", wantPhrase: "get http response", wantOK: true},
		{name: "plain word unchanged", input: "hello", wantOK: false},
		{name: "capitalized word unchanged", input: "Hello", wantOK: false},
		{name: "all caps unchanged", input: "HTTP", wantOK: false},
		{name: "phrase unchanged", input: "leave me alone", wantOK: false},
		{name: "empty", input: "", wantOK: false},
		{name: "only underscores", input: "___", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPhrase, gotOK := IdentifierPhrase(tt.input)
			if gotOK != tt.wantOK || gotPhrase != tt.wantPhrase {
				t.Fatalf("IdentifierPhrase(%q) = (%q, %v), want (%q, %v)", tt.input, gotPhrase, gotOK, tt.wantPhrase, tt.wantOK)
			}
		})
	}
}

func TestRewriteIdentifiers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "camel in sentence", input: "Call getUserByID now", want: "Call get user by id now"},
		{name: "snake in sentence", input: "set user_name please", want: "set user name please"},
		{name: "plain sentence untouched", input: "leave me alone", want: "leave me alone"},
		{name: "punctuation kept", input: "use (helloWorld).", want: "use (hello world)."},
		{name: "Chinese mixed", input: "调用 getUserByID 方法", want: "调用 get user by id 方法"},
		{name: "all caps untouched", input: "HTTP server", want: "HTTP server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RewriteIdentifiers(tt.input); got != tt.want {
				t.Fatalf("RewriteIdentifiers(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSimplifyLines(t *testing.T) {
	input := "n.\n\n 小羊；羔羊\n                    小羊肉\t\n\n"
	got := SimplifyLines(input)
	want := []string{"n.", "小羊；羔羊", "小羊肉"}
	if len(got) != len(want) {
		t.Fatalf("SimplifyLines(%q) = %q, want %q", input, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SimplifyLines(%q)[%d] = %q, want %q", input, i, got[i], want[i])
		}
	}
	if lines := SimplifyLines("  \n\t\n"); len(lines) != 0 {
		t.Fatalf("SimplifyLines(blank) = %q, want empty", lines)
	}
}
