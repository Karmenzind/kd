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
		{name: "remove newlines", input: "hello\nworld", want: "helloworld"},
		{name: "trim non-breaking space", input: "\u00a0中文 text\u00a0", want: "中文 text"},
		{name: "preserve carriage return inside text", input: "one\r\ntwo", want: "one\rtwo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Simplify(tt.input); got != tt.want {
				t.Fatalf("Simplify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
