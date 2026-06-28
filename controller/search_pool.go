package controller

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type searchPoolImportRequest struct {
	Text            string                    `json:"text"`
	Format          string                    `json:"format"`
	DefaultProvider string                    `json:"default_provider"`
	Replace         bool                      `json:"replace"`
	Accounts        []searchPoolImportAccount `json:"accounts"`
	Connect         bool                      `json:"connect"`
	GenerateAPIKey  bool                      `json:"generate_api_key"`
	Group           string                    `json:"group"`
	Tag             string                    `json:"tag"`
	ChannelPrefix   string                    `json:"channel_prefix"`
	TokenName       string                    `json:"token_name"`
	TokenUserId     int                       `json:"token_user_id"`
	TokenGroup      string                    `json:"token_group"`
	TokenUnlimited  *bool                     `json:"token_unlimited"`
	TokenQuota      int                       `json:"token_quota"`
}

type searchPoolImportAccount struct {
	Provider     string `json:"provider"`
	Name         string `json:"name"`
	ApiKey       string `json:"api_key"`
	Key          string `json:"key"`
	ApiKeyId     string `json:"api_key_id"`
	ProjectId    string `json:"project_id"`
	MonthlyLimit int    `json:"monthly_limit"`
	BaseURL      string `json:"base_url"`
	Proxy        string `json:"proxy"`
	PaidUntil    any    `json:"paid_until"`
	Remark       string `json:"remark"`
	Enabled      *bool  `json:"enabled"`
	Status       *int   `json:"status"`
}

type searchPoolApplyRequest struct {
	Provider          string `json:"provider"`
	Group             string `json:"group"`
	Tag               string `json:"tag"`
	ChannelPrefix     string `json:"channel_prefix"`
	CreateToken       bool   `json:"create_token"`
	TokenName         string `json:"token_name"`
	TokenUserId       int    `json:"token_user_id"`
	TokenGroup        string `json:"token_group"`
	TokenUnlimited    *bool  `json:"token_unlimited"`
	TokenQuota        int    `json:"token_quota"`
	ModelLimitEnabled *bool  `json:"model_limit_enabled"`
}

type searchPoolUpdateRequest struct {
	Name         *string `json:"name"`
	ApiKeyId     *string `json:"api_key_id"`
	ProjectId    *string `json:"project_id"`
	MonthlyLimit *int    `json:"monthly_limit"`
	BaseURL      *string `json:"base_url"`
	Proxy        *string `json:"proxy"`
	PaidUntil    *int64  `json:"paid_until"`
	Remark       *string `json:"remark"`
	Enabled      *bool   `json:"enabled"`
	Status       *int    `json:"status"`
}

type searchPoolImportResult struct {
	Imported int                      `json:"imported"`
	Skipped  int                      `json:"skipped"`
	Errors   []string                 `json:"errors"`
	Apply    *searchPoolApplyResponse `json:"apply,omitempty"`
}

type searchPoolProviderApplyResult struct {
	Provider      string   `json:"provider"`
	ChannelId     int      `json:"channel_id"`
	ChannelName   string   `json:"channel_name"`
	KeyCount      int      `json:"key_count"`
	Models        string   `json:"models"`
	BaseURL       string   `json:"base_url"`
	ProxyWarnings []string `json:"proxy_warnings,omitempty"`
}

type searchPoolApplyResponse struct {
	Success  bool                            `json:"success"`
	Message  string                          `json:"message,omitempty"`
	BaseURL  string                          `json:"base_url"`
	Channels []searchPoolProviderApplyResult `json:"channels"`
	Token    *searchPoolGeneratedToken       `json:"token,omitempty"`
}

type searchPoolGeneratedToken struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Key     string `json:"key"`
	FullKey string `json:"full_key"`
	Group   string `json:"group"`
}

type searchPoolSyncRequest struct {
	Provider string `json:"provider"`
	Tag      string `json:"tag"`
	KeyIndex *int   `json:"key_index"`
}

func GetSearchPoolSummary(c *gin.Context) {
	summaries, err := model.ListSearchPoolSummaries()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"providers": summaries})
}

func ListSearchPoolAccounts(c *gin.Context) {
	provider := c.Query("provider")
	var enabled *bool
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			common.ApiError(c, errors.New("enabled must be true or false"))
			return
		}
		enabled = &parsed
	}
	accounts, err := model.ListSearchPoolAccounts(provider, enabled)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, accounts)
}

