package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TavilyDefaultMonthlyLimitCredits = 1000

type TavilyKeyUsage struct {
	Id                  int    `json:"id"`
	ChannelId           int    `json:"channel_id" gorm:"uniqueIndex:idx_tavily_channel_key"`
	KeyIndex            int    `json:"key_index" gorm:"uniqueIndex:idx_tavily_channel_key"`
	KeyFingerprint      string `json:"key_fingerprint" gorm:"type:varchar(64);index"`
	KeyTail             string `json:"key_tail" gorm:"type:varchar(16)"`
	ProjectId           string `json:"project_id" gorm:"type:varchar(128)"`
	MonthlyLimitCredits int    `json:"monthly_limit_credits" gorm:"default:1000"`
	UsedCredits         int    `json:"used_credits" gorm:"default:0"`
	ResetAt             int64  `json:"reset_at" gorm:"bigint;default:0"`
	LastSyncAt          int64  `json:"last_sync_at" gorm:"bigint;default:0"`
	LastError           string `json:"last_error" gorm:"type:text"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt           int64  `json:"updated_at" gorm:"bigint"`
}

func FingerprintTavilyKey(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

func TavilyKeyTail(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 8 {
		return key
	}
	return key[len(key)-8:]
}

func nextTavilyMonthlyReset(now time.Time) int64 {
	location := now.Location()
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, location)
	return nextMonth.Unix()
}

func GetOrCreateTavilyKeyUsage(channelId, keyIndex int, key string) (*TavilyKeyUsage, error) {
	now := common.GetTimestamp()
	usage := &TavilyKeyUsage{
		ChannelId:           channelId,
		KeyIndex:            keyIndex,
		KeyFingerprint:      FingerprintTavilyKey(key),
		KeyTail:             TavilyKeyTail(key),
		MonthlyLimitCredits: TavilyDefaultMonthlyLimitCredits,
		ResetAt:             nextTavilyMonthlyReset(time.Now()),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "key_index"}},
		DoNothing: true,
	}).Create(usage).Error
	if err != nil {
		return nil, err
	}
	err = DB.Where("channel_id = ? AND key_index = ?", channelId, keyIndex).First(usage).Error
	if err != nil {
		return nil, err
	}
	if usage.MonthlyLimitCredits <= 0 {
		usage.MonthlyLimitCredits = TavilyDefaultMonthlyLimitCredits
	}
	if usage.ResetAt <= 0 {
		usage.ResetAt = nextTavilyMonthlyReset(time.Now())
		usage.UpdatedAt = now
		_ = DB.Save(usage).Error
	}
	if usage.ResetAt > 0 && usage.ResetAt <= now {
		usage.UsedCredits = 0
		usage.LastError = ""
		usage.ResetAt = nextTavilyMonthlyReset(time.Now())
		usage.UpdatedAt = now
		_ = DB.Save(usage).Error
	}
	if usage.KeyFingerprint != FingerprintTavilyKey(key) || usage.KeyTail != TavilyKeyTail(key) {
		usage.KeyFingerprint = FingerprintTavilyKey(key)
		usage.KeyTail = TavilyKeyTail(key)
		usage.UsedCredits = 0
		usage.LastError = ""
		usage.ResetAt = nextTavilyMonthlyReset(time.Now())
		usage.UpdatedAt = now
		_ = DB.Save(usage).Error
	}
	return usage, nil
}

func AddTavilyKeyUsedCredits(channelId, keyIndex, credits int) error {
	if credits <= 0 {
		return nil
	}
	return DB.Model(&TavilyKeyUsage{}).
		Where("channel_id = ? AND key_index = ?", channelId, keyIndex).
		Updates(map[string]interface{}{
			"used_credits": gorm.Expr("used_credits + ?", credits),
			"last_error":   "",
			"updated_at":   common.GetTimestamp(),
		}).Error
}

func SetTavilyKeyLastError(channelId, keyIndex int, message string) error {
	return DB.Model(&TavilyKeyUsage{}).
		Where("channel_id = ? AND key_index = ?", channelId, keyIndex).
		Updates(map[string]interface{}{
			"last_error": message,
			"updated_at": common.GetTimestamp(),
		}).Error
}

func UpdateTavilyKeyUsageSettings(channelId, keyIndex int, monthlyLimitCredits *int, projectId *string) error {
	updates := map[string]interface{}{
		"updated_at": common.GetTimestamp(),
	}
	if monthlyLimitCredits != nil {
		updates["monthly_limit_credits"] = *monthlyLimitCredits
	}
	if projectId != nil {
		updates["project_id"] = strings.TrimSpace(*projectId)
	}
	return DB.Model(&TavilyKeyUsage{}).
		Where("channel_id = ? AND key_index = ?", channelId, keyIndex).
		Updates(updates).Error
}

func SyncTavilyKeyUsageCredits(channelId, keyIndex int, usedCredits *int, monthlyLimitCredits *int) error {
	updates := map[string]interface{}{
		"last_sync_at": common.GetTimestamp(),
		"last_error":   "",
		"updated_at":   common.GetTimestamp(),
	}
	if usedCredits != nil {
		updates["used_credits"] = *usedCredits
	}
	if monthlyLimitCredits != nil {
		updates["monthly_limit_credits"] = *monthlyLimitCredits
	}
	return DB.Model(&TavilyKeyUsage{}).
		Where("channel_id = ? AND key_index = ?", channelId, keyIndex).
		Updates(updates).Error
}

func ResetTavilyKeyUsageCredits(channelId int, keyIndex *int) error {
	query := DB.Model(&TavilyKeyUsage{}).Where("channel_id = ?", channelId)
	if keyIndex != nil {
		query = query.Where("key_index = ?", *keyIndex)
	}
	return query.Updates(map[string]interface{}{
		"used_credits": 0,
		"last_error":   "",
		"reset_at":     nextTavilyMonthlyReset(time.Now()),
		"updated_at":   common.GetTimestamp(),
	}).Error
}

func ListTavilyKeyUsages(channelId int) ([]TavilyKeyUsage, error) {
	usages := make([]TavilyKeyUsage, 0)
	err := DB.Where("channel_id = ?", channelId).Order("key_index asc").Find(&usages).Error
	return usages, err
}
