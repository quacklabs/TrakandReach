package filter

import (
	"os"
	"testing"
)

func TestSpamFilter(t *testing.T) {
	tmpFile := "test_spam.txt"
	content := `
phone: 12345
word: buy now
# comment
word: viagra
`
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	sf := NewSpamFilter(tmpFile)

	tests := []struct {
		phone   string
		message string
		want    bool
	}{
		{"12345", "hello", true},
		{"99999", "hello", false},
		{"99999", "please buy now", true},
		{"99999", "VIAGRA is here", true},
		{"12345@c.us", "hey", true},
	}

	for _, tt := range tests {
		if got := sf.IsSpam(tt.phone, tt.message); got != tt.want {
			t.Errorf("IsSpam(%q, %q) = %v, want %v", tt.phone, tt.message, got, tt.want)
		}
	}

	// Test reload
	os.WriteFile(tmpFile, []byte("phone: 99999"), 0644)
	sf.Reload()

	if !sf.IsSpam("99999", "hi") {
		t.Errorf("Expected 99999 to be blocked after reload")
	}
}