func ImportSearchPoolAccounts(c *gin.Context) {
	req := searchPoolImportRequest{Format: "auto"}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	accounts, parseErrors := parseSearchPoolImport(req)
	result := searchPoolImportResult{Errors: parseErrors}
	for i := range accounts {
		_, changed, err := model.UpsertSearchPoolAccount(&accounts[i], req.Replace)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: %s", i+1, err.Error()))
			continue
		}
		if changed {
			result.Imported++
		} else {
			result.Skipped++
		}
	}
	if req.Connect || req.GenerateAPIKey {
		applyReq := searchPoolApplyRequest{
			Provider:          req.DefaultProvider,
			Group:             req.Group,
			Tag:               req.Tag,
			ChannelPrefix:     req.ChannelPrefix,
			CreateToken:       req.GenerateAPIKey,
			TokenName:         req.TokenName,
			TokenUserId:       req.TokenUserId,
			TokenGroup:        req.TokenGroup,
			TokenUnlimited:    req.TokenUnlimited,
			TokenQuota:        req.TokenQuota,
			ModelLimitEnabled: common.GetPointer(true),
		}
		applied, err := buildSearchPoolApplyResponse(c, applyReq)
		if err != nil {
			result.Errors = append(result.Errors, "apply: "+err.Error())
		} else {
			result.Apply = applied
		}
	}
	common.ApiSuccess(c, result)
}

func UpdateSearchPoolAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, errors.New("invalid account id"))
		return
	}
	account, err := model.GetSearchPoolAccountById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req := searchPoolUpdateRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Name != nil {
		account.Name = strings.TrimSpace(*req.Name)
	}
	if req.ApiKeyId != nil {
		account.ApiKeyId = strings.TrimSpace(*req.ApiKeyId)
	}
	if req.ProjectId != nil {
		account.ProjectId = strings.TrimSpace(*req.ProjectId)
	}
	if req.MonthlyLimit != nil {
		if *req.MonthlyLimit <= 0 {
			common.ApiError(c, errors.New("monthly_limit must be greater than 0"))
			return
		}
		account.MonthlyLimit = *req.MonthlyLimit
	}
	if req.BaseURL != nil {
		account.BaseURL = strings.TrimRight(strings.TrimSpace(*req.BaseURL), "/")
	}
	if req.Proxy != nil {
		account.Proxy = strings.TrimSpace(*req.Proxy)
	}
	if req.PaidUntil != nil {
		account.PaidUntil = *req.PaidUntil
	}
	if req.Remark != nil {
		account.Remark = strings.TrimSpace(*req.Remark)
	}
	if req.Enabled != nil {
		account.Enabled = *req.Enabled
	}
	if req.Status != nil {
		account.Status = *req.Status
	}
	if err := model.UpdateSearchPoolAccount(account); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, account)
}

func DeleteSearchPoolAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, errors.New("invalid account id"))
		return
	}
	if err := model.DeleteSearchPoolAccount(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ApplySearchPoolToNewAPI(c *gin.Context) {
	req := searchPoolApplyRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	resp, err := buildSearchPoolApplyResponse(c, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func buildSearchPoolApplyResponse(c *gin.Context, req searchPoolApplyRequest) (*searchPoolApplyResponse, error) {
	if req.Group = strings.TrimSpace(req.Group); req.Group == "" {
		req.Group = model.SearchPoolDefaultGroup
	}
	if req.Tag = strings.TrimSpace(req.Tag); req.Tag == "" {
		req.Tag = model.SearchPoolDefaultTag
	}
	if req.ChannelPrefix = strings.TrimSpace(req.ChannelPrefix); req.ChannelPrefix == "" {
		req.ChannelPrefix = model.SearchPoolDefaultChannelName
	}
	providers := []string{model.SearchPoolProviderTavily, model.SearchPoolProviderExa}
	if normalized, ok := model.NormalizeSearchPoolProvider(req.Provider); ok {
		providers = []string{normalized}
	} else if strings.TrimSpace(req.Provider) != "" && !strings.EqualFold(req.Provider, "all") {
		return nil, errors.New("provider must be tavily, exa, all, or empty")
	}

	results := make([]searchPoolProviderApplyResult, 0, len(providers))
	for _, provider := range providers {
		result, err := applyOneSearchPoolProvider(req, provider)
		if err != nil {
			return nil, err
		}
		if result.KeyCount > 0 {
			results = append(results, result)
		}
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()

	var generatedToken *searchPoolGeneratedToken
	if req.CreateToken {
		token, err := createSearchPoolToken(c, req)
		if err != nil {
			return nil, err
		}
		generatedToken = token
	}
	return &searchPoolApplyResponse{
		Success:  true,
		BaseURL:  requestBaseURL(c),
		Channels: results,
		Token:    generatedToken,
	}, nil
}

func SyncSearchPoolUsage(c *gin.Context) {
	req := searchPoolSyncRequest{}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		common.ApiError(c, errors.New("invalid request body"))
		return
	}
	if req.Tag = strings.TrimSpace(req.Tag); req.Tag == "" {
		req.Tag = model.SearchPoolDefaultTag
	}
	providers := []string{model.SearchPoolProviderTavily, model.SearchPoolProviderExa}
	if normalized, ok := model.NormalizeSearchPoolProvider(req.Provider); ok {
		providers = []string{normalized}
	} else if strings.TrimSpace(req.Provider) != "" && !strings.EqualFold(req.Provider, "all") {
		common.ApiError(c, errors.New("provider must be tavily, exa, all, or empty"))
		return
	}

	results := make([]gin.H, 0)
	allOK := true
	for _, provider := range providers {
		providerResults, err := syncOneSearchPoolProvider(c, provider, req)
		if err != nil {
			allOK = false
			results = append(results, gin.H{
				"provider": provider,
				"success":  false,
				"message":  err.Error(),
			})
			continue
		}
		for _, result := range providerResults {
			if success, _ := result["success"].(bool); !success {
				allOK = false
			}
			results = append(results, result)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": allOK,
		"data":    results,
	})
}

func syncOneSearchPoolProvider(c *gin.Context, provider string, req searchPoolSyncRequest) ([]gin.H, error) {
	searchPoolChannel, err := model.FindSearchPoolChannel(provider, req.Tag)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []gin.H{{
				"provider": provider,
				"success":  false,
				"message":  "search pool channel not found; apply the pool first",
			}}, nil
		}
		return nil, err
	}
	channel, err := model.GetChannelById(searchPoolChannel.Id, true)
	if err != nil {
		return nil, err
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return []gin.H{{
			"provider":   provider,
			"channel_id": channel.Id,
			"success":    false,
			"message":    "channel has no keys",
		}}, nil
	}

	var indexes []int
	switch provider {
	case model.SearchPoolProviderTavily:
		indexes, err = tavilyUsageTargetIndexes(keys, req.KeyIndex)
	case model.SearchPoolProviderExa:
		indexes, err = exaUsageTargetIndexes(keys, req.KeyIndex)
	default:
		err = errors.New("unsupported provider")
	}
	if err != nil {
		return nil, err
	}

	results := make([]gin.H, 0, len(indexes))
	for _, index := range indexes {
		if err := ensureSearchPoolUsageRowExists(provider, channel.Id, index, keys[index]); err != nil {
			result := gin.H{
				"provider":   provider,
				"channel_id": channel.Id,
				"key_index":  index,
				"success":    false,
				"message":    err.Error(),
			}
			updateSearchPoolSyncAccount(provider, channel.Id, result)
			results = append(results, result)
			continue
		}
		var result gin.H
		switch provider {
		case model.SearchPoolProviderTavily:
			result = syncOneTavilyKeyUsage(c, channel, channel.Id, index, keys[index])
		case model.SearchPoolProviderExa:
			result = syncOneExaKeyUsage(c, channel, channel.Id, index, keys[index])
		}
		result["provider"] = provider
		result["channel_id"] = channel.Id
		updateSearchPoolSyncAccount(provider, channel.Id, result)
		results = append(results, result)
	}
	return results, nil
}

func ensureSearchPoolUsageRowExists(provider string, channelId int, keyIndex int, key string) error {
	switch provider {
	case model.SearchPoolProviderTavily:
		_, err := model.GetOrCreateTavilyKeyUsage(channelId, keyIndex, key)
		return err
	case model.SearchPoolProviderExa:
		_, err := model.GetOrCreateExaKeyUsage(channelId, keyIndex, key)
		return err
	default:
		return errors.New("unsupported provider")
	}
}

func updateSearchPoolSyncAccount(provider string, channelId int, result gin.H) {
	rawIndex, ok := result["key_index"].(int)
	if !ok {
		return
	}
	account, err := model.GetSearchPoolAccountByChannelKeyIndex(provider, channelId, rawIndex)
	if err != nil {
		return
	}
	success, _ := result["success"].(bool)
	if success {
		_ = model.SetSearchPoolAccountLastError(account.Id, "")
		return
	}
	message, _ := result["message"].(string)
	if message == "" {
		message = "usage sync failed"
	}
	_ = model.SetSearchPoolAccountLastError(account.Id, message)
}

func parseSearchPoolImport(req searchPoolImportRequest) ([]model.SearchPoolAccount, []string) {
	accounts := make([]model.SearchPoolAccount, 0)
	errorsOut := make([]string, 0)
	defaultProvider, _ := model.NormalizeSearchPoolProvider(req.DefaultProvider)
	for _, item := range req.Accounts {
		account, err := accountFromImportItem(item, defaultProvider)
		if err != nil {
			errorsOut = append(errorsOut, err.Error())
			continue
		}
		accounts = append(accounts, account)
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return accounts, errorsOut
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" || format == "auto" {
		if strings.HasPrefix(text, "[") || strings.HasPrefix(text, "{") {
			format = "json"
		} else {
			format = "csv"
		}
	}
	switch format {
	case "json":
		parsed, err := parseSearchPoolJSON(text, defaultProvider)
		if err != nil {
			errorsOut = append(errorsOut, err.Error())
		} else {
			accounts = append(accounts, parsed...)
		}
	case "csv", "lines", "line":
		parsed, errs := parseSearchPoolCSV(text, defaultProvider)
		accounts = append(accounts, parsed...)
		errorsOut = append(errorsOut, errs...)
	default:
		errorsOut = append(errorsOut, "unsupported import format")
	}
	return accounts, errorsOut
}

func parseSearchPoolJSON(text string, defaultProvider string) ([]model.SearchPoolAccount, error) {
	items := make([]searchPoolImportAccount, 0)
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		var wrapper struct {
			Accounts []searchPoolImportAccount `json:"accounts"`
		}
		if err := json.Unmarshal([]byte(text), &wrapper); err != nil {
			return nil, err
		}
		items = wrapper.Accounts
		if len(items) == 0 {
			var single searchPoolImportAccount
			if err := json.Unmarshal([]byte(text), &single); err != nil {
				return nil, err
			}
			items = []searchPoolImportAccount{single}
		}
	} else {
		if err := json.Unmarshal([]byte(text), &items); err != nil {
			return nil, err
		}
	}
	accounts := make([]model.SearchPoolAccount, 0, len(items))
	for i, item := range items {
		account, err := accountFromImportItem(item, defaultProvider)
		if err != nil {
			return nil, fmt.Errorf("json row %d: %w", i+1, err)
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func parseSearchPoolCSV(text string, defaultProvider string) ([]model.SearchPoolAccount, []string) {
	reader := csv.NewReader(bytes.NewBufferString(text))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil && err != io.EOF {
		return nil, []string{err.Error()}
	}
	accounts := make([]model.SearchPoolAccount, 0, len(rows))
	errorsOut := make([]string, 0)
	header := map[string]int{}
	start := 0
	if len(rows) > 0 && looksLikeSearchPoolHeader(rows[0]) {
		for idx, column := range rows[0] {
			header[normalizeSearchPoolColumn(column)] = idx
		}
		start = 1
	}
	for rowIndex := start; rowIndex < len(rows); rowIndex++ {
		row := rows[rowIndex]
		if len(row) == 0 || strings.HasPrefix(strings.TrimSpace(row[0]), "#") {
			continue
		}
		item := searchPoolImportAccount{}
		if len(header) > 0 {
			item = importItemFromHeaderRow(row, header)
		} else {
			item = importItemFromLooseRow(row, defaultProvider)
		}
		account, err := accountFromImportItem(item, defaultProvider)
		if err != nil {
			errorsOut = append(errorsOut, fmt.Sprintf("line %d: %s", rowIndex+1, err.Error()))
			continue
		}
		accounts = append(accounts, account)
	}
	return accounts, errorsOut
}

func looksLikeSearchPoolHeader(row []string) bool {
	for _, column := range row {
		switch normalizeSearchPoolColumn(column) {
		case "provider", "api_key", "key", "name", "api_key_id", "monthly_limit", "enabled", "status":
			return true
		}
	}
	return false
}

func normalizeSearchPoolColumn(column string) string {
	column = strings.ToLower(strings.TrimSpace(column))
	column = strings.ReplaceAll(column, "-", "_")
	column = strings.ReplaceAll(column, " ", "_")
	return column
}

func importItemFromHeaderRow(row []string, header map[string]int) searchPoolImportAccount {
	get := func(names ...string) string {
		for _, name := range names {
			if idx, ok := header[name]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
		}
		return ""
	}
	monthlyLimit, _ := strconv.Atoi(get("monthly_limit", "limit", "quota"))
	enabled := parseOptionalBool(get("enabled", "active"))
	status := parseOptionalInt(get("status"))
	return searchPoolImportAccount{
		Provider:     get("provider", "type"),
		Name:         get("name", "account", "account_name"),
		ApiKey:       get("api_key", "apikey", "key", "token"),
		ApiKeyId:     get("api_key_id", "key_id", "exa_key_id"),
		ProjectId:    get("project_id", "project", "tavily_project_id"),
		MonthlyLimit: monthlyLimit,
		BaseURL:      get("base_url", "baseurl", "url"),
		Proxy:        get("proxy", "proxy_url"),
		PaidUntil:    get("paid_until", "expire_at", "expires_at"),
		Remark:       get("remark", "note", "notes"),
		Enabled:      enabled,
		Status:       status,
	}
}

func importItemFromLooseRow(row []string, defaultProvider string) searchPoolImportAccount {
	clean := make([]string, 0, len(row))
	for _, value := range row {
		value = strings.TrimSpace(value)
		if value != "" {
			clean = append(clean, value)
		}
	}
	item := searchPoolImportAccount{}
	if len(clean) == 0 {
		return item
	}
	if provider, ok := model.NormalizeSearchPoolProvider(clean[0]); ok {
		item.Provider = provider
		if len(clean) > 1 {
			item.ApiKey = clean[1]
		}
		if len(clean) > 2 {
			item.Name = clean[2]
		}
		if len(clean) > 3 {
			item.ApiKeyId = clean[3]
		}
		if len(clean) > 4 {
			item.MonthlyLimit, _ = strconv.Atoi(clean[4])
		}
		if len(clean) > 5 {
			item.BaseURL = clean[5]
		}
		if len(clean) > 6 {
			item.Proxy = clean[6]
		}
		return item
	}
	item.Provider = defaultProvider
	item.ApiKey = clean[0]
	if len(clean) > 1 {
		item.Name = clean[1]
	}
	if len(clean) > 2 {
		item.ApiKeyId = clean[2]
	}
	if len(clean) > 3 {
		item.MonthlyLimit, _ = strconv.Atoi(clean[3])
	}
	return item
}

func accountFromImportItem(item searchPoolImportAccount, defaultProvider string) (model.SearchPoolAccount, error) {
	provider := strings.TrimSpace(item.Provider)
	if provider == "" {
		provider = defaultProvider
	}
	key := strings.TrimSpace(item.ApiKey)
	if key == "" {
		key = strings.TrimSpace(item.Key)
	}
	paidUntil, err := parseSearchPoolPaidUntil(item.PaidUntil)
	if err != nil {
		return model.SearchPoolAccount{}, err
	}
	account := model.SearchPoolAccount{
		Provider:     provider,
		Name:         item.Name,
		ApiKey:       key,
		ApiKeyId:     item.ApiKeyId,
		ProjectId:    item.ProjectId,
		MonthlyLimit: item.MonthlyLimit,
		BaseURL:      item.BaseURL,
		Proxy:        item.Proxy,
		PaidUntil:    paidUntil,
		Remark:       item.Remark,
		Enabled:      true,
		Status:       common.ChannelStatusEnabled,
		KeyIndex:     -1,
	}
	if item.Enabled != nil {
		account.Enabled = *item.Enabled
	}
	if item.Status != nil {
		account.Status = *item.Status
	}
	return account, model.NormalizeSearchPoolAccount(&account)
}

func parseOptionalBool(value string) *bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		switch strings.ToLower(value) {
		case "1", "yes", "y", "on", "enabled", "active":
			parsed = true
		case "0", "no", "n", "off", "disabled", "inactive":
			parsed = false
		default:
			return nil
		}
	}
	return &parsed
}

func parseOptionalInt(value string) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseSearchPoolPaidUntil(value any) (int64, error) {
	switch typed := value.(type) {
	case nil:
		return 0, nil
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case float64:
		return int64(typed), nil
	case json.Number:
		return typed.Int64()
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, nil
		}
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			return parsed, nil
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02", "2006/01/02", "2006-01-02 15:04:05"} {
			parsed, err := time.ParseInLocation(layout, text, time.Local)
			if err == nil {
				return parsed.Unix(), nil
			}
		}
		return 0, fmt.Errorf("invalid paid_until: %s", text)
	default:
		return 0, nil
	}
}

