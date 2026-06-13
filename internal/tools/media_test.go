package tools

import (
	"context"
	"testing"
)

func TestAnalyzeBriefDetectsMediaFields(t *testing.T) {
	result, err := AnalyzeBriefTool{}.Run(context.Background(), Context{}, map[string]any{
		"brief": "把中文视频本地化成英文 TikTok 版本，需要 TTS 标题",
	})
	if err != nil {
		t.Fatalf("analyze brief failed: %v", err)
	}
	data := result.Data.(map[string]any)
	if data["target_locale"] != "English" {
		t.Fatalf("expected English target locale, got %v", data["target_locale"])
	}
	if data["channel"] != "TikTok" {
		t.Fatalf("expected TikTok channel, got %v", data["channel"])
	}
}

func TestBuildTTSPlanAlwaysHasConsentGate(t *testing.T) {
	result, err := BuildTTSPlanTool{}.Run(context.Background(), Context{}, map[string]any{
		"localized_script": "Short localized script for a product demo.",
		"voice_style":      "consent-gated cloned voice",
	})
	if err != nil {
		t.Fatalf("build tts plan failed: %v", err)
	}
	data := result.Data.(map[string]any)
	if data["voice_clone_policy"] == "" {
		t.Fatalf("expected voice clone policy to be present")
	}
}
