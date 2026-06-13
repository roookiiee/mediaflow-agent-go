package tools

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/portfolio/mediaflow-agent-go/internal/llm"
)

type AnalyzeBriefTool struct{}

func (AnalyzeBriefTool) Name() string {
	return "analyze_brief"
}

func (AnalyzeBriefTool) Description() string {
	return "Extract target locale, platform, source script, voice style, and product objective from a media localization brief."
}

func (AnalyzeBriefTool) Run(_ context.Context, _ Context, args map[string]any) (Result, error) {
	brief := stringArg(args, "brief")
	script := firstNonEmpty(stringArg(args, "source_script"), extractScript(brief), demoScript(brief))
	targetLocale := firstNonEmpty(stringArg(args, "target_locale"), detectTargetLocale(brief), "English")
	channel := firstNonEmpty(stringArg(args, "channel"), detectChannel(brief), "TikTok")
	voiceStyle := detectVoiceStyle(brief)
	objective := "localize video content and produce a ready-to-review dubbing plan"
	if strings.Contains(strings.ToLower(brief), "标题") || strings.Contains(strings.ToLower(brief), "content") {
		objective = "localize the script and generate a publishable content pack"
	}

	data := map[string]any{
		"objective":      objective,
		"source_script":  script,
		"target_locale":  targetLocale,
		"channel":        channel,
		"voice_style":    voiceStyle,
		"constraints":    []string{"short sentences", "platform-native hook", "human approval for cloned voices"},
		"estimated_secs": estimateDurationSeconds(script),
	}

	return Result{
		Text: fmt.Sprintf("Target=%s, channel=%s, voice=%s", targetLocale, channel, voiceStyle),
		Data: data,
	}, nil
}

type LoadSkillTool struct{}

func (LoadSkillTool) Name() string {
	return "load_skill"
}

func (LoadSkillTool) Description() string {
	return "Load a scenario skill by name so the agent can apply domain-specific workflow rules."
}

func (LoadSkillTool) Run(_ context.Context, toolCtx Context, args map[string]any) (Result, error) {
	if toolCtx.Skills == nil {
		return Result{Text: "No skill manager configured."}, nil
	}
	name := stringArg(args, "name")
	skill, ok := toolCtx.Skills.Load(name)
	if !ok {
		return Result{}, fmt.Errorf("skill %q not found", name)
	}
	return Result{
		Text: skill.Body,
		Data: map[string]any{"name": skill.Name, "summary": skill.Summary, "path": skill.Path},
	}, nil
}

type RetrieveKnowledgeTool struct{}

func (RetrieveKnowledgeTool) Name() string {
	return "retrieve_knowledge"
}

func (RetrieveKnowledgeTool) Description() string {
	return "Retrieve useful product, localization, and metrics knowledge from the local knowledge base."
}

func (RetrieveKnowledgeTool) Run(ctx context.Context, toolCtx Context, args map[string]any) (Result, error) {
	query := stringArg(args, "query")
	if toolCtx.Vector != nil {
		hits, err := toolCtx.Vector.Search(ctx, query, 4)
		if err == nil && len(hits) > 0 {
			var builder strings.Builder
			for i, hit := range hits {
				builder.WriteString(fmt.Sprintf("[%d] %s score=%.4f\n%s\n\n", i+1, hit.Source, hit.Score, hit.Text))
			}
			return Result{
				Text: strings.TrimSpace(builder.String()),
				Data: map[string]any{"mode": "milvus", "hits": hits},
			}, nil
		}
	}

	snippets, err := retrieveMarkdown(toolCtx.KnowledgeDir, query, 4)
	if err != nil {
		return Result{}, err
	}
	if len(snippets) == 0 {
		return Result{Text: "No knowledge snippets found.", Data: []knowledgeSnippet{}}, nil
	}
	var builder strings.Builder
	for i, snippet := range snippets {
		builder.WriteString(fmt.Sprintf("[%d] %s\n%s\n\n", i+1, snippet.Source, snippet.Text))
	}
	return Result{
		Text: strings.TrimSpace(builder.String()),
		Data: map[string]any{"mode": "local_markdown", "hits": snippets},
	}, nil
}

type TranslateScriptTool struct{}