func applyOneSearchPoolProvider(req searchPoolApplyRequest, provider string) (searchPoolProviderApplyResult, error) {
	accounts, err := model.ListEnabledSearchPoolAccountsByProvider(provider)
	if err != nil {
		return searchPoolProviderApplyResult{}, err
	}
	if len(accounts) == 0 {
		return searchPoolProviderApplyResult{Provider: provider}, nil
	}
	channelType, ok := model.SearchPoolChannelType(provider)
	if !ok {
		return searchPoolProviderApplyResult{}, errors.New("unsupported provider")
	}
	models := model.SearchPoolProviderModels(provider)
	keys := make([]string, 0, len(accounts))
	baseURL := firstNonEmptyBaseURL(provider, accounts)
	proxy := commonProxyOrEmpty(accounts)
	proxyWarnings := proxyWarnings(accounts, proxy)
	for _, account := range accounts {
		keys = append(keys, account.ApiKey)
	}
	keyText := strings.Join(keys, "\n")
	name := fmt.Sprintf("%s %s", req.ChannelPrefix, model.SearchPoolProviderDisplayName(provider))
	channel, err := model.FindSearchPoolChannel(provider, req.Tag)
	created := false
	if errors.Is(err, gorm.ErrRecordNotFound) {
		created = true
		channel = &model.Channel{}
	} else if err != nil {
		return searchPoolProviderApplyResult{}, err
	}
	baseURLPtr := common.GetPointer(baseURL)
	tagPtr := common.GetPointer(req.Tag)
	weight := uint(100)
	priority := int64(0)
	autoBan := 1
	channel.Type = channelType
	channel.Name = name
	channel.Key = keyText
	channel.Models = models
	channel.Group = req.Group
	channel.BaseURL = baseURLPtr
	channel.Tag = tagPtr
	channel.Weight = &weight
	channel.Priority = &priority
	channel.AutoBan = &autoBan
	channel.Status = common.ChannelStatusEnabled
	channel.ChannelInfo.IsMultiKey = true
	channel.ChannelInfo.MultiKeySize = len(keys)
	channel.ChannelInfo.MultiKeyMode = constant.MultiKeyModePolling
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{}
	channel.ChannelInfo.MultiKeyDisabledReason = map[int]string{}
	channel.ChannelInfo.MultiKeyDisabledTime = map[int]int64{}
	channel.ChannelInfo.MultiKeyPollingIndex = 0
	channel.SetSetting(dto.ChannelSettings{Proxy: proxy})
	if created {
		channel.CreatedTime = common.GetTimestamp()
		if err := channel.Insert(); err != nil {
			return searchPoolProviderApplyResult{}, err
		}
	} else {
		if err := channel.Update(); err != nil {
			return searchPoolProviderApplyResult{}, err
		}
	}
	_ = model.BulkClearSearchPoolLinks(provider, channel.Id)
	for idx := range accounts {
		account := accounts[idx]
		if err := model.UpdateSearchPoolAccountLink(account.Id, channel.Id, idx); err != nil {
			return searchPoolProviderApplyResult{}, err
		}
		if err := ensureSearchPoolUsageRow(provider, channel.Id, idx, account); err != nil {
			_ = model.SetSearchPoolAccountLastError(account.Id, err.Error())
			return searchPoolProviderApplyResult{}, err
		}
	}
	return searchPoolProviderApplyResult{
		Provider:      provider,
		ChannelId:     channel.Id,
		ChannelName:   channel.Name,
		KeyCount:      len(keys),
		Models:        models,
		BaseURL:       baseURL,
		ProxyWarnings: proxyWarnings,
	}, nil
}

