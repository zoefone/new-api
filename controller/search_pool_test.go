package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestParseSearchPoolImportPlainLines(t *testing.T) {
	accounts, errs := parseSearchPoolImport(searchPoolImportRequest{
		DefaultProvider: model.SearchPoolProviderTavily,
		Text:            "tvly-key-a\ntvly-key-b\n",
	})
	if len(errs) != 0 {
		t.Fatalf("errors = %#v, want none", errs)
	}
	if len(accounts) != 2 {
		t.Fatalf("len(accounts) = %d, want 2", len(accounts))
	}
	if accounts[0].Provider != model.SearchPoolProviderTavily || accounts[0].ApiKey != "tvly-key-a" {
		t.Fatalf("account[0] = %#v", accounts[0])
	}
	if accounts[0].KeyIndex != -1 || !accounts[0].Enabled {
		t.Fatalf("account defaults = keyIndex %d enabled %v", accounts[0].KeyIndex, accounts[0].Enabled)
	}
}

func TestParseSearchPoolImportCSVHeader(t *testing.T) {
	text := "provider,api_key,api_key_id,monthly_limit,base_url,proxy,enabled\nexa,exa-key,key-id,123,https://api.exa.ai,http://127.0.0.1:7890,false"
	accounts, errs := parseSearchPoolImport(searchPoolImportRequest{Text: text})
	if len(errs) != 0 {
		t.Fatalf("errors = %#v, want none", errs)
	}
	if len(accounts) != 1 {
		t.Fatalf("len(accounts) = %d, want 1", len(accounts))
	}
	account := accounts[0]
	if account.Provider != model.SearchPoolProviderExa || account.ApiKey != "exa-key" || account.ApiKeyId != "key-id" {
		t.Fatalf("account identity = %#v", account)
	}
	if account.MonthlyLimit != 123 || account.BaseURL != "https://api.exa.ai" || account.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("account settings = %#v", account)
	}
	if account.Enabled {
		t.Fatalf("account.Enabled = true, want false")
	}
}

func TestParseSearchPoolImportJSONSingle(t *testing.T) {
	text := `{"provider":"tavily","api_key":"tvly-key","project_id":"project-a","monthly_limit":456}`
	accounts, errs := parseSearchPoolImport(searchPoolImportRequest{Text: text})
	if len(errs) != 0 {
		t.Fatalf("errors = %#v, want none", errs)
	}
	if len(accounts) != 1 {
		t.Fatalf("len(accounts) = %d, want 1", len(accounts))
	}
	if accounts[0].ProjectId != "project-a" || accounts[0].MonthlyLimit != 456 {
		t.Fatalf("account = %#v", accounts[0])
	}
}
