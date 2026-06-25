package controller

import (
	"io"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type tavilyUsageResetRequest struct {
	KeyIndex *int `json:"key_index"`
}

func GetTavilyChannelUsage(c *gin.Context) {
	channelId, channel, ok := getTavilyUsageChannel(c)
	if !ok {
		return
	}
	respondTavilyChannelUsage(c, channelId, channel)
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

func respondTavilyChannelUsage(c *gin.Context, channelId int, channel *model.Channel) {
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

	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}
