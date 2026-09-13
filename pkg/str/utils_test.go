package str

import "testing"

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
