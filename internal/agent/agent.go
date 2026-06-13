package agent

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/portfolio/mediaflow-agent-go/internal/guardrails"
	"github.com/portfolio/mediaflow-agent-go/internal/llm"
	"github.com/portfolio/mediaflow-agent-go/internal/rag"
	"github.com/portfolio/mediaflow-agent-go/internal/skills"
	"github.com/portfolio/mediaflow-agent-go/internal/store"
	"github.com/portfolio/mediaflow-agent-go/internal/tools"
)

type Agent struct {
	llm          llm.Client
	tools        *tools.Registry
	store        *store.FileStore
	skills       *skills.Manager
	vector       rag.Searcher
	knowledgeDir string
	logger       *slog.Logger
}

type Options struct {
	LLM          llm.Client
	Tools        *tools.Registry
	Store        *store.FileStore
	Skills       *skills.Manager
	Vector       rag.Searcher
	KnowledgeDir string
	Logger       *slog.Logger
}

type Request struct {
	Brief        string `json:"brief"`
	SourceScript string `json:"source_script,omitempty"`
	TargetLocale string `json:"target_locale,omitempty"`
	Channel      string `json:"channel,omitempty"`
	Mode         string `json:"mode,omitempty"`
}

type Event struct {
	Type       string    `json:"type"`
	TraceID    string    `json:"trace_id,omitempty"`
	Tool       string    `json:"tool,omitempty"`
	Message    string    `json:"message,omitempty"`
	Payload    any       `json:"payload,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
	At         time.Time `json:"at"`
}

type RunResult struct {
	JobID        string           `json:"job_id"`
	TraceID      string           `json:"trace_id"`
	Provider     string           `json:"provider"`
	Summary      string           `json:"summary"`
	TargetLocale string           `json:"target_locale"`
	Channel      string           `json:"channel"`
	Metrics      map[string]any   `json:"metrics"`
	Artifacts    []store.Artifact `json:"artifacts"`
	ToolCalls    []ToolCall       `json:"tool_calls"`
}

type ToolCall struct {
	Name       string    `json:"name"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
}

func New(options Options) *Agent {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Agent{
		llm:          options.LLM,
		tools:        options.Tools,
		store:        options.Store,
		skills:       options.Skills,
		vector:       options.Vector,
		knowledgeDir: options.KnowledgeDir,
		logger:       logger,
	}
}

func (a *Agent) ProviderName() string {
	if a.llm == nil {
		return "none"
	}
	return a.llm.Name()
}

