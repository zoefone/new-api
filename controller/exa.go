package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type exaUsageResetRequest struct {
	KeyIndex *int `json:"key_index"`
}

type exaUsageUpdateRequest struct {
	KeyIndex            int     `json:"key_index"`
	MonthlyLimitCredits *int    `json:"monthly_limit_credits"`
	ProjectId           *string `json:"project_id"`
}

type exaUsageSyncRequest struct {
	KeyIndex *int `json:"key_index"`
}

func GetExaChannelUsage(c *gin.Context) {
	channelId, channel, ok := getExaUsageChannel(c)
	if !ok {
		return
	}
	respondExaChannelUsage(c, channelId, channel)
}

func UpdateExaChannelUsageSettings(c *gin.Context) {
	channelId, channel, ok := getExaUsageChannel(c)
	if !ok {
		return
	}

	req := exaUsageUpdateRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	keys := channel.GetKeys()
	if req.KeyIndex < 0 || req.KeyIndex >= len(keys) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid key index"})
		return
	}
	if req.MonthlyLimitCredits != nil && *req.MonthlyLimitCredits <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "monthly_limit_credits must be greater than 0"})
		return
	}
	if _, err := model.GetOrCreateExaKeyUsage(channelId, req.KeyIndex, keys[req.KeyIndex]); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateExaKeyUsageSettings(channelId, req.KeyIndex, req.MonthlyLimitCredits, req.ProjectId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	respondExaChannelUsage(c, channelId, channel)
}

func SyncExaChannelUsage(c *gin.Context) {
	channelId, channel, ok := getExaUsageChannel(c)
	if !ok {
		return
	}

	req := exaUsageSyncRequest{}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	keys := channel.GetKeys()
	indexes, err := exaUsageTargetIndexes(keys, req.KeyIndex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	results := make([]gin.H, 0, len(indexes))
	allOK := true
	for _, index := range indexes {
		key := keys[index]
		if _, err := model.GetOrCreateExaKeyUsage(channelId, index, key); err != nil {
			allOK = false
			results = append(results, gin.H{"key_index": index, "success": false, "message": err.Error()})
			continue
		}
		result := syncOneExaKeyUsage(c, channel, channelId, index, key)
		if success, _ := result["success"].(bool); !success {
			allOK = false
		}
		results = append(results, result)
	}
	respondExaChannelUsage(c, channelId, channel, gin.H{
		"sync":    results,
		"success": allOK,
	})
}

func ResetExaChannelUsage(c *gin.Context) {
	channelId, channel, ok := getExaUsageChannel(c)
	if !ok {
		return
	}

	req := exaUsageResetRequest{}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}

	keys := channel.GetKeys()
	if req.KeyIndex != nil {
		if *req.KeyIndex < 0 || *req.KeyIndex >= len(keys) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid key index"})
			return
		}
		if _, err := model.GetOrCreateExaKeyUsage(channelId, *req.KeyIndex, keys[*req.KeyIndex]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
			return
		}
	} else {
		for index, key := range keys {
			if _, err := model.GetOrCreateExaKeyUsage(channelId, index, key); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
				return
			}
		}
	}

	if err := model.ResetExaKeyUsageCredits(channelId, req.KeyIndex); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	respondExaChannelUsage(c, channelId, channel)
}

func getExaUsageChannel(c *gin.Context) (int, *model.Channel, bool) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return 0, nil, false
	}
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "channel not found"})
		return 0, nil, false
	}
	if channel.Type != constant.ChannelTypeExa {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel is not Exa"})
		return 0, nil, false
	}
	return channelId, channel, true
}

