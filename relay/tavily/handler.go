package tavily

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ModelSearch  = "tavily-search"
	ModelExtract = "tavily-extract"

	endpointSearch  = "search"
	endpointExtract = "extract"
)

type requestMeta struct {
	Credits int
	Detail  map[string]any
}

type keySelection struct {
	Key   string
	Index int
}

func RelaySearch(c *gin.Context) {
	relay(c, endpointSearch)
}

func RelayExtract(c *gin.Context) {
	relay(c, endpointExtract)
}

func relay(c *gin.Context, endpoint string) {
	rawBody, err := requestBodyBytes(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}

	meta, err := estimateCredits(endpoint, rawBody)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}

	channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	selection, err := selectAvailableKey(c, channelId)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": err.Error(), "type": "upstream_error"}})
		return
	}
	common.SetContextKey(c, constant.ContextKeyChannelKey, selection.Key)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, selection.Index)

	info := relaycommon.GenRelayInfoOpenAI(c, nil)
	info.InitChannelMeta(c)
	if info.ChannelType != constant.ChannelTypeTavily {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "selected channel is not Tavily", "type": "invalid_request_error"}})
		return
	}

	priceData, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}
	info.PriceData = priceData

	preConsumeQuota := priceData.Quota * meta.Credits
	if !priceData.FreeModel {
		if apiErr := service.PreConsumeBilling(c, preConsumeQuota, info); apiErr != nil {
			statusCode := apiErr.StatusCode
			if statusCode == 0 {
				statusCode = http.StatusForbidden
			}
			c.JSON(statusCode, gin.H{"error": apiErr.ToOpenAIError()})
			return
		}
	}

	statusCode, contentType, responseBody, err := doUpstreamRequest(c, info, endpoint, rawBody, selection.Index)
	if err != nil {
		refund(c, info)
		_ = model.SetTavilyKeyLastError(channelId, selection.Index, err.Error())
		setSearchPoolLastError(channelId, selection.Index, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "upstream error: do request failed", "type": "upstream_error"}})
		return
	}

	if statusCode >= http.StatusBadRequest {
		refund(c, info)
		handleUpstreamKeyError(channelId, selection.Key, selection.Index, statusCode, responseBody)
		c.Data(statusCode, contentType, responseBody)
		return
	}

	actualCredits, actualDetail, err := actualCredits(endpoint, rawBody, responseBody)
	if err != nil {
		refund(c, info)
		_ = model.SetTavilyKeyLastError(channelId, selection.Index, err.Error())
		setSearchPoolLastError(channelId, selection.Index, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "upstream response usage could not be parsed", "type": "upstream_error"}})
		return
	}

	actualQuota := priceData.Quota * actualCredits
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, actualQuota)
	model.UpdateChannelUsedQuota(info.ChannelId, actualQuota)
	if !priceData.FreeModel {
		if err := service.SettleBilling(c, info, actualQuota); err != nil {
			logger.LogError(c, "error settling Tavily billing: "+err.Error())
		}
	}
	_ = model.AddTavilyKeyUsedCredits(channelId, selection.Index, actualCredits)
	setSearchPoolLastError(channelId, selection.Index, "")
	recordConsumeLog(c, info, priceData, meta, actualCredits, actualQuota, actualDetail)
	c.Data(statusCode, contentType, responseBody)
}

func requestBodyBytes(c *gin.Context) ([]byte, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("request body is empty")
	}
	return body, nil
}

func estimateCredits(endpoint string, body []byte) (requestMeta, error) {
	switch endpoint {
	case endpointSearch:
		if !json.Valid(body) {
			return requestMeta{}, errors.New("invalid JSON request body")
		}
		depth := jsonString(body, "search_depth")
		credits := searchCredits(depth)
		return requestMeta{
			Credits: credits,
			Detail:  map[string]any{"search_depth": normalizedSearchDepth(depth)},
		}, nil
	case endpointExtract:
		urlCount, err := extractURLCount(body)
		if err != nil {
			return requestMeta{}, err
		}
		depth := jsonString(body, "extract_depth")
		credits := extractCredits(urlCount, depth)
		return requestMeta{
			Credits: credits,
			Detail: map[string]any{
				"extract_depth": normalizedExtractDepth(depth),
				"url_count":     urlCount,
			},
		}, nil
	default:
		return requestMeta{}, fmt.Errorf("unsupported Tavily endpoint: %s", endpoint)
	}
}

