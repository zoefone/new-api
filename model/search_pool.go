package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SearchPoolProviderTavily = "tavily"
	SearchPoolProviderExa    = "exa"

	SearchPoolDefaultTag         = "search-pool"
	SearchPoolDefaultGroup       = "default"
	SearchPoolDefaultChannelName = "Search Pool"
)

type SearchPoolAccount struct {
	Id             int    `json:"id"`
	Provider       string `json:"provider" gorm:"type:varchar(32);uniqueIndex:idx_search_pool_provider_key"`
	Name           string `json:"name" gorm:"type:varchar(128);index"`
	ApiKey         string `json:"-" gorm:"type:text"`
	KeyFingerprint string `json:"key_fingerprint" gorm:"type:varchar(64);uniqueIndex:idx_search_pool_provider_key"`
	KeyTail        string `json:"key_tail" gorm:"type:varchar(16)"`
	ApiKeyId       string `json:"api_key_id" gorm:"type:varchar(128)"`
	ProjectId      string `json:"project_id" gorm:"type:varchar(128)"`
	MonthlyLimit   int    `json:"monthly_limit" gorm:"default:1000"`
	BaseURL        string `json:"base_url" gorm:"type:varchar(255)"`
	Proxy          string `json:"proxy" gorm:"type:varchar(255)"`
	PaidUntil      int64  `json:"paid_until" gorm:"bigint;default:0"`
	Remark         string `json:"remark" gorm:"type:text"`
	Enabled        bool   `json:"enabled" gorm:"default:true"`
	Status         int    `json:"status" gorm:"default:1"`
	ChannelId      int    `json:"channel_id" gorm:"index;default:0"`
	KeyIndex       int    `json:"key_index" gorm:"default:-1"`
	LastError      string `json:"last_error" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

type SearchPoolSummary struct {
	Provider        string `json:"provider"`
	Total           int64  `json:"total"`
	Enabled         int64  `json:"enabled"`
	Disabled        int64  `json:"disabled"`
	Linked          int64  `json:"linked"`
	MonthlyCapacity int64  `json:"monthly_capacity"`
}

func NormalizeSearchPoolProvider(provider string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case SearchPoolProviderTavily:
		return SearchPoolProviderTavily, true
	case SearchPoolProviderExa:
		return SearchPoolProviderExa, true
	default:
		return "", false
	}
}

func SearchPoolChannelType(provider string) (int, bool) {
	switch provider {
	case SearchPoolProviderTavily:
		return constant.ChannelTypeTavily, true
	case SearchPoolProviderExa:
		return constant.ChannelTypeExa, true
	default:
		return 0, false
	}
}

func SearchPoolProviderModels(provider string) string {
	switch provider {
	case SearchPoolProviderTavily:
		return "tavily-search,tavily-extract"
	case SearchPoolProviderExa:
		return "exa-search,exa-contents"
	default:
		return ""
	}
}

func SearchPoolProviderDefaultBaseURL(provider string) string {
	channelType, ok := SearchPoolChannelType(provider)
	if !ok || channelType < 0 || channelType >= len(constant.ChannelBaseURLs) {
		return ""
	}
	return constant.ChannelBaseURLs[channelType]
}

func SearchPoolProviderDisplayName(provider string) string {
	switch provider {
	case SearchPoolProviderTavily:
		return "Tavily"
	case SearchPoolProviderExa:
		return "Exa"
	default:
		return provider
	}
}

func FingerprintSearchPoolKey(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

func SearchPoolKeyTail(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 8 {
		return key
	}
	return key[len(key)-8:]
}

func NormalizeSearchPoolAccount(account *SearchPoolAccount) error {
	if account == nil {
		return errors.New("account is nil")
	}
	provider, ok := NormalizeSearchPoolProvider(account.Provider)
	if !ok {
		return errors.New("provider must be tavily or exa")
	}
	key := strings.TrimSpace(account.ApiKey)
	if key == "" {
		return errors.New("api_key is required")
	}
	account.Provider = provider
	account.ApiKey = key
	account.Name = strings.TrimSpace(account.Name)
	if account.Name == "" {
		account.Name = SearchPoolProviderDisplayName(provider) + " " + SearchPoolKeyTail(key)
	}
	account.KeyFingerprint = FingerprintSearchPoolKey(key)
	account.KeyTail = SearchPoolKeyTail(key)
	account.ApiKeyId = strings.TrimSpace(account.ApiKeyId)
	account.ProjectId = strings.TrimSpace(account.ProjectId)
	account.BaseURL = strings.TrimRight(strings.TrimSpace(account.BaseURL), "/")
	account.Proxy = strings.TrimSpace(account.Proxy)
	account.Remark = strings.TrimSpace(account.Remark)
	if account.MonthlyLimit <= 0 {
		switch provider {
		case SearchPoolProviderTavily:
			account.MonthlyLimit = TavilyDefaultMonthlyLimitCredits
		case SearchPoolProviderExa:
			account.MonthlyLimit = ExaDefaultMonthlyLimitCredits
		default:
			account.MonthlyLimit = 1000
		}
	}
	if account.Status == 0 {
		account.Status = common.ChannelStatusEnabled
	}
	return nil
}

func UpsertSearchPoolAccount(account *SearchPoolAccount, replace bool) (*SearchPoolAccount, bool, error) {
	if err := NormalizeSearchPoolAccount(account); err != nil {
		return nil, false, err
	}
	now := common.GetTimestamp()
	existing := &SearchPoolAccount{}
	err := DB.Where("provider = ? AND key_fingerprint = ?", account.Provider, account.KeyFingerprint).First(existing).Error
	if err == nil {
		if !replace {
			return existing, false, nil
		}
		updates := map[string]interface{}{
			"name":          account.Name,
			"api_key":       account.ApiKey,
			"key_tail":      account.KeyTail,
			"api_key_id":    account.ApiKeyId,
			"project_id":    account.ProjectId,
			"monthly_limit": account.MonthlyLimit,
			"base_url":      account.BaseURL,
			"proxy":         account.Proxy,
			"paid_until":    account.PaidUntil,
			"remark":        account.Remark,
			"enabled":       account.Enabled,
			"status":        account.Status,
			"updated_at":    now,
		}
		if err := DB.Model(existing).Updates(updates).Error; err != nil {
			return nil, false, err
		}
		if err := DB.First(existing, existing.Id).Error; err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	account.CreatedAt = now
	account.UpdatedAt = now
	if account.ChannelId <= 0 && account.KeyIndex == 0 {
		account.KeyIndex = -1
	}
	if err := DB.Create(account).Error; err != nil {
		return nil, false, err
	}
	return account, true, nil
}

func ListSearchPoolAccounts(provider string, enabled *bool) ([]SearchPoolAccount, error) {
	query := DB.Model(&SearchPoolAccount{})
	if normalized, ok := NormalizeSearchPoolProvider(provider); ok {
		query = query.Where("provider = ?", normalized)
	}
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	accounts := make([]SearchPoolAccount, 0)
	err := query.Order("provider asc, id asc").Find(&accounts).Error
	return accounts, err
}

func ListEnabledSearchPoolAccountsByProvider(provider string) ([]SearchPoolAccount, error) {
	normalized, ok := NormalizeSearchPoolProvider(provider)
	if !ok {
		return nil, errors.New("provider must be tavily or exa")
	}
	accounts := make([]SearchPoolAccount, 0)
	err := DB.Where("provider = ? AND enabled = ?", normalized, true).Order("id asc").Find(&accounts).Error
	return accounts, err
}

func GetSearchPoolAccountById(id int) (*SearchPoolAccount, error) {
	account := &SearchPoolAccount{Id: id}
	err := DB.First(account, "id = ?", id).Error
	return account, err
}

func UpdateSearchPoolAccount(account *SearchPoolAccount) error {
	if account == nil || account.Id == 0 {
		return errors.New("invalid account")
	}
	account.UpdatedAt = common.GetTimestamp()
	return DB.Model(account).Select(
		"name", "api_key_id", "project_id", "monthly_limit", "base_url", "proxy",
		"paid_until", "remark", "enabled", "status", "last_error", "updated_at",
	).Updates(account).Error
}

func DeleteSearchPoolAccount(id int) error {
	return DB.Delete(&SearchPoolAccount{}, id).Error
}

func UpdateSearchPoolAccountLink(id int, channelId int, keyIndex int) error {
	return DB.Model(&SearchPoolAccount{}).Where("id = ?", id).Updates(map[string]interface{}{
		"channel_id": channelId,
		"key_index":  keyIndex,
		"last_error": "",
		"updated_at": common.GetTimestamp(),
	}).Error
}

func SetSearchPoolAccountLastError(id int, message string) error {
	return DB.Model(&SearchPoolAccount{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_error": message,
		"updated_at": common.GetTimestamp(),
	}).Error
}

func GetSearchPoolAccountByChannelKeyIndex(provider string, channelId int, keyIndex int) (*SearchPoolAccount, error) {
	normalized, ok := NormalizeSearchPoolProvider(provider)
	if !ok {
		return nil, errors.New("provider must be tavily or exa")
	}
	account := &SearchPoolAccount{}
	err := DB.Where("provider = ? AND channel_id = ? AND key_index = ? AND enabled = ?", normalized, channelId, keyIndex, true).
		First(account).Error
	return account, err
}

func FindSearchPoolChannel(provider string, tag string) (*Channel, error) {
	channelType, ok := SearchPoolChannelType(provider)
	if !ok {
		return nil, errors.New("provider must be tavily or exa")
	}
	if tag == "" {
		tag = SearchPoolDefaultTag
	}
	channel := &Channel{}
	err := DB.Where("type = ? AND tag = ?", channelType, tag).Order("id asc").First(channel).Error
	return channel, err
}

func ListSearchPoolSummaries() ([]SearchPoolSummary, error) {
	accounts := make([]SearchPoolAccount, 0)
	if err := DB.Find(&accounts).Error; err != nil {
		return nil, err
	}
	byProvider := map[string]*SearchPoolSummary{}
	for _, provider := range []string{SearchPoolProviderTavily, SearchPoolProviderExa} {
		byProvider[provider] = &SearchPoolSummary{Provider: provider}
	}
	for _, account := range accounts {
		provider, ok := NormalizeSearchPoolProvider(account.Provider)
		if !ok {
			continue
		}
		summary := byProvider[provider]
		summary.Total++
		if account.Enabled {
			summary.Enabled++
			summary.MonthlyCapacity += int64(account.MonthlyLimit)
		} else {
			summary.Disabled++
		}
		if account.ChannelId > 0 && account.KeyIndex >= 0 {
			summary.Linked++
		}
	}
	providers := make([]string, 0, len(byProvider))
	for provider := range byProvider {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	result := make([]SearchPoolSummary, 0, len(providers))
	for _, provider := range providers {
		result = append(result, *byProvider[provider])
	}
	return result, nil
}

func BulkClearSearchPoolLinks(provider string, channelId int) error {
	query := DB.Model(&SearchPoolAccount{})
	if normalized, ok := NormalizeSearchPoolProvider(provider); ok {
		query = query.Where("provider = ?", normalized)
	}
	if channelId > 0 {
		query = query.Where("channel_id = ?", channelId)
	}
	return query.Updates(map[string]interface{}{
		"channel_id": 0,
		"key_index":  -1,
		"updated_at": common.GetTimestamp(),
	}).Error
}

func InsertSearchPoolAccountIgnore(account *SearchPoolAccount) error {
	if err := NormalizeSearchPoolAccount(account); err != nil {
		return err
	}
	now := common.GetTimestamp()
	account.CreatedAt = now
	account.UpdatedAt = now
	if account.ChannelId <= 0 && account.KeyIndex == 0 {
		account.KeyIndex = -1
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(account).Error
}
