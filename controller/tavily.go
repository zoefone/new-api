package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type tavilyUsageResetRequest struct {
	KeyIndex *int `json:"key_index"`
}

type tavilyUsageUpdateRequest struct {
	KeyIndex            int     `json:"key_index"`
	MonthlyLimitCredits *int    `json:"monthly_limit_credits"`
	ProjectId           *string `json:"project_id"`
}

type tavilyUsageSyncRequest struct {
	KeyIndex *int `json:"key_index"`
}

func GetTavilyChannelUsage(c *gin.Context) {
	channelId, channel, ok := getTavilyUsageChannel(c)
	if !ok {
		return
	}
	respondTavilyChannelUsage(c, channelId, channel)
}

func UpdateTavilyChannelUsageSettings(c *gin.Context) {
	channelId, channel, ok := getTavilyUsageChannel(c)
	if !ok {
		return
	}

	req := tavilyUsageUpdateRequest{}
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
	if _, err := model.GetOrCreateTavilyKeyUsage(channelId, req.KeyIndex, keys[req.KeyIndex]); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateTavilyKeyUsageSettings(channelId, req.KeyIndex, req.MonthlyLimitCredits, req.ProjectId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	respondTavilyChannelUsage(c, channelId, channel)
}

func SyncTavilyChannelUsage(c *gin.Context) {
	channelId, channel, ok := getTavilyUsageChannel(c)
	if !ok {
		return
	}

	req := tavilyUsageSyncRequest{}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	keys := channel.GetKeys()
	indexes, err := tavilyUsageTargetIndexes(keys, req.KeyIndex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	results := make([]gin.H, 0, len(indexes))
	allOK := true
	for _, index := range indexes {
		key := keys[index]
		if _, err := model.GetOrCreateTavilyKeyUsage(channelId, index, key); err != nil {
			allOK = false
			results = append(results, gin.H{"key_index": index, "success": false, "message": err.Error()})
			continue
		}
		result := syncOneTavilyKeyUsage(c, channel, channelId, index, key)
		if success, _ := result["success"].(bool); !success {
			allOK = false
		}
		results = append(results, result)
	}
	respondTavilyChannelUsage(c, channelId, channel, gin.H{
		"sync":    results,
		"success": allOK,
	})
}

func ResetTavilyChannelUsage(c *gin.Context) {
	channelId, channel, ok := getTavilyUsageChannel(c)
	if !ok {
		return
	}

	req := tavilyUsageResetRequest{}
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
		if _, err := model.GetOrCreateTavilyKeyUsage(channelId, *req.KeyIndex, keys[*req.KeyIndex]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
			return
		}
	} else {
		for index, key := range keys {
			if _, err := model.GetOrCreateTavilyKeyUsage(channelId, index, key); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
				return
			}
		}
	}

	if err := model.ResetTavilyKeyUsageCredits(channelId, req.KeyIndex); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	respondTavilyChannelUsage(c, channelId, channel)
}

func getTavilyUsageChannel(c *gin.Context) (int, *model.Channel, bool) {
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
	if channel.Type != constant.ChannelTypeTavily {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel is not Tavily"})
		return 0, nil, false
	}
	return channelId, channel, true
}

func respondTavilyChannelUsage(c *gin.Context, channelId int, channel *model.Channel, extras ...gin.H) {
	usages, err := model.ListTavilyKeyUsages(channelId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	statusMap := channel.ChannelInfo.MultiKeyStatusList
	keys := channel.GetKeys()
	usageByIndex := make(map[int]model.TavilyKeyUsage, len(usages))
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
			usage = model.TavilyKeyUsage{
				ChannelId:           channelId,
				KeyIndex:            index,
				KeyFingerprint:      model.FingerprintTavilyKey(key),
				KeyTail:             model.TavilyKeyTail(key),
				MonthlyLimitCredits: model.TavilyDefaultMonthlyLimitCredits,
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

func tavilyUsageTargetIndexes(keys []string, keyIndex *int) ([]int, error) {
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

func syncOneTavilyKeyUsage(c *gin.Context, channel *model.Channel, channelId int, keyIndex int, key string) gin.H {
	statusCode, payload, err := fetchTavilyUsage(c, channel, key)
	if err != nil {
		_ = model.SetTavilyKeyLastError(channelId, keyIndex, err.Error())
		return gin.H{"key_index": keyIndex, "success": false, "message": err.Error()}
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		message := fmt.Sprintf("upstream status %d: %s", statusCode, truncateTavilyMessage(string(payload)))
		_ = model.SetTavilyKeyLastError(channelId, keyIndex, message)
		return gin.H{"key_index": keyIndex, "success": false, "upstream_status": statusCode, "message": message}
	}

	usedCredits, monthlyLimitCredits, upstreamData, err := parseTavilyUsagePayload(payload)
	if err != nil {
		_ = model.SetTavilyKeyLastError(channelId, keyIndex, err.Error())
		return gin.H{"key_index": keyIndex, "success": false, "upstream_status": statusCode, "message": err.Error()}
	}
	if usedCredits == nil && monthlyLimitCredits == nil {
		message := "upstream response does not include key usage or limit"
		_ = model.SyncTavilyKeyUsageCredits(channelId, keyIndex, nil, nil)
		_ = model.SetTavilyKeyLastError(channelId, keyIndex, message)
		return gin.H{"key_index": keyIndex, "success": false, "upstream_status": statusCode, "message": message, "upstream_data": upstreamData}
	}
	if err := model.SyncTavilyKeyUsageCredits(channelId, keyIndex, usedCredits, monthlyLimitCredits); err != nil {
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

func fetchTavilyUsage(c *gin.Context, channel *model.Channel, key string) (int, []byte, error) {
	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return 0, nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	baseURL := strings.TrimRight(channel.GetBaseURL(), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(constant.ChannelBaseURLs[constant.ChannelTypeTavily], "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/usage", nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
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

func parseTavilyUsagePayload(body []byte) (*int, *int, map[string]any, error) {
	payload := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid Tavily usage response: %w", err)
	}
	usedCredits := firstIntPointerFromMap(payload, []string{"key", "usage"}, []string{"usage"}, []string{"used_credits"}, []string{"credits_used"})
	monthlyLimitCredits := firstIntPointerFromMap(payload, []string{"key", "limit"}, []string{"limit"}, []string{"monthly_limit_credits"}, []string{"credit_limit"})
	return usedCredits, monthlyLimitCredits, payload, nil
}

func firstIntPointerFromMap(payload map[string]any, paths ...[]string) *int {
	for _, path := range paths {
		if value, ok := valueFromPath(payload, path...); ok {
			if intValue, ok := intFromTavilyJSONValue(value); ok {
				return &intValue
			}
		}
	}
	return nil
}

func valueFromPath(payload map[string]any, path ...string) (any, bool) {
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

func intFromTavilyJSONValue(value any) (int, bool) {
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

func truncateTavilyMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 1000 {
		return message
	}
	return message[:1000]
}
