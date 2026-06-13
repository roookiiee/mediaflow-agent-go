package rag

import "context"

type Hit struct {
	Source string  `json:"source"`
	Text   string  `json:"text"`
	Score  float64 `json:"score"`
}

type Searcher interface {
	Search(ctx context.Context, query string, limit int) ([]Hit, error)
}
