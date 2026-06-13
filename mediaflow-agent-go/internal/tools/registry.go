package tools

import (
	"context"
	"fmt"
	"sort"

	"github.com/portfolio/mediaflow-agent-go/internal/llm"
	"github.com/portfolio/mediaflow-agent-go/internal/rag"
	"github.com/portfolio/mediaflow-agent-go/internal/skills"
)

type Tool interface {
	Name() string
	Description() string
	Run(ctx context.Context, toolCtx Context, args map[string]any) (Result, error)
}

type Context struct {
	LLM          llm.Client
	KnowledgeDir string
	Skills       *skills.Manager
	Vector       rag.Searcher
}

type Result struct {
	Text string `json:"text"`
	Data any    `json:"data,omitempty"`
}

type ToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry(tools []Tool) *Registry {
	registry := &Registry{tools: map[string]Tool{}}
	for _, tool := range tools {
		registry.Register(tool)
	}
	return registry
}

func (r *Registry) Register(tool Tool) {
	if tool == nil {
		return
	}
	name := tool.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = tool
	sort.Strings(r.order)
}

func (r *Registry) Invoke(ctx context.Context, toolCtx Context, name string, args map[string]any) (Result, error) {
	tool, ok := r.tools[name]
	if !ok {
		return Result{}, fmt.Errorf("tool %q not found", name)
	}
	return tool.Run(ctx, toolCtx, args)
}

func (r *Registry) List() []ToolSpec {
	items := make([]ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		items = append(items, ToolSpec{Name: tool.Name(), Description: tool.Description()})
	}
	return items
}

func DefaultTools() []Tool {
	return []Tool{
		AnalyzeBriefTool{},
		LoadSkillTool{},
		RetrieveKnowledgeTool{},
		TranslateScriptTool{},
		BuildTTSPlanTool{},
		GenerateContentPackTool{},
		ScoreQualityTool{},
	}
}
