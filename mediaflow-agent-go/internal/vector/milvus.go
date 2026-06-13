package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/portfolio/mediaflow-agent-go/internal/rag"
)

type MilvusConfig struct {
	Enabled    bool
	Address    string
	Token      string
	Collection string
	Dim        int
	Timeout    time.Duration
}

type MilvusClient struct {
	address    string
	token      string
	collection string
	dim        int
	client     *http.Client
}

func NewMilvusClient(cfg MilvusConfig) *MilvusClient {
	address := strings.TrimRight(cfg.Address, "/")
	if address == "" {
		address = "http://localhost:19530"
	}
	collection := strings.TrimSpace(cfg.Collection)
	if collection == "" {
		collection = "mediaflow_knowledge"
	}
	dim := cfg.Dim
	if dim <= 0 {
		dim = 64
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return &MilvusClient{
		address:    address,
		token:      cfg.Token,
		collection: collection,
		dim:        dim,
		client:     &http.Client{Timeout: timeout},
	}
}

func (c *MilvusClient) Collection() string {
	return c.collection
}

func (c *MilvusClient) EnsureCollection(ctx context.Context) error {
	body := map[string]any{
		"collectionName": c.collection,
		"schema": map[string]any{
			"autoId":              false,
			"enabledDynamicField": true,
			"fields": []map[string]any{
				{
					"fieldName": "id",
					"dataType":  "Int64",
					"isPrimary": true,
				},
				{
					"fieldName": "vector",
					"dataType":  "FloatVector",
					"elementTypeParams": map[string]any{
						"dim": fmt.Sprint(c.dim),
					},
				},
			},
		},
		"indexParams": []map[string]any{
			{
				"fieldName":  "vector",
				"indexName":  "vector",
				"indexType":  "AUTOINDEX",
				"metricType": "COSINE",
			},
		},
	}

	err := c.post(ctx, "/v2/vectordb/collections/create", body, nil)
	if err != nil && !isAlreadyExists(err) {
		return err
	}

	_ = c.post(ctx, "/v2/vectordb/collections/load", map[string]any{
		"collectionName": c.collection,
	}, nil)
	return nil
}

func (c *MilvusClient) IndexMarkdownDir(ctx context.Context, dir string) (int, error) {
	docs, err := LoadMarkdownDocuments(dir)
	if err != nil {
		return 0, err
	}
	if len(docs) == 0 {
		return 0, nil
	}
	return len(docs), c.Upsert(ctx, docs)
}

func (c *MilvusClient) Upsert(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}

	rows := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		rows = append(rows, map[string]any{
			"id":     doc.ID,
			"vector": EmbedText(doc.Source+"\n"+doc.Text, c.dim),
			"source": doc.Source,
			"text":   doc.Text,
		})
	}

	body := map[string]any{
		"collectionName": c.collection,
		"data":           rows,
	}

	if err := c.post(ctx, "/v2/vectordb/entities/upsert", body, nil); err != nil {
		insertErr := c.post(ctx, "/v2/vectordb/entities/insert", body, nil)
		if insertErr != nil && !isDuplicate(insertErr) {
			return fmt.Errorf("upsert failed: %v; insert fallback failed: %w", err, insertErr)
		}
	}
	return nil
}

func (c *MilvusClient) Search(ctx context.Context, query string, limit int) ([]rag.Hit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 4
	}

	body := map[string]any{
		"collectionName": c.collection,
		"annsField":      "vector",
		"data":           [][]float32{EmbedText(query, c.dim)},
		"limit":          limit,
		"outputFields":   []string{"source", "text"},
	}

	var parsed searchResponse
	if err := c.post(ctx, "/v2/vectordb/entities/search", body, &parsed); err != nil {
		return nil, err
	}

	hits := make([]rag.Hit, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		source := stringFromAny(item.Entity["source"])
		text := stringFromAny(item.Entity["text"])
		if source == "" {
			source = stringFromAny(item.Source)
		}
		if text == "" {
			text = stringFromAny(item.Text)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		hits = append(hits, rag.Hit{
			Source: source,
			Text:   text,
			Score:  item.Score(),
		})
	}
	return hits, nil
}

func (c *MilvusClient) post(ctx context.Context, path string, request any, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.address+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("milvus %s failed with status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var status milvusStatus
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &status); err == nil {
			if status.Code != 0 && status.Code != 200 {
				message := firstNonEmpty(status.Message, status.Msg, string(raw))
				return fmt.Errorf("milvus %s failed: %s", path, message)
			}
		}
	}
	if response != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, response); err != nil {
			return err
		}
	}
	return nil
}

type milvusStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Msg     string `json:"msg"`
}

type searchResponse struct {
	Code int          `json:"code"`
	Data []searchHit `json:"data"`
}

type searchHit struct {
	ID       any            `json:"id"`
	Distance float64        `json:"distance"`
	ScoreRaw float64        `json:"score"`
	Source   any            `json:"source"`
	Text     any            `json:"text"`
	Entity   map[string]any `json:"entity"`
}

func (h searchHit) Score() float64 {
	if h.ScoreRaw != 0 {
		return h.ScoreRaw
	}
	return h.Distance
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func isAlreadyExists(err error) bool {
	return containsAny(strings.ToLower(err.Error()), "already", "exist")
}

func isDuplicate(err error) bool {
	return containsAny(strings.ToLower(err.Error()), "duplicate", "primary key", "already")
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