func (TranslateScriptTool) Name() string {
	return "translate_script"
}

func (TranslateScriptTool) Description() string {
	return "Localize the source script for a target locale and keep it suitable for dubbing and subtitles."
}

func (TranslateScriptTool) Run(ctx context.Context, toolCtx Context, args map[string]any) (Result, error) {
	script := stringArg(args, "script")
	targetLocale := firstNonEmpty(stringArg(args, "target_locale"), "English")
	channel := firstNonEmpty(stringArg(args, "channel"), "TikTok")
	guidance := stringArg(args, "guidance")

	if toolCtx.LLM != nil && !strings.HasPrefix(toolCtx.LLM.Name(), "mock") {
		content, usage, err := toolCtx.LLM.Complete(ctx, []llm.Message{
			{Role: "system", Content: "You are a senior video localization producer. Return concise Markdown."},
			{Role: "user", Content: fmt.Sprintf("Localize this script for %s on %s. Keep sentences short for TTS and preserve meaning.\n\nGuidance:\n%s\n\nScript:\n%s", targetLocale, channel, guidance, script)},
		}, llm.CompleteOptions{MaxTokens: 900, Temperature: 0.3})
		if err == nil {
			return Result{Text: content, Data: map[string]any{"mode": "llm", "usage": usage}}, nil
		}
	}

	segments := buildDemoSegments(script, targetLocale)
	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteString(fmt.Sprintf("%02d. %s\n", segment.Index, segment.Text))
	}
	return Result{
		Text: strings.TrimSpace(builder.String()),
		Data: map[string]any{"mode": "demo", "segments": segments},
	}, nil
}

type BuildTTSPlanTool struct{}

func (BuildTTSPlanTool) Name() string {
	return "build_tts_plan"
}

func (BuildTTSPlanTool) Description() string {
	return "Build a TTS and optional voice-cloning execution plan with timing, approval, and cost controls."
}

func (BuildTTSPlanTool) Run(_ context.Context, _ Context, args map[string]any) (Result, error) {
	script := firstNonEmpty(stringArg(args, "localized_script"), stringArg(args, "script"))
	voiceStyle := firstNonEmpty(stringArg(args, "voice_style"), "warm product narrator")
	estimatedSeconds := estimateDurationSeconds(script)
	segments := math.Max(1, math.Ceil(float64(estimatedSeconds)/8))

	plan := map[string]any{
		"voice_style":        voiceStyle,
		"estimated_seconds":  estimatedSeconds,
		"segment_count":      int(segments),
		"voice_clone_policy": "require signed consent and reviewer approval before cloning",
		"latency_budget_ms":  1800,
		"retry_policy":       "retry TTS once, then fall back to neutral preset voice",
		"output_formats":     []string{"wav", "srt", "mp4 dubbing manifest"},
	}

	text := fmt.Sprintf("TTS plan: %d segments, %ds estimated audio, voice=%s, consent gate enabled.", int(segments), estimatedSeconds, voiceStyle)
	return Result{Text: text, Data: plan}, nil
}

type GenerateContentPackTool struct{}

func (GenerateContentPackTool) Name() string {
	return "generate_content_pack"
}

func (GenerateContentPackTool) Description() string {
	return "Generate platform titles, captions, hashtags, and publishing notes from the localized script."
}

func (GenerateContentPackTool) Run(ctx context.Context, toolCtx Context, args map[string]any) (Result, error) {
	localizedScript := stringArg(args, "localized_script")
	channel := firstNonEmpty(stringArg(args, "channel"), "TikTok")
	targetLocale := firstNonEmpty(stringArg(args, "target_locale"), "English")

	if toolCtx.LLM != nil && !strings.HasPrefix(toolCtx.LLM.Name(), "mock") {
		content, usage, err := toolCtx.LLM.Complete(ctx, []llm.Message{
			{Role: "system", Content: "You are an AI content strategist. Return concise Markdown."},
			{Role: "user", Content: fmt.Sprintf("Create a content pack for %s in %s. Include titles, caption, hashtags, and a CTA.\n\nLocalized script:\n%s", channel, targetLocale, localizedScript)},
		}, llm.CompleteOptions{MaxTokens: 500, Temperature: 0.45})
		if err == nil {
			return Result{Text: content, Data: map[string]any{"mode": "llm", "usage": usage}}, nil
		}
	}

	pack := map[string]any{
		"titles": []string{
			fmt.Sprintf("Make %s Videos Feel Native", targetLocale),
			"From Raw Script to Ready-to-Dub Content",
			"AI Localization Built for Fast Teams",
		},
		"caption":  fmt.Sprintf("A %s-ready localized video workflow: script, dubbing plan, and quality score in one run.", channel),
		"hashtags": []string{"#AI", "#Localization", "#TTS", "#ContentOps"},
		"cta":      "Review the dubbing plan, then ship the localized version.",
	}
	return Result{Text: renderContentPack(pack), Data: pack}, nil
}

