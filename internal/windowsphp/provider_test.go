package windowsphp

import "testing"

func TestCompare(t *testing.T) {
	if compare("8.4.10", "8.4.9") <= 0 {
		t.Fatal("expected 8.4.10 to be newer")
	}
	if compare("8.3.1", "8.4.0") >= 0 {
		t.Fatal("expected 8.3.1 to be older")
	}
}
