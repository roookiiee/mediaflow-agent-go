package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Job struct {
	ID           string         `json:"id"`
	TraceID      string         `json:"trace_id"`
	Brief        string         `json:"brief"`
	TargetLocale string         `json:"target_locale,omitempty"`
	Channel      string         `json:"channel,omitempty"`
	Status       string         `json:"status"`
	Provider     string         `json:"provider"`
	Summary      string         `json:"summary"`
	Metrics      map[string]any `json:"metrics,omitempty"`
	Artifacts    []Artifact     `json:"artifacts,omitempty"`
	Events       []EventRecord  `json:"events,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type Artifact struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Content any    `json:"content"`
}

type EventRecord struct {
	Type       string    `json:"type"`
	Tool       string    `json:"tool,omitempty"`
	Message    string    `json:"message,omitempty"`
	Payload    any       `json:"payload,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
	At         time.Time `json:"at"`
}

type FileStore struct {
	dir string
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) Save(_ context.Context, job Job) error {
	if job.ID == "" {
		return fmt.Errorf("job id is required")
	}
	path := filepath.Join(s.dir, sanitize(job.ID)+".json")
	body, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func (s *FileStore) Get(_ context.Context, id string) (Job, error) {
	path := filepath.Join(s.dir, sanitize(id)+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		return Job{}, err
	}
	var job Job
	if err := json.Unmarshal(body, &job); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *FileStore) List(_ context.Context) ([]Job, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var job Job
		if err := json.Unmarshal(body, &job); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	return jobs, nil
}

func sanitize(id string) string {
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "\\", "_")
	id = strings.ReplaceAll(id, "..", "_")
	return id
}
