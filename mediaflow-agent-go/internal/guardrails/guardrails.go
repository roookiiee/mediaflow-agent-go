package guardrails

import "strings"

type Assessment struct {
	Allowed      bool     `json:"allowed"`
	RiskLevel    string   `json:"risk_level"`
	Reasons      []string `json:"reasons,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

func AssessMediaRequest(text string) Assessment {
	lower := strings.ToLower(text)
	assessment := Assessment{
		Allowed:   true,
		RiskLevel: "low",
		Requirements: []string{
			"voice cloning requires proof of speaker consent",
			"generated content should be labeled when required by platform policy",
		},
	}

	blockedTerms := []string{"诈骗", "冒充", "骗", "impersonate", "scam", "phishing"}
	for _, term := range blockedTerms {
		if strings.Contains(lower, term) {
			assessment.Allowed = false
			assessment.RiskLevel = "blocked"
			assessment.Reasons = append(assessment.Reasons, "request may enable impersonation or fraud")
			return assessment
		}
	}

	highRiskTerms := []string{"明星", "名人", "celebrity", "clone", "克隆", "模仿"}
	for _, term := range highRiskTerms {
		if strings.Contains(lower, term) {
			assessment.RiskLevel = "medium"
			assessment.Reasons = append(assessment.Reasons, "voice or likeness usage needs human approval")
			break
		}
	}

	return assessment
}
