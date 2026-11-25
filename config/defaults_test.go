package config

import "testing"

func TestApplyDefaults(t *testing.T) {
	type nested struct {
		Enabled bool   `default:"true"`
		Count   uint64 `default:"18446744073709551615"`
	}
	type settings struct {
		Enabled  bool   `default:"true"`
		Name     string `default:"a literal string"`
		Empty    string `default:""`
		Count    uint64 `default:"2048"`
		Nested   nested
		Untagged int
		private  int `default:"invalid"`
	}
	got := settings{}
	if err := applyDefaults(&got); err != nil {
		t.Fatal(err)
	}
	want := settings{
		Enabled: true, Name: "a literal string", Count: 2048,
		Nested: nested{Enabled: true, Count: ^uint64(0)},
	}
	if got != want {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}

	got.Name, got.Count, got.Nested.Count = "keep", 7, 42
	want = got
	if err := applyDefaults(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("nonzero fields changed: %+v, want %+v", got, want)
	}
}

func TestApplyDefaultsErrors(t *testing.T) {
	var nilStruct *Config
	value := Config{}
	tests := []struct {
		name  string
		value any
	}{
		{"nil", nil},
		{"typed nil", nilStruct},
		{"non-pointer", Config{}},
		{"pointer to non-struct", new(string)},
		{"pointer to pointer", &nilStruct},
		{"unsupported int", &struct {
			Value int `default:"1"`
		}{}},
		{"unsupported nonzero int", &struct {
			Value int `default:"1"`
		}{Value: 2}},
		{"unsupported slice", &struct {
			Value []string `default:"text"`
		}{}},
		{"unsupported struct", &struct {
			Value Config `default:"text"`
		}{}},
		{"invalid bool", &struct {
			Value bool `default:"yes"`
		}{}},
		{"invalid uint64", &struct {
			Value uint64 `default:"word"`
		}{}},
		{"negative uint64", &struct {
			Value uint64 `default:"-1"`
		}{}},
		{"non-decimal uint64", &struct {
			Value uint64 `default:"0xff"`
		}{}},
		{"overflowing uint64", &struct {
			Value uint64 `default:"18446744073709551616"`
		}{}},
		{"nested invalid default", &struct {
			Nested struct {
				Value bool `default:"invalid"`
			}
		}{}},
		{"unsupported pointer field", &struct {
			Value *Config `default:"text"`
		}{Value: &value}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := applyDefaults(tt.value); err == nil {
				t.Fatal("applyDefaults() succeeded, want error")
			}
		})
	}
}
