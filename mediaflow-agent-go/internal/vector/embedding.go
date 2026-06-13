package vector

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

func EmbedText(text string, dim int) []float32 {
	if dim <= 0 {
		dim = 64
	}
	vector := make([]float32, dim)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return vector
	}

	for _, token := range tokens {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(token))
		sum := hash.Sum64()
		index := int(sum % uint64(dim))
		sign := float32(1)
		if (sum>>8)&1 == 1 {
			sign = -1
		}
		vector[index] += sign
	}

	var norm float64
	for _, value := range vector {
		norm += float64(value * value)
	}
	if norm == 0 {
		return vector
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vector {
		vector[i] *= scale
	}
	return vector
}

func tokenize(text string) []string {
	tokens := make([]string, 0, 32)
	var builder strings.Builder
	flush := func() {
		if builder.Len() >= 2 {
			tokens = append(tokens, strings.ToLower(builder.String()))
		}
		builder.Reset()
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return tokens
}
