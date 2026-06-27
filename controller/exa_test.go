package controller

import "testing"

func TestParseExaUsagePayloadOfficialShape(t *testing.T) {
	body := []byte(`{
		"id": "key_abc123",
		"api_key_id": "550e8400-e29b-41d4-a716-446655440000",
		"total_cost_usd": 45.67,
		"cost_breakdown": [
			{"price_name": "Neural Search", "quantity": 1000, "amount_usd": 30},
			{"price_name": "Content Retrieval", "quantity": 500, "amount_usd": 15.67}
		]
	}`)

	usedRequests, monthlyLimit, upstreamData, err := parseExaUsagePayload(body)
	if err != nil {
		t.Fatalf("parseExaUsagePayload() error = %v", err)
	}
	if usedRequests == nil || *usedRequests != 1500 {
		t.Fatalf("usedRequests = %v, want 1500", usedRequests)
	}
	if monthlyLimit != nil {
		t.Fatalf("monthlyLimit = %v, want nil", monthlyLimit)
	}
	if upstreamData["cost_breakdown"] == nil || upstreamData["total_cost_usd"] == nil {
		t.Fatalf("upstreamData missing cost fields: %#v", upstreamData)
	}
}

func TestParseExaUsagePayloadFallbackFields(t *testing.T) {
	body := []byte(`{"used_requests":"42","monthly_limit_requests":"999"}`)

	usedRequests, monthlyLimit, _, err := parseExaUsagePayload(body)
	if err != nil {
		t.Fatalf("parseExaUsagePayload() error = %v", err)
	}
	if usedRequests == nil || *usedRequests != 42 {
		t.Fatalf("usedRequests = %v, want 42", usedRequests)
	}
	if monthlyLimit == nil || *monthlyLimit != 999 {
		t.Fatalf("monthlyLimit = %v, want 999", monthlyLimit)
	}
}

func TestParseExaUsagePayloadInvalidJSON(t *testing.T) {
	if _, _, _, err := parseExaUsagePayload([]byte(`{`)); err == nil {
		t.Fatal("parseExaUsagePayload() error = nil, want invalid JSON error")
	}
}