type ScoreQualityTool struct{}

func (ScoreQualityTool) Name() string {
	return "score_quality"
}

func (ScoreQualityTool) Description() string {
	return "Score the generated artifacts with product metrics: quality proxy, risk, latency, and cost estimate."
}

func (ScoreQualityTool) Run(_ context.Context, _ Context, args map[string]any) (Result, error) {
	sourceScript := stringArg(args, "source_script")
	localizedScript := stringArg(args, "localized_script")
	hasTTS := stringArg(args, "tts_plan") != ""
	hasContent := stringArg(args, "content_pack") != ""

	sourceLen := len([]rune(sourceScript))
	localizedLen := len([]rune(localizedScript))
	lengthRatio := 1.0
	if sourceLen > 0 {
		lengthRatio = float64(localizedLen) / float64(sourceLen)
	}

	quality := 62.0
	if localizedLen > 80 {
		quality += 12
	}
	if hasTTS {
		quality += 10
	}
	if hasContent {
		quality += 8
	}
	if lengthRatio > 0.6 && lengthRatio < 1.8 {
		quality += 8
	}
	if quality > 98 {
		quality = 98
	}

	estimatedSeconds := estimateDurationSeconds(localizedScript)
	metrics := map[string]any{
		"quality_score":        round(quality, 1),
		"success_rate_proxy":   round(quality/100, 3),
		"estimated_audio_secs": estimatedSeconds,
		"estimated_cost_usd":   round(float64(estimatedSeconds)*0.0009+0.012, 4),
		"length_ratio":         round(lengthRatio, 2),
		"review_required":      true,
		"generated_at":         time.Now().UTC().Format(time.RFC3339),
	}
	text := fmt.Sprintf("Quality %.1f/100, success proxy %.2f, est. cost $%.4f.", metrics["quality_score"], metrics["success_rate_proxy"], metrics["estimated_cost_usd"])
	return Result{Text: text, Data: metrics}, nil
}

type localizedSegment struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

type knowledgeSnippet struct {
	Source string `json:"source"`
	Text   string `json:"text"`
	Score  int    `json:"score"`
}

func retrieveMarkdown(dir, query string, limit int) ([]knowledgeSnippet, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	queryTokens := tokenSet(query)
	var snippets []knowledgeSnippet
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		for _, chunk := range splitParagraphs(string(body)) {
			score := scoreChunk(queryTokens, chunk)
			if score == 0 && len(queryTokens) > 0 {
				continue
			}
			snippets = append(snippets, knowledgeSnippet{Source: entry.Name(), Text: chunk, Score: score})
		}
	}
	sort.Slice(snippets, func(i, j int) bool {
		if snippets[i].Score == snippets[j].Score {
			return snippets[i].Source < snippets[j].Source
		}
		return snippets[i].Score > snippets[j].Score
	})
	if limit > 0 && len(snippets) > limit {
		snippets = snippets[:limit]
	}
	return snippets, nil
}

func scoreChunk(tokens map[string]struct{}, chunk string) int {
	lower := strings.ToLower(chunk)
	score := 0
	for token := range tokens {
		if strings.Contains(lower, token) {
			score++
		}
	}
	return score
}

func splitParagraphs(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	chunks := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if len([]rune(item)) > 900 {
			item = string([]rune(item)[:900])
		}
		chunks = append(chunks, item)
	}
	return chunks
}

