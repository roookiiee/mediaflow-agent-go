package vector

import "testing"

func TestEmbedTextIsDeterministicAndNormalized(t *testing.T) {
	first := EmbedText("video localization tts metrics", 64)
	second := EmbedText("video localization tts metrics", 64)
	if len(first) != 64 {
		t.Fatalf("expected 64 dimensions, got %d", len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("embedding should be deterministic at index %d", i)
		}
	}

	var nonZero bool
	for _, value := range first {
		if value != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatalf("expected non-zero embedding")
	}
}
