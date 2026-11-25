package str

import (
	"errors"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr error
	}{
		{
			name: "less -RF",
			in:   "less -RF",
			want: []string{"less", "-RF"},
		},
		{
			name: "bat --paging=always",
			in:   "bat --paging=always",
			want: []string{"bat", "--paging=always"},
		},
		{
			name: "single quotes",
			in:   "less -R 'foo bar'",
			want: []string{"less", "-R", "foo bar"},
		},
		{
			name: "double quote escape",
			in:   `less -R "foo \"bar\""`,
			want: []string{"less", "-R", `foo "bar"`},
		},
		{
			name: "backslash space",
			in:   `less\ -RF`,
			want: []string{"less -RF"},
		},
		{
			name: "empty single quotes",
			in:   `less '' -R`,
			want: []string{"less", "", "-R"},
		},
		{
			name: "hash at arg start",
			in:   "less\n# comment here\n-RF",
			want: []string{"less", "-RF"},
		},
		{
			name: "hash in middle",
			in:   "less -R#notcomment",
			want: []string{"less", "-R#notcomment"},
		},
		{
			name: "multiline comment",
			in:   "less\n# full line comment\n-RF",
			want: []string{"less", "-RF"},
		},
		{
			name:    "unclosed single",
			in:      "less 'unclosed",
			want:    []string{"less"},
			wantErr: ErrUnclosedQuote,
		},
		{
			name:    "unclosed double",
			in:      `less "unclosed`,
			want:    []string{"less"},
			wantErr: ErrUnclosedQuote,
		},
		{
			name:    "trailing backslash",
			in:      `less \`,
			want:    []string{"less"},
			wantErr: ErrTrailingBackslash,
		},
		{
			name:    "trailing backslash in dq",
			in:      `less "\`,
			want:    []string{"less"},
			wantErr: ErrTrailingBackslash,
		},
		{
			name: "windows path",
			in:   `C:\Program Files\x\less.exe -R`,
			want: []string{"C:Program", "Filesxless.exe", "-R"},
		},
		{
			name: "empty double quotes",
			in:   `less "" -R`,
			want: []string{"less", "", "-R"},
		},
		{
			name: "mixed quotes",
			in:   `less "a"'b' -R`,
			want: []string{"less", "ab", "-R"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := SplitCommand(tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SplitCommand() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("SplitCommand() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("SplitCommand() = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("SplitCommand()[%d] = %q, want %q (full %#v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}