func actualCredits(endpoint string, requestBody, responseBody []byte) (int, map[string]any, error) {
	switch endpoint {
	case endpointSearch:
		depth := jsonString(requestBody, "search_depth")
		credits := searchCredits(depth)
		return credits, map[string]any{"search_depth": normalizedSearchDepth(depth)}, nil
	case endpointExtract:
		depth := jsonString(requestBody, "extract_depth")
		successes, err := extractSuccessCount(responseBody)
		if err != nil {
			return 0, nil, err
		}
		credits := extractCredits(successes, depth)
		return credits, map[string]any{
			"extract_depth":        normalizedExtractDepth(depth),
			"successful_url_count": successes,
		}, nil
	default:
		return 0, nil, fmt.Errorf("unsupported Tavily endpoint: %s", endpoint)
	}
}

func searchCredits(depth string) int {
	if strings.EqualFold(depth, "advanced") {
		return 2
	}
	return 1
}

func normalizedSearchDepth(depth string) string {
	switch strings.ToLower(strings.TrimSpace(depth)) {
	case "advanced":
		return "advanced"
	case "fast":
		return "fast"
	case "ultra-fast":
		return "ultra-fast"
	default:
		return "basic"
	}
}

func extractCredits(successfulURLs int, depth string) int {
	if successfulURLs <= 0 {
		return 0
	}
	units := (successfulURLs + 4) / 5
	if strings.EqualFold(depth, "advanced") {
		return units * 2
	}
	return units
}

func normalizedExtractDepth(depth string) string {
	if strings.EqualFold(strings.TrimSpace(depth), "advanced") {
		return "advanced"
	}
	return "basic"
}

func jsonString(body []byte, key string) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}