func respondExaChannelUsage(c *gin.Context, channelId int, channel *model.Channel, extras ...gin.H) {
	usages, err := model.ListExaKeyUsages(channelId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	statusMap := channel.ChannelInfo.MultiKeyStatusList
	keys := channel.GetKeys()
	usageByIndex := make(map[int]model.ExaKeyUsage, len(usages))
	for _, usage := range usages {
		usageByIndex[usage.KeyIndex] = usage
	}

	items := make([]gin.H, 0, len(keys))
	for index, key := range keys {
		status := common.ChannelStatusEnabled
		if statusMap != nil {
			if value, ok := statusMap[index]; ok {
				status = value
			}
		}
		usage, ok := usageByIndex[index]
		if !ok {
			usage = model.ExaKeyUsage{
				ChannelId:           channelId,
				KeyIndex:            index,
				KeyFingerprint:      model.FingerprintExaKey(key),
				KeyTail:             model.ExaKeyTail(key),
				MonthlyLimitCredits: model.ExaDefaultMonthlyLimitCredits,
			}
		}
		items = append(items, gin.H{
			"key_index":             index,
			"key_tail":              usage.KeyTail,
			"key_fingerprint":       usage.KeyFingerprint,
			"project_id":            usage.ProjectId,
			"monthly_limit_credits": usage.MonthlyLimitCredits,
			"used_credits":          usage.UsedCredits,
			"remaining_credits":     usage.MonthlyLimitCredits - usage.UsedCredits,
			"reset_at":              usage.ResetAt,
			"last_sync_at":          usage.LastSyncAt,
			"last_error":            usage.LastError,
			"status":                status,
		})
	}

	resp := gin.H{"success": true, "data": items}
	for _, extra := range extras {
		for key, value := range extra {
			resp[key] = value
		}
	}
	c.JSON(http.StatusOK, resp)
}

func exaUsageTargetIndexes(keys []string, keyIndex *int) ([]int, error) {
	if keyIndex != nil {
		if *keyIndex < 0 || *keyIndex >= len(keys) {
			return nil, fmt.Errorf("invalid key index")
		}
		return []int{*keyIndex}, nil
	}
	indexes := make([]int, 0, len(keys))
	for index := range keys {
		indexes = append(indexes, index)
	}
	return indexes, nil
}

func syncOneExaKeyUsage(c *gin.Context, channel *model.Channel, channelId int, keyIndex int, key string) gin.H {
	usage, err := model.GetOrCreateExaKeyUsage(channelId, keyIndex, key)
	if err != nil {
		_ = model.SetExaKeyLastError(channelId, keyIndex, err.Error())
		return gin.H{"key_index": keyIndex, "success": false, "message": err.Error()}
	}
	apiKeyID := strings.TrimSpace(usage.ProjectId)
	if apiKeyID == "" {
		message := "Exa API key ID is required for usage sync; fill the Project/API Key ID field first"
		_ = model.SetExaKeyLastError(channelId, keyIndex, message)
		return gin.H{"key_index": keyIndex, "success": false, "message": message}
	}

	statusCode, payload, err := fetchExaUsage(c, channel, key, apiKeyID)
	if err != nil {
		_ = model.SetExaKeyLastError(channelId, keyIndex, err.Error())
		return gin.H{"key_index": keyIndex, "success": false, "message": err.Error()}
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		message := fmt.Sprintf("upstream status %d: %s", statusCode, truncateExaMessage(string(payload)))
		_ = model.SetExaKeyLastError(channelId, keyIndex, message)
		return gin.H{"key_index": keyIndex, "success": false, "upstream_status": statusCode, "message": message}
	}

	usedCredits, monthlyLimitCredits, upstreamData, err := parseExaUsagePayload(payload)
	if err != nil {
		_ = model.SetExaKeyLastError(channelId, keyIndex, err.Error())
		return gin.H{"key_index": keyIndex, "success": false, "upstream_status": statusCode, "message": err.Error()}
	}
	if usedCredits == nil && monthlyLimitCredits == nil {
		message := "upstream response does not include key usage or limit"
		_ = model.SyncExaKeyUsageCredits(channelId, keyIndex, nil, nil)
		_ = model.SetExaKeyLastError(channelId, keyIndex, message)
		return gin.H{"key_index": keyIndex, "success": false, "upstream_status": statusCode, "message": message, "upstream_data": upstreamData}
	}
	if err := model.SyncExaKeyUsageCredits(channelId, keyIndex, usedCredits, monthlyLimitCredits); err != nil {
		return gin.H{"key_index": keyIndex, "success": false, "upstream_status": statusCode, "message": err.Error()}
	}
	result := gin.H{"key_index": keyIndex, "success": true, "upstream_status": statusCode, "upstream_data": upstreamData}
	if usedCredits != nil {
		result["used_credits"] = *usedCredits
	}
	if monthlyLimitCredits != nil {
		result["monthly_limit_credits"] = *monthlyLimitCredits
	}
	return result
}

func fetchExaUsage(c *gin.Context, channel *model.Channel, serviceKey string, apiKeyID string) (int, []byte, error) {
	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return 0, nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	baseURL := exaAdminBaseURL(channel)
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	query := url.Values{}
	query.Set("start_date", start.Format(time.RFC3339))
	query.Set("end_date", now.Format(time.RFC3339))
	targetURL := fmt.Sprintf("%s/api-keys/%s/usage?%s", baseURL, url.PathEscape(apiKeyID), query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("x-api-key", strings.TrimSpace(serviceKey))
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func exaAdminBaseURL(channel *model.Channel) string {
	const defaultExaAdminBaseURL = "https://admin-api.exa.ai/team-management"
	if channel == nil {
		return defaultExaAdminBaseURL
	}
	otherInfo := channel.GetOtherInfo()
	for _, key := range []string{"exa_admin_base_url", "admin_base_url"} {
		if value, ok := otherInfo[key].(string); ok {
			if value = strings.TrimRight(strings.TrimSpace(value), "/"); value != "" {
				return value
			}
		}
	}
	return defaultExaAdminBaseURL
}

func parseExaUsagePayload(body []byte) (*int, *int, map[string]any, error) {
	payload := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid Exa usage response: %w", err)
	}
	usedCredits := firstExaIntPointerFromMap(payload,
		[]string{"usage"},
		[]string{"used_credits"},
		[]string{"used_requests"},
		[]string{"request_count"},
		[]string{"requests"},
	)
	if usedCredits == nil {
		usedCredits = exaUsedRequestsFromCostBreakdown(payload)
	}
	monthlyLimitCredits := firstExaIntPointerFromMap(payload,
		[]string{"limit"},
		[]string{"monthly_limit_credits"},
		[]string{"monthly_limit_requests"},
		[]string{"request_limit"},
		[]string{"credit_limit"},
	)
	return usedCredits, monthlyLimitCredits, payload, nil
}

func exaUsedRequestsFromCostBreakdown(payload map[string]any) *int {
	breakdown, ok := payload["cost_breakdown"].([]any)
	if !ok {
		breakdown, ok = payload["costBreakdown"].([]any)
	}
	if !ok {
		return nil
	}
	total := 0
	for _, item := range breakdown {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		quantity, ok := object["quantity"]
		if !ok {
			continue
		}
		if value, ok := exaQuantityToInt(quantity); ok {
			total += value
		}
	}
	return &total
}

func exaQuantityToInt(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		if int64Value, err := typed.Int64(); err == nil {
			return int(int64Value), true
		}
		if floatValue, err := typed.Float64(); err == nil {
			return int(math.Ceil(floatValue)), true
		}
	case float64:
		return int(math.Ceil(typed)), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return int(math.Ceil(parsed)), true
		}
	}
	return 0, false
}

func firstExaIntPointerFromMap(payload map[string]any, paths ...[]string) *int {
	for _, path := range paths {
		if value, ok := valueFromExaPath(payload, path...); ok {
			if intValue, ok := intFromExaJSONValue(value); ok {
				return &intValue
			}
		}
	}
	return nil
}

func valueFromExaPath(payload map[string]any, path ...string) (any, bool) {
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

func intFromExaJSONValue(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		if int64Value, err := typed.Int64(); err == nil {
			return int(int64Value), true
		}
		if floatValue, err := typed.Float64(); err == nil {
			return int(floatValue), true
		}
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	}
	return 0, false
}

func truncateExaMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 1000 {
		return message
	}
	return message[:1000]
}
