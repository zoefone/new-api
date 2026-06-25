package controller

import "testing"

func TestParseTavilyUsagePayloadOfficialShape(t *testing.T) {
	body := []byte(`{
		"key": {
			"usage": 150,
			"limit": 1000,
			"search_usage": 100,
			"extract_usage": 25
		},
		"account": {
			"current_plan": "Bootstrap",
			"plan_usage": 500,
			"plan_limit": 15000
		}
	}`)

	usedCredits, monthlyLimitCredits, upstreamData, err := parseTavilyUsagePayload(body)
	if err != nil {
		t.Fatalf("parseTavilyUsagePayload() error = %v", err)
	}
	if usedCredits == nil || *usedCredits != 150 {
		t.Fatalf("usedCredits = %v, want 150", usedCredits)
	}
	if monthlyLimitCredits == nil || *monthlyLimitCredits != 1000 {
		t.Fatalf("monthlyLimitCredits = %v, want 1000", monthlyLimitCredits)
	}
	if upstreamData["key"] == nil || upstreamData["account"] == nil {
		t.Fatalf("upstreamData missing key/account: %#v", upstreamData)
	}
}

func TestParseTavilyUsagePayloadFallbackFields(t *testing.T) {
	body := []byte(`{"used_credits":"42","credit_limit":"999"}`)

	usedCredits, monthlyLimitCredits, _, err := parseTavilyUsagePayload(body)
	if err != nil {
		t.Fatalf("parseTavilyUsagePayload() error = %v", err)
	}
	if usedCredits == nil || *usedCredits != 42 {
		t.Fatalf("usedCredits = %v, want 42", usedCredits)
	}
	if monthlyLimitCredits == nil || *monthlyLimitCredits != 999 {
		t.Fatalf("monthlyLimitCredits = %v, want 999", monthlyLimitCredits)
	}
}

func TestParseTavilyUsagePayloadInvalidJSON(t *testing.T) {
	if _, _, _, err := parseTavilyUsagePayload([]byte(`{`)); err == nil {
		t.Fatal("parseTavilyUsagePayload() error = nil, want invalid JSON error")
	}
}
