package tavily

import "testing"

func TestSearchCredits(t *testing.T) {
	tests := []struct {
		name  string
		depth string
		want  int
	}{
		{name: "empty defaults basic", depth: "", want: 1},
		{name: "basic", depth: "basic", want: 1},
		{name: "fast", depth: "fast", want: 1},
		{name: "ultra fast", depth: "ultra-fast", want: 1},
		{name: "advanced", depth: "advanced", want: 2},
		{name: "advanced case insensitive", depth: "Advanced", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchCredits(tt.depth); got != tt.want {
				t.Fatalf("searchCredits(%q) = %d, want %d", tt.depth, got, tt.want)
			}
		})
	}
}

func TestExtractCredits(t *testing.T) {
	tests := []struct {
		name          string
		successfulURL int
		depth         string
		want          int
	}{
		{name: "zero success basic", successfulURL: 0, depth: "basic", want: 0},
		{name: "one success basic", successfulURL: 1, depth: "basic", want: 1},
		{name: "five success basic", successfulURL: 5, depth: "basic", want: 1},
		{name: "six success basic", successfulURL: 6, depth: "basic", want: 2},
		{name: "ten success basic", successfulURL: 10, depth: "basic", want: 2},
		{name: "eleven success basic", successfulURL: 11, depth: "basic", want: 3},
		{name: "one success advanced", successfulURL: 1, depth: "advanced", want: 2},
		{name: "six success advanced", successfulURL: 6, depth: "advanced", want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractCredits(tt.successfulURL, tt.depth); got != tt.want {
				t.Fatalf("extractCredits(%d, %q) = %d, want %d", tt.successfulURL, tt.depth, got, tt.want)
			}
		})
	}
}

func TestExtractURLCount(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    int
		wantErr bool
	}{
		{name: "single url", body: `{"urls":"https://example.com"}`, want: 1},
		{name: "multiple urls", body: `{"urls":["https://example.com","https://example.org"]}`, want: 2},
		{name: "empty string", body: `{"urls":" "}`, wantErr: true},
		{name: "empty array", body: `{"urls":[]}`, wantErr: true},
		{name: "missing urls", body: `{}`, wantErr: true},
		{name: "invalid json", body: `{`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractURLCount([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("extractURLCount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("extractURLCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExtractSuccessCount(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    int
		wantErr bool
	}{
		{name: "no results", body: `{"results":[]}`, want: 0},
		{name: "two results", body: `{"results":[{"url":"https://example.com"},{"url":"https://example.org"}]}`, want: 2},
		{name: "failed results ignored", body: `{"results":[{}],"failed_results":[{},{}]}`, want: 1},
		{name: "invalid json", body: `{`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractSuccessCount([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("extractSuccessCount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("extractSuccessCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEstimateCreditsRejectsInvalidSearchJSON(t *testing.T) {
	if _, err := estimateCredits(endpointSearch, []byte(`{`)); err == nil {
		t.Fatal("estimateCredits() error = nil, want invalid JSON error")
	}
}