func firstNonEmptyBaseURL(provider string, accounts []model.SearchPoolAccount) string {
	for _, account := range accounts {
		if strings.TrimSpace(account.BaseURL) != "" {
			return strings.TrimRight(strings.TrimSpace(account.BaseURL), "/")
		}
	}
	return model.SearchPoolProviderDefaultBaseURL(provider)
}

func commonProxyOrEmpty(accounts []model.SearchPoolAccount) string {
	proxy := ""
	for _, account := range accounts {
		current := strings.TrimSpace(account.Proxy)
		if current == "" {
			continue
		}
		if proxy == "" {
			proxy = current
		}
	}
	return proxy
}

func proxyWarnings(accounts []model.SearchPoolAccount, channelProxy string) []string {
	warnings := make([]string, 0)
	seen := map[string]struct{}{}
	for _, account := range accounts {
		proxy := strings.TrimSpace(account.Proxy)
		if proxy == "" || proxy == channelProxy {
			continue
		}
		if _, ok := seen[proxy]; ok {
			continue
		}
		seen[proxy] = struct{}{}
		warnings = append(warnings, fmt.Sprintf("account %s uses a different per-key proxy; relay will use it when this key is selected", account.Name))
	}
	return warnings
}

func ensureSearchPoolUsageRow(provider string, channelId int, keyIndex int, account model.SearchPoolAccount) error {
	switch provider {
	case model.SearchPoolProviderTavily:
		if _, err := model.GetOrCreateTavilyKeyUsage(channelId, keyIndex, account.ApiKey); err != nil {
			return err
		}
		projectId := account.ProjectId
		if projectId == "" {
			projectId = account.ApiKeyId
		}
		return model.UpdateTavilyKeyUsageSettings(channelId, keyIndex, &account.MonthlyLimit, &projectId)
	case model.SearchPoolProviderExa:
		if _, err := model.GetOrCreateExaKeyUsage(channelId, keyIndex, account.ApiKey); err != nil {
			return err
		}
		apiKeyId := account.ApiKeyId
		if apiKeyId == "" {
			apiKeyId = account.ProjectId
		}
		return model.UpdateExaKeyUsageSettings(channelId, keyIndex, &account.MonthlyLimit, &apiKeyId)
	default:
		return errors.New("unsupported provider")
	}
}