func extractURLCount(body []byte) (int, error) {
	var payload struct {
		URLs any `json:"urls"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("invalid JSON request body: %w", err)
	}
	switch urls := payload.URLs.(type) {
	case string:
		if strings.TrimSpace(urls) == "" {
			return 0, errors.New("urls cannot be empty")
		}
		return 1, nil
	case []any:
		if len(urls) == 0 {
			return 0, errors.New("urls cannot be empty")
		}
		return len(urls), nil
	default:
		return 0, errors.New("urls must be a string or an array")
	}
}

func extractSuccessCount(responseBody []byte) (int, error) {
	var payload struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return 0, fmt.Errorf("invalid JSON upstream response: %w", err)
	}
	return len(payload.Results), nil
}

func selectAvailableKey(c *gin.Context, channelId int) (*keySelection, error) {
	channel, err := model.CacheGetChannel(channelId)
	if err != nil || channel == nil {
		return nil, fmt.Errorf("failed to load Tavily channel: %w", err)
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return nil, errors.New("Tavily channel has no keys")
	}
	for attempts := 0; attempts < len(keys); attempts++ {
		channel, err = model.CacheGetChannel(channelId)
		if err != nil || channel == nil {
			return nil, fmt.Errorf("failed to reload Tavily channel: %w", err)
		}
		key, index, apiErr := channel.GetNextEnabledKey()
		if apiErr != nil {
			return nil, apiErr
		}
		usage, err := model.GetOrCreateTavilyKeyUsage(channelId, index, key)
		if err != nil {
			return nil, err
		}
		if usage.MonthlyLimitCredits <= 0 || usage.UsedCredits < usage.MonthlyLimitCredits {
			return &keySelection{Key: key, Index: index}, nil
		}
		reason := fmt.Sprintf("Tavily key monthly credits exhausted: %d/%d", usage.UsedCredits, usage.MonthlyLimitCredits)
		model.UpdateChannelStatus(channelId, key, common.ChannelStatusAutoDisabled, reason)
		logger.LogWarn(c, reason)
	}
	return nil, errors.New("no Tavily key with remaining credits")
}

func doUpstreamRequest(c *gin.Context, info *relaycommon.RelayInfo, endpoint string, body []byte, keyIndex int) (int, string, []byte, error) {
	baseURL := strings.TrimRight(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(constant.ChannelBaseURLs[constant.ChannelTypeTavily], "/")
	}
	proxy := info.ChannelSetting.Proxy
	projectID := ""
	if account, err := model.GetSearchPoolAccountByChannelKeyIndex(model.SearchPoolProviderTavily, info.ChannelId, keyIndex); err == nil {
		if account.BaseURL != "" {
			baseURL = strings.TrimRight(account.BaseURL, "/")
		}
		if strings.TrimSpace(account.Proxy) != "" {
			proxy = strings.TrimSpace(account.Proxy)
		}
		projectID = strings.TrimSpace(account.ProjectId)
		if projectID == "" {
			projectID = strings.TrimSpace(account.ApiKeyId)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.LogWarn(c, fmt.Sprintf("search pool Tavily account lookup skipped: channel=%d key_index=%d error=%v", info.ChannelId, keyIndex, err))
	}
	targetURL := relaycommon.GetFullRequestURL(baseURL, "/"+endpoint, info.ChannelType)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return 0, "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	if projectID != "" {
		req.Header.Set("X-Project-ID", projectID)
	}
	if projectID := strings.TrimSpace(c.GetHeader("X-Project-ID")); projectID != "" {
		req.Header.Set("X-Project-ID", projectID)
	}

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return 0, "", nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", nil, err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	if requestID := resp.Header.Get(common.UpstreamRequestIdKey); requestID != "" {
		c.Set(common.UpstreamRequestIdKey, requestID)
	}
	return resp.StatusCode, contentType, respBody, nil
}

func refund(c *gin.Context, info *relaycommon.RelayInfo) {
	if info != nil && info.Billing != nil {
		info.Billing.Refund(c)
	}
}

func handleUpstreamKeyError(channelId int, key string, keyIndex int, statusCode int, body []byte) {
	reason := fmt.Sprintf("Tavily upstream returned HTTP %d", statusCode)
	_ = model.SetTavilyKeyLastError(channelId, keyIndex, string(body))
	setSearchPoolLastError(channelId, keyIndex, string(body))
	switch statusCode {
	case http.StatusPaymentRequired, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		model.UpdateChannelStatus(channelId, key, common.ChannelStatusAutoDisabled, reason)
	}
}

func setSearchPoolLastError(channelId int, keyIndex int, message string) {
	account, err := model.GetSearchPoolAccountByChannelKeyIndex(model.SearchPoolProviderTavily, channelId, keyIndex)
	if err != nil {
		return
	}
	_ = model.SetSearchPoolAccountLastError(account.Id, message)
}

func recordConsumeLog(c *gin.Context, info *relaycommon.RelayInfo, priceData types.PriceData, estimated requestMeta, actualCredits int, actualQuota int, actualDetail map[string]any) {
	if info == nil {
		return
	}
	useTimeSeconds := int(time.Since(info.StartTime).Seconds())
	content := fmt.Sprintf("Tavily credits %d，模型价格 %.4f，分组倍率 %.2f", actualCredits, priceData.ModelPrice, priceData.GroupRatioInfo.GroupRatio)
	other := map[string]any{
		"estimated_credits": estimated.Credits,
		"actual_credits":    actualCredits,
		"estimated_detail":  estimated.Detail,
		"actual_detail":     actualDetail,
		"billing_unit":      "tavily_credit",
	}
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId:      info.ChannelId,
		ModelName:      info.OriginModelName,
		TokenName:      c.GetString("token_name"),
		Quota:          actualQuota,
		Content:        content,
		TokenId:        info.TokenId,
		UseTimeSeconds: useTimeSeconds,
		IsStream:       false,
		Group:          info.UsingGroup,
		Other:          other,
	})
}
