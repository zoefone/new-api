package exa

import (
	"bytes"
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
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	ModelSearch   = "exa-search"
	ModelContents = "exa-contents"

	endpointSearch   = "search"
	endpointContents = "contents"
)

type requestMeta struct {
	Requests int
	Detail   map[string]any
}

type keySelection struct {
	Key   string
	Index int
}

func RelaySearch(c *gin.Context) {
	relay(c, endpointSearch)
}

func RelayContents(c *gin.Context) {
	relay(c, endpointContents)
}

func relay(c *gin.Context, endpoint string) {
	rawBody, err := requestBodyBytes(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}

	meta, err := estimateRequests(endpoint, rawBody)
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
	if info.ChannelType != constant.ChannelTypeExa {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "selected channel is not Exa", "type": "invalid_request_error"}})
		return
	}

	priceData, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}
	info.PriceData = priceData

	preConsumeQuota := priceData.Quota * meta.Requests
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

	statusCode, contentType, responseBody, err := doUpstreamRequest(c, info, endpoint, rawBody)
	if err != nil {
		refund(c, info)
		_ = model.SetExaKeyLastError(channelId, selection.Index, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "upstream error: do request failed", "type": "upstream_error"}})
		return
	}

	if statusCode >= http.StatusBadRequest {
		refund(c, info)
		handleUpstreamKeyError(channelId, selection.Key, selection.Index, statusCode, responseBody)
		c.Data(statusCode, contentType, responseBody)
		return
	}

	actualRequests, actualDetail := actualRequests(endpoint, responseBody)
	actualQuota := priceData.Quota * actualRequests
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, actualQuota)
	model.UpdateChannelUsedQuota(info.ChannelId, actualQuota)
	if !priceData.FreeModel {
		if err := service.SettleBilling(c, info, actualQuota); err != nil {
			logger.LogError(c, "error settling Exa billing: "+err.Error())
		}
	}
	_ = model.AddExaKeyUsedCredits(channelId, selection.Index, actualRequests)
	recordConsumeLog(c, info, priceData, meta, actualRequests, actualQuota, actualDetail)
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

func estimateRequests(endpoint string, body []byte) (requestMeta, error) {
	switch endpoint {
	case endpointSearch, endpointContents:
		if !json.Valid(body) {
			return requestMeta{}, errors.New("invalid JSON request body")
		}
		return requestMeta{
			Requests: 1,
			Detail:   map[string]any{"endpoint": endpoint},
		}, nil
	default:
		return requestMeta{}, fmt.Errorf("unsupported Exa endpoint: %s", endpoint)
	}
}

func actualRequests(endpoint string, responseBody []byte) (int, map[string]any) {
	detail := map[string]any{"endpoint": endpoint}
	if cost := exaCostDollarsTotal(responseBody); cost != nil {
		detail["cost_dollars_total"] = *cost
	}
	return 1, detail
}

func exaCostDollarsTotal(responseBody []byte) *float64 {
	payload := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	value, ok := valueFromMapPath(payload, "costDollars", "total")
	if !ok {
		value, ok = valueFromMapPath(payload, "cost_dollars", "total")
	}
	if !ok {
		return nil
	}
	floatValue, ok := floatFromExaJSONValue(value)
	if !ok {
		return nil
	}
	return &floatValue
}

func selectAvailableKey(c *gin.Context, channelId int) (*keySelection, error) {
	channel, err := model.CacheGetChannel(channelId)
	if err != nil || channel == nil {
		return nil, fmt.Errorf("failed to load Exa channel: %w", err)
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return nil, errors.New("Exa channel has no keys")
	}
	for attempts := 0; attempts < len(keys); attempts++ {
		channel, err = model.CacheGetChannel(channelId)
		if err != nil || channel == nil {
			return nil, fmt.Errorf("failed to reload Exa channel: %w", err)
		}
		key, index, apiErr := channel.GetNextEnabledKey()
		if apiErr != nil {
			return nil, apiErr
		}
		usage, err := model.GetOrCreateExaKeyUsage(channelId, index, key)
		if err != nil {
			return nil, err
		}
		if usage.MonthlyLimitCredits <= 0 || usage.UsedCredits < usage.MonthlyLimitCredits {
			return &keySelection{Key: key, Index: index}, nil
		}
		reason := fmt.Sprintf("Exa key monthly requests exhausted: %d/%d", usage.UsedCredits, usage.MonthlyLimitCredits)
		model.UpdateChannelStatus(channelId, key, common.ChannelStatusAutoDisabled, reason)
		logger.LogWarn(c, reason)
	}
	return nil, errors.New("no Exa key with remaining requests")
}

func doUpstreamRequest(c *gin.Context, info *relaycommon.RelayInfo, endpoint string, body []byte) (int, string, []byte, error) {
	baseURL := strings.TrimRight(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(constant.ChannelBaseURLs[constant.ChannelTypeExa], "/")
	}
	targetURL := relaycommon.GetFullRequestURL(baseURL, "/"+endpoint, info.ChannelType)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return 0, "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", info.ApiKey)
	if accept := strings.TrimSpace(c.GetHeader("Accept")); accept != "" {
		req.Header.Set("Accept", accept)
	}

	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
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
	reason := fmt.Sprintf("Exa upstream returned HTTP %d", statusCode)
	_ = model.SetExaKeyLastError(channelId, keyIndex, string(body))
	switch statusCode {
	case http.StatusPaymentRequired, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		model.UpdateChannelStatus(channelId, key, common.ChannelStatusAutoDisabled, reason)
	}
}

func recordConsumeLog(c *gin.Context, info *relaycommon.RelayInfo, priceData types.PriceData, estimated requestMeta, actualRequests int, actualQuota int, actualDetail map[string]any) {
	if info == nil {
		return
	}
	useTimeSeconds := int(time.Since(info.StartTime).Seconds())
	content := fmt.Sprintf("Exa requests %d，模型价格 %.4f，分组倍率 %.2f", actualRequests, priceData.ModelPrice, priceData.GroupRatioInfo.GroupRatio)
	other := map[string]any{
		"estimated_requests": estimated.Requests,
		"actual_requests":    actualRequests,
		"estimated_detail":   estimated.Detail,
		"actual_detail":      actualDetail,
		"billing_unit":       "exa_request",
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

func valueFromMapPath(payload map[string]any, path ...string) (any, bool) {
	var current any = payload
	for _, segment := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func floatFromExaJSONValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		floatValue, err := typed.Float64()
		return floatValue, err == nil
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	}
	return 0, false
}
