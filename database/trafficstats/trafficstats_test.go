package trafficstats

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveResetDay(t *testing.T) {
	location := models.GetAppLocation()
	expiration := time.Date(2026, 12, 7, 0, 0, 0, 0, location)

	assert.Equal(t, 20, ResolveResetDay(models.Client{
		TrafficResetDay: 20,
		ExpiredAt:       models.FromTime(expiration),
	}))
	assert.Equal(t, 7, ResolveResetDay(models.Client{
		ExpiredAt: models.FromTime(expiration),
	}))
	assert.Equal(t, 1, ResolveResetDay(models.Client{}))
}

func TestCycleStartForDay(t *testing.T) {
	location := models.GetAppLocation()

	assert.Equal(
		t,
		time.Date(2026, 7, 20, 0, 0, 0, 0, location),
		CycleStartForDay(time.Date(2026, 7, 26, 12, 0, 0, 0, location), 20),
	)
	assert.Equal(
		t,
		time.Date(2026, 6, 20, 0, 0, 0, 0, location),
		CycleStartForDay(time.Date(2026, 7, 19, 12, 0, 0, 0, location), 20),
	)
	assert.Equal(
		t,
		time.Date(2026, 2, 28, 0, 0, 0, 0, location),
		CycleStartForDay(time.Date(2026, 2, 28, 12, 0, 0, 0, location), 31),
	)
	assert.Equal(
		t,
		time.Date(2026, 1, 31, 0, 0, 0, 0, location),
		CycleStartForDay(time.Date(2026, 2, 27, 12, 0, 0, 0, location), 31),
	)
}

func TestCurrentWithDBUsesManualStartingTraffic(t *testing.T) {
	db := newTrafficStatsTestDB(t)
	location := models.GetAppLocation()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, location)
	initialAt := time.Date(2026, 7, 22, 8, 0, 0, 0, location)
	client := models.Client{
		UUID:              "manual-start",
		TrafficLimitType:  "sum",
		TrafficMultiplier: 2,
		TrafficResetDay:   20,
		TrafficInitial:    100,
		TrafficInitialAt:  models.FromTime(initialAt),
	}

	cycleStart := time.Date(2026, 7, 20, 0, 0, 0, 0, location)
	insertTrafficRecord(t, db, client.UUID, cycleStart.Add(-time.Minute), 70, 150, 0, 0)
	insertTrafficRecord(t, db, client.UUID, cycleStart.Add(time.Minute), 80, 170, 10, 20)
	insertTrafficRecord(t, db, client.UUID, initialAt.Add(-time.Minute), 100, 200, 20, 30)
	insertTrafficRecord(t, db, client.UUID, initialAt.Add(time.Minute), 105, 207, 5, 7)

	usage, err := CurrentWithDB(db, client, now)
	require.NoError(t, err)
	assert.Equal(t, int64(35), usage.Up)
	assert.Equal(t, int64(57), usage.Down)
	assert.Equal(t, int64(136), usage.Used)
	assert.Equal(t, 20, usage.ResetDay)
	assert.Equal(t, cycleStart, usage.CycleStart)
}

func TestCurrentWithDBShowsManualValueBeforeNextReport(t *testing.T) {
	db := newTrafficStatsTestDB(t)
	location := models.GetAppLocation()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, location)
	initialAt := now.Add(-time.Minute)
	client := models.Client{
		UUID:              "manual-start-before-report",
		TrafficLimitType:  "sum",
		TrafficMultiplier: 1,
		TrafficResetDay:   20,
		TrafficInitial:    54 * 1024 * 1024 * 1024,
		TrafficInitialAt:  models.FromTime(initialAt),
	}

	insertTrafficRecord(t, db, client.UUID, initialAt.Add(-time.Minute), 100, 200, 0, 0)

	usage, err := CurrentWithDB(db, client, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), usage.Up)
	assert.Equal(t, int64(0), usage.Down)
	assert.Equal(t, client.TrafficInitial, usage.Used)
}

func TestCurrentWithDBIgnoresManualStartFromPreviousCycle(t *testing.T) {
	db := newTrafficStatsTestDB(t)
	location := models.GetAppLocation()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, location)
	cycleStart := time.Date(2026, 7, 20, 0, 0, 0, 0, location)
	client := models.Client{
		UUID:              "expired-manual-start",
		TrafficLimitType:  "sum",
		TrafficMultiplier: 1.5,
		TrafficResetDay:   20,
		TrafficInitial:    1000,
		TrafficInitialAt: models.FromTime(
			time.Date(2026, 7, 19, 8, 0, 0, 0, location),
		),
	}

	insertTrafficRecord(t, db, client.UUID, cycleStart.Add(-time.Minute), 100, 200, 0, 0)
	insertTrafficRecord(t, db, client.UUID, cycleStart.Add(time.Minute), 110, 220, 10, 20)

	usage, err := CurrentWithDB(db, client, now)
	require.NoError(t, err)
	assert.Equal(t, int64(10), usage.Up)
	assert.Equal(t, int64(20), usage.Down)
	assert.Equal(t, int64(75), usage.Used)
}

func TestApplyTrafficMultiplierUsesAdditionalMultiples(t *testing.T) {
	assert.Equal(t, int64(100), applyTrafficMultiplier(100, 0))
	assert.Equal(t, int64(100), applyTrafficMultiplier(100, -1))
	assert.Equal(t, int64(200), applyTrafficMultiplier(100, 1))
	assert.Equal(t, int64(300), applyTrafficMultiplier(100, 2))
	assert.Equal(t, int64(250), applyTrafficMultiplier(100, 1.5))
}

func TestCurrentWithDBUsesSelectedXraySource(t *testing.T) {
	db := newTrafficStatsTestDB(t)
	location := models.GetAppLocation()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, location)
	cycleStart := time.Date(2026, 7, 20, 0, 0, 0, 0, location)
	client := models.Client{
		UUID:             "xray-source",
		TrafficLimitType: "sum",
		TrafficResetDay:  20,
		TrafficSource:    models.TrafficSourceXray,
	}

	require.NoError(t, db.Create(&models.Record{
		Client:        client.UUID,
		Time:          models.FromTime(cycleStart.Add(-time.Minute)),
		NetTotalUp:    1000,
		NetTotalDown:  2000,
		XrayTotalUp:   100,
		XrayTotalDown: 200,
		XrayBootTime:  10,
		XrayAvailable: true,
	}).Error)
	require.NoError(t, db.Create(&models.Record{
		Client:        client.UUID,
		Time:          models.FromTime(cycleStart.Add(time.Minute)),
		NetTotalUp:    5000,
		NetTotalDown:  8000,
		XrayTotalUp:   130,
		XrayTotalDown: 260,
		XrayBootTime:  10,
		XrayAvailable: true,
	}).Error)

	usage, err := CurrentWithDB(db, client, now)
	require.NoError(t, err)
	assert.Equal(t, int64(30), usage.Up)
	assert.Equal(t, int64(60), usage.Down)
	assert.Equal(t, int64(90), usage.Used)
}

func newTrafficStatsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Record{}))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))
	return db
}

func insertTrafficRecord(
	t *testing.T,
	db *gorm.DB,
	clientUUID string,
	recordTime time.Time,
	totalUp int64,
	totalDown int64,
	trafficUp int64,
	trafficDown int64,
) {
	t.Helper()

	require.NoError(t, db.Create(&models.Record{
		Client:       clientUUID,
		Time:         models.FromTime(recordTime),
		NetTotalUp:   totalUp,
		NetTotalDown: totalDown,
		TrafficUp:    trafficUp,
		TrafficDown:  trafficDown,
	}).Error)
}
