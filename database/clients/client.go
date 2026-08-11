package clients

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/tasks"
	"github.com/komari-monitor/komari/database/trafficstats"
	"github.com/komari-monitor/komari/utils"
	"gorm.io/gorm"

	"github.com/google/uuid"
)

func DeleteClient(clientUuid string) error {
	db := dbcore.GetDBInstance()
	err := db.Delete(&models.Client{}, "uuid = ?", clientUuid).Error
	if err != nil {
		return err
	}
	trafficstats.Invalidate(clientUuid)
	NotifyChanged()
	return nil
}

func SaveClientInfo(update map[string]interface{}) error {
	db := dbcore.GetDBInstance()
	clientUUID, ok := update["uuid"].(string)
	if !ok || clientUUID == "" {
		return fmt.Errorf("invalid client UUID")
	}

	// 确保更新的字段不为空
	if len(update) == 0 {
		return fmt.Errorf("no fields to update")
	}

	update["updated_at"] = time.Now()

	toFloat64 := func(value interface{}) (float64, bool) {
		switch typed := value.(type) {
		case float64:
			return typed, true
		case float32:
			return float64(typed), true
		case int:
			return float64(typed), true
		case int8:
			return float64(typed), true
		case int16:
			return float64(typed), true
		case int32:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case uint:
			return float64(typed), true
		case uint8:
			return float64(typed), true
		case uint16:
			return float64(typed), true
		case uint32:
			return float64(typed), true
		case uint64:
			return float64(typed), true
		case json.Number:
			parsed, err := typed.Float64()
			if err != nil {
				return 0, false
			}
			return parsed, true
		default:
			return 0, false
		}
	}

	checkOptionalInt := func(name, key string, maxValue float64) error {
		value, exists := update[key]
		if !exists || value == nil {
			return nil
		}

		numericValue, ok := toFloat64(value)
		if !ok {
			return fmt.Errorf("%s must be a valid number", name)
		}
		if numericValue < 0 || numericValue > maxValue {
			return fmt.Errorf("%s must be a valid non-negative number: %v", name, value)
		}
		return nil
	}

	verify := func(update map[string]interface{}) error {
		if err := checkOptionalInt("Cpu.Cores", "cpu_cores", math.MaxInt-1); err != nil {
			return err
		}
		if err := checkOptionalInt("Cpu.PhysicalCores", "cpu_physical_cores", math.MaxInt-1); err != nil {
			return err
		}
		if err := checkOptionalInt("Ram.Total", "mem_total", math.MaxInt64-1); err != nil {
			return err
		}
		if err := checkOptionalInt("Swap.Total", "swap_total", math.MaxInt64-1); err != nil {
			return err
		}
		if err := checkOptionalInt("Disk.Total", "disk_total", math.MaxInt64-1); err != nil {
			return err
		}
		return nil
	}

	if err := verify(update); err != nil {
		return err
	}

	err := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Updates(update).Error
	if err != nil {
		return err
	}
	NotifyChanged()
	return nil
}

func EditClientName(clientUUID, clientName string) error {
	db := dbcore.GetDBInstance()
	err := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Update("name", clientName).Error
	if err != nil {
		return err
	}
	NotifyChanged()
	return nil
}

func EditClientToken(clientUUID, token string) error {
	db := dbcore.GetDBInstance()
	err := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Update("token", token).Error
	if err != nil {
		return err
	}
	NotifyChanged()
	return nil
}

// CreateClient 创建新客户端
func CreateClient() (clientUUID, token string, err error) {
	db := dbcore.GetDBInstance()
	token = utils.GenerateToken()
	clientUUID = uuid.New().String()

	client := newClient(clientUUID, token, "client_"+clientUUID[0:8], time.Now())

	err = db.Create(&client).Error
	if err != nil {
		return "", "", err
	}
	if err := tasks.AddDefaultOnClientUUID(clientUUID); err != nil {
		log.Println("Failed to apply default-on ping tasks to new client:", err)
	}
	NotifyChanged()
	return clientUUID, token, nil
}

func CreateClientWithName(name string) (clientUUID, token string, err error) {
	if name == "" {
		return CreateClient()
	}
	db := dbcore.GetDBInstance()
	token = utils.GenerateToken()
	clientUUID = uuid.New().String()
	client := newClient(clientUUID, token, name, time.Now())

	err = db.Create(&client).Error
	if err != nil {
		return "", "", err
	}
	if err := tasks.AddDefaultOnClientUUID(clientUUID); err != nil {
		log.Println("Failed to apply default-on ping tasks to new client:", err)
	}
	NotifyChanged()
	return clientUUID, token, nil
}

func newClient(clientUUID, token, name string, now time.Time) models.Client {
	location := models.GetAppLocation()
	localNow := now.In(location)
	year, month, day := localNow.Date()

	return models.Client{
		UUID:        clientUUID,
		Token:       token,
		Name:        name,
		Price:       0,
		Currency:    "¥",
		ExpiredAt:   models.FromTime(time.Date(year, month, day, 0, 0, 0, 0, location)),
		AutoRenewal: true,
		CreatedAt:   models.FromTime(localNow),
		UpdatedAt:   models.FromTime(localNow),
	}
}

