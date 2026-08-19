package app

import "testing"

func TestParseTOML(t *testing.T) {
	c := parseTOML("version = \"8.4\"\nvariant = \"ts\"\narch = \"x64\"\n[ini]\nmemory_limit = \"1G\"\n")
	if c.Version != "8.4" || c.Variant != "ts" || c.INI["memory_limit"] != "1G" {
		t.Fatalf("unexpected config: %#v", c)
	}
}
func TestConstraintVersion(t *testing.T) {
	for input, want := range map[string]string{"^8.3": "8.3", ">= 8.2": "8.2", "~8.4.1": "8.4"} {
		if got := constraintVersion(input); got != want {
			t.Errorf("%q: got %q want %q", input, got, want)
		}
	}
}
