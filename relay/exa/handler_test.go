package exa

import "testing"

func TestEstimateRequests(t *testing.T) {
	for _, endpoint := range []string{endpointSearch, endpointContents} {
		t.Run(endpoint, func(t *testing.T) {
			meta, err := estimateRequests(endpoint, []byte(`{"query":"llm"}`))
			if err != nil {
				t.Fatalf("estimateRequests() error = %v", err)
			}
			if meta.Requests != 1 {
				t.Fatalf("Requests = %d, want 1", meta.Requests)
			}
			if meta.Detail["endpoint"] != endpoint {
				t.Fatalf("endpoint detail = %v, want %s", meta.Detail["endpoint"], endpoint)
			}
		})
	}
}

func TestEstimateRequestsRejectsInvalidJSON(t *testing.T) {
	if _, err := estimateRequests(endpointSearch, []byte(`{`)); err == nil {
		t.Fatal("estimateRequests() error = nil, want invalid JSON error")
	}
}

func TestExaCostDollarsTotal(t *testing.T) {
	cost := exaCostDollarsTotal([]byte(`{"costDollars":{"total":0.007}}`))
	if cost == nil || *cost != 0.007 {
		t.Fatalf("cost = %v, want 0.007", cost)
	}
}