/*
// GetAllClients 获取所有客户端配置

	func getAllClients() (clients []models.Client, err error) {
		db := dbcore.GetDBInstance()
		err = db.Find(&clients).Error
		if err != nil {
			return nil, err
		}
		return clients, nil
	}
*/
func GetClientByUUID(uuid string) (client models.Client, err error) {
	db := dbcore.GetDBInstance()
	err = db.Where("uuid = ?", uuid).First(&client).Error
	if err != nil {
		return models.Client{}, err
	}
	populateCurrentTraffic(&client)
	return client, nil
}

// GetClientBasicInfo 获取指定 UUID 的客户端基本信息
func GetClientBasicInfo(uuid string) (client models.Client, err error) {
	db := dbcore.GetDBInstance()
	err = db.Where("uuid = ?", uuid).First(&client).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return models.Client{}, fmt.Errorf("客户端不存在: %s", uuid)
		}
		return models.Client{}, err
	}
	populateCurrentTraffic(&client)
	return client, nil
}

func GetClientTokenByUUID(uuid string) (token string, err error) {
	db := dbcore.GetDBInstance()
	var client models.Client
	err = db.Where("uuid = ?", uuid).First(&client).Error
	if err != nil {
		return "", err
	}
	return client.Token, nil
}

func GetAllClientBasicInfo() (clients []models.Client, err error) {
	db := dbcore.GetDBInstance()
	err = db.Find(&clients).Error
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func GetAllClientBasicInfoWithTraffic() (clients []models.Client, err error) {
	clients, err = GetAllClientBasicInfo()
	if err != nil {
		return nil, err
	}
	for index := range clients {
		populateCurrentTraffic(&clients[index])
	}
	return clients, nil
}

func populateCurrentTraffic(client *models.Client) {
	if client == nil || client.UUID == "" {
		return
	}
	usage, err := trafficstats.Current(*client, time.Now())
	if err != nil {
		return
	}
	client.TrafficUp = usage.Up
	client.TrafficDown = usage.Down
	client.TrafficUsed = usage.Used
	client.TrafficCycleStart = models.FromTime(usage.CycleStart)
}

func SaveClient(updates map[string]interface{}) error {
	db := dbcore.GetDBInstance()
	clientUUID, ok := updates["uuid"].(string)
	if !ok || clientUUID == "" {
		return fmt.Errorf("invalid client UUID")
	}

	// 确保更新的字段不为空
	if len(updates) == 0 {
		return fmt.Errorf("no fields to update")
	}

	if v, exists := updates["traffic_limit"]; exists {
		if val, ok := v.(float64); ok {
			if val < 0 || val > math.MaxInt64-1 {
				return fmt.Errorf("traffic_limit must be a valid non-negative int64 value, got %v", val)
			}
		}
	}
	if v, exists := updates["traffic_limit_type"]; exists {
		val, ok := v.(string)
		if !ok {
			return fmt.Errorf("traffic_limit_type must be a string")
		}
		switch val {
		case "sum", "max", "min", "up", "down":
		default:
			return fmt.Errorf("unsupported traffic_limit_type: %s", val)
		}
	}
	if v, exists := updates["traffic_multiplier"]; exists {
		val, ok := v.(float64)
		if !ok || math.IsNaN(val) || math.IsInf(val, 0) || val < 0 || val > 999999 {
			return fmt.Errorf("traffic_multiplier must be a finite number between 0 and 999999")
		}
	}
	if v, exists := updates["traffic_reset_day"]; exists {
		val, ok := numericFloat64(v)
		if !ok || math.Trunc(val) != val || val < 0 || val > 31 {
			return fmt.Errorf("traffic_reset_day must be an integer between 0 and 31")
		}
		updates["traffic_reset_day"] = int(val)
	}
	if v, exists := updates["traffic_initial"]; exists {
		val, ok := numericFloat64(v)
		if !ok || math.Trunc(val) != val || val < 0 || val >= float64(math.MaxInt64) {
			return fmt.Errorf("traffic_initial must be a valid non-negative int64 value")
		}
		updates["traffic_initial"] = int64(val)
		updates["traffic_initial_at"] = models.Now()
	} else {
		delete(updates, "traffic_initial_at")
	}
	if v, exists := updates["expired_at"]; exists {
		expiredAt, err := normalizeExpirationDate(v)
		if err != nil {
			return err
		}
		updates["expired_at"] = expiredAt
	}

	updates["updated_at"] = time.Now()

	err := db.Model(&models.Client{}).Where("uuid = ?", clientUUID).Updates(updates).Error
	if err != nil {
		return err
	}
	trafficstats.Invalidate(clientUUID)
	NotifyChanged()
	return nil
}

func normalizeExpirationDate(value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	location := models.GetAppLocation()
	var date time.Time
	switch typed := value.(type) {
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return nil, nil
		}
		if len(raw) < len("2006-01-02") ||
			(len(raw) > len("2006-01-02") && raw[10] != 'T' && raw[10] != ' ') {
			return nil, fmt.Errorf("expired_at must use YYYY-MM-DD format")
		}
		parsed, err := time.ParseInLocation(
			"2006-01-02",
			raw[:len("2006-01-02")],
			location,
		)
		if err != nil {
			return nil, fmt.Errorf("invalid expired_at date: %w", err)
		}
		date = parsed
	case models.LocalTime:
		date = typed.ToTime().In(location)
	case time.Time:
		date = typed.In(location)
	default:
		return nil, fmt.Errorf("expired_at must be a date string, time value, or null")
	}

	year, month, day := date.Date()
	return models.FromTime(time.Date(year, month, day, 0, 0, 0, 0, location)), nil
}

func numericFloat64(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