func (a *Agent) Run(ctx context.Context, req Request, emit func(Event)) (*RunResult, error) {
	if strings.TrimSpace(req.Brief) == "" {
		return nil, errors.New("brief is required")
	}
	if a.tools == nil {
		return nil, errors.New("tool registry is not configured")
	}

	traceID := newID("trace")
	jobID := newID("job")
	events := make([]Event, 0, 16)
	toolCalls := make([]ToolCall, 0, 8)

	emitEvent := func(event Event) {
		if event.At.IsZero() {
			event.At = time.Now().UTC()
		}
		event.TraceID = traceID
		events = append(events, event)
		if emit != nil {
			emit(event)
		}
	}

	emitEvent(Event{Type: "status", Message: "Agent received the media localization brief."})

	assessment := guardrails.AssessMediaRequest(req.Brief + "\n" + req.SourceScript)
	emitEvent(Event{Type: "guardrail", Message: "Media safety policy checked.", Payload: assessment})
	if !assessment.Allowed {
		return nil, errors.New("request blocked by media guardrails")
	}

	toolCtx := tools.Context{
		LLM:          a.llm,
		KnowledgeDir: a.knowledgeDir,
		Skills:       a.skills,
		Vector:       a.vector,
	}

	invoke := func(name string, args map[string]any) (tools.Result, error) {
		start := time.Now()
		emitEvent(Event{Type: "tool_start", Tool: name, Message: "Running " + name, Payload: args})
		result, err := a.tools.Invoke(ctx, toolCtx, name, args)
		duration := time.Since(start)
		call := ToolCall{Name: name, StartedAt: start.UTC(), DurationMS: duration.Milliseconds()}
		if err != nil {
			call.Error = err.Error()
			toolCalls = append(toolCalls, call)
			emitEvent(Event{Type: "tool_error", Tool: name, Message: err.Error(), DurationMS: duration.Milliseconds()})
			return tools.Result{}, err
		}
		toolCalls = append(toolCalls, call)
		emitEvent(Event{Type: "tool_result", Tool: name, Message: result.Text, Payload: result.Data, DurationMS: duration.Milliseconds()})
		return result, nil
	}

	analysis, err := invoke("analyze_brief", map[string]any{
		"brief":         req.Brief,
		"source_script": req.SourceScript,
		"target_locale": req.TargetLocale,
		"channel":       req.Channel,
	})
	if err != nil {
		return nil, err
	}
	analysisData := asMap(analysis.Data)
	sourceScript := firstNonEmpty(req.SourceScript, mapString(analysisData, "source_script"))
	targetLocale := firstNonEmpty(req.TargetLocale, mapString(analysisData, "target_locale"), "English")
	channel := firstNonEmpty(req.Channel, mapString(analysisData, "channel"), "TikTok")
	voiceStyle := firstNonEmpty(mapString(analysisData, "voice_style"), "warm product narrator")

	skillGuidance := a.loadMatchedSkills(ctx, invoke, req.Brief+" "+channel+" "+targetLocale, emitEvent)

	knowledge, err := invoke("retrieve_knowledge", map[string]any{
		"query": req.Brief + " " + channel + " " + targetLocale,
	})
	if err != nil {
		return nil, err
	}

	guidance := strings.TrimSpace(skillGuidance + "\n\n" + knowledge.Text)
	localized, err := invoke("translate_script", map[string]any{
		"script":        sourceScript,
		"target_locale": targetLocale,
		"channel":       channel,
		"guidance":      guidance,
	})
	if err != nil {
		return nil, err
	}

	ttsPlan, err := invoke("build_tts_plan", map[string]any{
		"localized_script": localized.Text,
		"voice_style":      voiceStyle,
	})
	if err != nil {
		return nil, err
	}

	contentPack, err := invoke("generate_content_pack", map[string]any{
		"localized_script": localized.Text,
		"target_locale":    targetLocale,
		"channel":          channel,
	})
	if err != nil {
		return nil, err
	}

	score, err := invoke("score_quality", map[string]any{
		"source_script":    sourceScript,
		"localized_script": localized.Text,
		"tts_plan":         ttsPlan.Text,
		"content_pack":     contentPack.Text,
	})
	if err != nil {
		return nil, err
	}

	metrics := asMap(score.Data)
	summary := buildSummary(targetLocale, channel, score.Text)
	artifacts := []store.Artifact{
		{Name: "localized_script.md", Kind: "markdown", Content: localized.Text},
		{Name: "tts_plan.json", Kind: "json", Content: ttsPlan.Data},
		{Name: "content_pack.md", Kind: "markdown", Content: contentPack.Text},
		{Name: "quality_metrics.json", Kind: "json", Content: metrics},
	}

	now := time.Now().UTC()
	job := store.Job{
		ID:           jobID,
		TraceID:      traceID,
		Brief:        req.Brief,
		TargetLocale: targetLocale,
		Channel:      channel,
		Status:       "completed",
		Provider:     a.ProviderName(),
		Summary:      summary,
		Metrics:      metrics,
		Artifacts:    artifacts,
		Events:       toStoreEvents(events),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if a.store != nil {
		if err := a.store.Save(ctx, job); err != nil {
			return nil, err
		}
	}

	emitEvent(Event{Type: "status", Message: "Job persisted and ready for review.", Payload: map[string]string{"job_id": jobID}})

	return &RunResult{
		JobID:        jobID,
		TraceID:      traceID,
		Provider:     a.ProviderName(),
		Summary:      summary,
		TargetLocale: targetLocale,
		Channel:      channel,
		Metrics:      metrics,
		Artifacts:    artifacts,
		ToolCalls:    toolCalls,
	}, nil
}

func (a *Agent) loadMatchedSkills(ctx context.Context, invoke func(string, map[string]any) (tools.Result, error), query string, emit func(Event)) string {
	if a.skills == nil {
		return ""
	}
	matches := a.skills.Match(query, 2)
	if len(matches) == 0 {
		emit(Event{Type: "skill", Message: "No matching skill found; continuing with default workflow."})
		return ""
	}
	var builder strings.Builder
	for _, skill := range matches {
		if ctx.Err() != nil {
			return builder.String()
		}
		result, err := invoke("load_skill", map[string]any{"name": skill.Name})
		if err != nil {
			continue
		}
		builder.WriteString("\n\n# Skill: " + skill.Name + "\n")
		builder.WriteString(result.Text)
	}
	return strings.TrimSpace(builder.String())
}

func buildSummary(targetLocale, channel, scoreText string) string {
	return fmt.Sprintf("Completed %s localization workflow for %s. %s", targetLocale, channel, scoreText)
}

func toStoreEvents(events []Event) []store.EventRecord {
	records := make([]store.EventRecord, 0, len(events))
	for _, event := range events {
		records = append(records, store.EventRecord{
			Type:       event.Type,
			Tool:       event.Tool,
			Message:    event.Message,
			Payload:    event.Payload,
			DurationMS: event.DurationMS,
			At:         event.At,
		})
	}
	return records
}

func asMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func mapString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
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

func newID(prefix string) string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s_%x", prefix, time.Now().UTC().Format("20060102T150405"), buf)
}
