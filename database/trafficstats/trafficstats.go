package trafficstats

import (
	"math"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	recordstore "github.com/komari-monitor/komari/database/records"
	"gorm.io/gorm"
)

const usageCacheTTL = 5 * time.Second

type Usage struct {
	Up         int64
	Down       int64
	Used       int64
	ResetDay   int
	CycleStart time.Time
}

type cacheKey struct {
	resetDay       int
	cycleStartUnix int64
	initial        int64
	initialAtUnix  int64
	limitType      string
	multiplier     float64
}

type cacheEntry struct {
	key       cacheKey
	usage     Usage
	expiresAt time.Time
}

var usageCache = struct {
	sync.RWMutex
	items map[string]cacheEntry
}{
	items: make(map[string]cacheEntry),
}

// ResolveResetDay uses the explicit day first, then the expiration day.
func ResolveResetDay(client models.Client) int {
	if client.TrafficResetDay >= 1 && client.TrafficResetDay <= 31 {
		return client.TrafficResetDay
	}

	expiredAt := client.ExpiredAt.ToTime()
	if !expiredAt.IsZero() && expiredAt.Year() >= 2 && expiredAt.Year() <= 2200 {
		return expiredAt.Day()
	}
	return 1
}

// CycleStartForDay returns the most recent monthly reset boundary.
func CycleStartForDay(now time.Time, resetDay int) time.Time {
	location := now.Location()
	if resetDay < 1 || resetDay > 31 {
		resetDay = 1
	}

	candidate := clampedMonthDay(
		now.Year(),
		now.Month(),
		resetDay,
		location,
	)
	if !now.Before(candidate) {
		return candidate
	}

	previousMonth := time.Date(
		now.Year(),
		now.Month(),
		1,
		0,
		0,
		0,
		0,
		location,
	).AddDate(0, -1, 0)
	return clampedMonthDay(
		previousMonth.Year(),
		previousMonth.Month(),
		resetDay,
		location,
	)
}

func Current(client models.Client, now time.Time) (Usage, error) {
	now = now.In(models.GetAppLocation())
	resetDay := ResolveResetDay(client)
	cycleStart := CycleStartForDay(now, resetDay)
	key := buildCacheKey(client, resetDay, cycleStart)

	usageCache.RLock()
	cached, exists := usageCache.items[client.UUID]
	usageCache.RUnlock()
	if exists && cached.key == key && now.Before(cached.expiresAt) {
		return cached.usage, nil
	}

	usage, err := CurrentWithDB(dbcore.GetDBInstance(), client, now)
	if err != nil {
		return Usage{}, err
	}

	usageCache.Lock()
	usageCache.items[client.UUID] = cacheEntry{
		key:       key,
		usage:     usage,
		expiresAt: now.Add(usageCacheTTL),
	}
	usageCache.Unlock()
	return usage, nil
}

func CurrentWithDB(
	db *gorm.DB,
	client models.Client,
	now time.Time,
) (Usage, error) {
	now = now.In(models.GetAppLocation())
	resetDay := ResolveResetDay(client)
	cycleStart := CycleStartForDay(now, resetDay)
	up, down, err := recordstore.GetTrafficTotalsInRangeWithDB(
		db,
		client.UUID,
		cycleStart,
		now,
	)
	if err != nil {
		return Usage{}, err
	}

	accountedUp := up
	accountedDown := down
	initial := int64(0)

	initialAt := client.TrafficInitialAt.ToTime()
	if !initialAt.IsZero() {
		initialAt = initialAt.In(now.Location())
	}
	if !initialAt.IsZero() &&
		!initialAt.Before(cycleStart) &&
		!initialAt.After(now) {
		initial = client.TrafficInitial
		accountedUp, accountedDown, err =
			recordstore.GetTrafficTotalsInRangeWithDB(
				db,
				client.UUID,
				initialAt,
				now,
			)
		if err != nil {
			return Usage{}, err
		}
	}

	deltaUsed := applyTrafficMultiplier(
		computeUsedByType(
			client.TrafficLimitType,
			accountedUp,
			accountedDown,
		),
		client.TrafficMultiplier,
	)

	return Usage{
		Up:         up,
		Down:       down,
		Used:       saturatingAdd(initial, deltaUsed),
		ResetDay:   resetDay,
		CycleStart: cycleStart,
	}, nil
}

func Invalidate(clientUUID string) {
	usageCache.Lock()
	delete(usageCache.items, clientUUID)
	usageCache.Unlock()
}

func buildCacheKey(
	client models.Client,
	resetDay int,
	cycleStart time.Time,
) cacheKey {
	initialAt := client.TrafficInitialAt.ToTime()
	return cacheKey{
		resetDay:       resetDay,
		cycleStartUnix: cycleStart.Unix(),
		initial:        client.TrafficInitial,
		initialAtUnix:  initialAt.UnixNano(),
		limitType:      strings.ToLower(client.TrafficLimitType),
		multiplier:     client.TrafficMultiplier,
	}
}

func clampedMonthDay(
	year int,
	month time.Month,
	day int,
	location *time.Location,
) time.Time {
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, location).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, 0, 0, 0, 0, location)
}

func computeUsedByType(trafficType string, up, down int64) int64 {
	switch strings.ToLower(trafficType) {
	case "up":
		return up
	case "down":
		return down
	case "max":
		if up > down {
			return up
		}
		return down
	case "min":
		if up < down {
			return up
		}
		return down
	case "sum":
		fallthrough
	default:
		return saturatingAdd(up, down)
	}
}

func applyTrafficMultiplier(used int64, multiplier float64) int64 {
	if multiplier < 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		multiplier = 0
	}
	weighted := float64(used) * (1 + multiplier)
	if weighted >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	if weighted <= 0 {
		return 0
	}
	return int64(math.Round(weighted))
}

func saturatingAdd(left, right int64) int64 {
	if right <= 0 {
		return left
	}
	if left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}