func buildDemoSegments(script, targetLocale string) []localizedSegment {
	lines := compactLines(script)
	if len(lines) == 0 {
		lines = []string{"Introduce the product problem.", "Show the AI workflow.", "End with a clear call to action."}
	}
	segments := make([]localizedSegment, 0, len(lines))
	for i, line := range lines {
		segments = append(segments, localizedSegment{
			Index: i + 1,
			Text:  fmt.Sprintf("[%s demo localization] %s", targetLocale, normalizeSentence(line)),
		})
	}
	return segments
}

func compactLines(script string) []string {
	replacer := strings.NewReplacer("。", ".\n", "！", "!\n", "？", "?\n")
	script = replacer.Replace(script)
	raw := strings.Split(script, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func normalizeSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	if len([]rune(text)) > 110 {
		text = string([]rune(text)[:110]) + "..."
	}
	return text
}

func renderContentPack(pack map[string]any) string {
	var builder strings.Builder
	if titles, ok := pack["titles"].([]string); ok {
		builder.WriteString("Titles:\n")
		for _, title := range titles {
			builder.WriteString("- " + title + "\n")
		}
	}
	builder.WriteString("\nCaption:\n")
	builder.WriteString(fmt.Sprint(pack["caption"]))
	builder.WriteString("\n\nHashtags:\n")
	if hashtags, ok := pack["hashtags"].([]string); ok {
		builder.WriteString(strings.Join(hashtags, " "))
	}
	builder.WriteString("\n\nCTA:\n")
	builder.WriteString(fmt.Sprint(pack["cta"]))
	return strings.TrimSpace(builder.String())
}

func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
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

func extractScript(brief string) string {
	if strings.Contains(brief, "\n") {
		lines := compactLines(brief)
		if len(lines) > 1 {
			return strings.Join(lines[1:], "\n")
		}
	}
	return ""
}

func demoScript(brief string) string {
	if strings.TrimSpace(brief) == "" {
		return "Introduce the AI product, explain the workflow, and invite users to try it."
	}
	return "Open with the user's pain point. Show how the AI workflow saves time. Close with a concrete call to action."
}

func detectTargetLocale(brief string) string {
	lower := strings.ToLower(brief)
	switch {
	case strings.Contains(lower, "英文") || strings.Contains(lower, "english") || strings.Contains(lower, "en"):
		return "English"
	case strings.Contains(lower, "日文") || strings.Contains(lower, "japanese"):
		return "Japanese"
	case strings.Contains(lower, "韩文") || strings.Contains(lower, "korean"):
		return "Korean"
	case strings.Contains(lower, "西班牙") || strings.Contains(lower, "spanish"):
		return "Spanish"
	default:
		return ""
	}
}

func detectChannel(brief string) string {
	lower := strings.ToLower(brief)
	switch {
	case strings.Contains(lower, "tiktok"):
		return "TikTok"
	case strings.Contains(lower, "youtube"):
		return "YouTube Shorts"
	case strings.Contains(lower, "b站") || strings.Contains(lower, "bilibili"):
		return "Bilibili"
	case strings.Contains(lower, "小红书"):
		return "Xiaohongshu"
	default:
		return ""
	}
}

func detectVoiceStyle(brief string) string {
	lower := strings.ToLower(brief)
	switch {
	case strings.Contains(lower, "克隆") || strings.Contains(lower, "clone"):
		return "consent-gated cloned voice"
	case strings.Contains(lower, "年轻") || strings.Contains(lower, "energetic"):
		return "energetic young narrator"
	case strings.Contains(lower, "专业") || strings.Contains(lower, "professional"):
		return "professional product narrator"
	default:
		return "warm product narrator"
	}
}

func estimateDurationSeconds(script string) int {
	runes := len([]rune(strings.TrimSpace(script)))
	if runes == 0 {
		return 0
	}
	seconds := int(math.Ceil(float64(runes) / 9.5))
	if seconds < 5 {
		return 5
	}
	return seconds
}

func tokenSet(text string) map[string]struct{} {
	tokens := map[string]struct{}{}
	var builder strings.Builder
	flush := func() {
		if builder.Len() >= 2 {
			tokens[strings.ToLower(builder.String())] = struct{}{}
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

func round(value float64, precision int) float64 {
	factor := math.Pow(10, float64(precision))
	return math.Round(value*factor) / factor
}