func createSearchPoolToken(c *gin.Context, req searchPoolApplyRequest) (*searchPoolGeneratedToken, error) {
	userId := req.TokenUserId
	if userId <= 0 {
		userId = c.GetInt("id")
	}
	if userId <= 0 {
		return nil, errors.New("token user id is required")
	}
	name := strings.TrimSpace(req.TokenName)
	if name == "" {
		name = "Search Pool API Key"
	}
	if len(name) > 50 {
		name = name[:50]
	}
	group := strings.TrimSpace(req.TokenGroup)
	if group == "" {
		group = req.Group
	}
	unlimited := true
	if req.TokenUnlimited != nil {
		unlimited = *req.TokenUnlimited
	}
	modelLimitEnabled := true
	if req.ModelLimitEnabled != nil {
		modelLimitEnabled = *req.ModelLimitEnabled
	}
	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	token := model.Token{
		UserId:             userId,
		Name:               name,
		Key:                key,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        -1,
		RemainQuota:        req.TokenQuota,
		UnlimitedQuota:     unlimited,
		ModelLimitsEnabled: modelLimitEnabled,
		ModelLimits:        "tavily-search,tavily-extract,exa-search,exa-contents",
		Group:              group,
	}
	if err := token.Insert(); err != nil {
		return nil, err
	}
	return &searchPoolGeneratedToken{Id: token.Id, Name: token.Name, Key: token.Key, FullKey: "sk-" + token.Key, Group: token.Group}, nil
}

func requestBaseURL(c *gin.Context) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := c.Request.Host
	if forwardedHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	return scheme + "://" + host
}
