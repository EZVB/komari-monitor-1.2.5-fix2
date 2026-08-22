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

func TestLedgerKeepsTrafficAfterMonitoringRecordsAreDeleted(t *testing.T) {
	db := newTrafficStatsTestDB(t)
	location := models.GetAppLocation()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, location)
	client := models.Client{
		UUID:             "durable-ledger",
		TrafficLimitType: "sum",
		TrafficResetDay:  20,
	}

	insertTrafficRecord(t, db, client.UUID, now.Add(-time.Hour), 100, 200, 10, 20)
	insertTrafficRecord(t, db, client.UUID, now, 105, 207, 5, 7)
	require.NoError(t, RecordReportWithDB(
		db,
		client,
		now,
		105,
		207,
		3600,
		5,
		7,
	))

	usage, err := CurrentWithDB(db, client, now)
	require.NoError(t, err)
	assert.Equal(t, int64(15), usage.Up)
	assert.Equal(t, int64(27), usage.Down)
	assert.Equal(t, int64(42), usage.Used)

	require.NoError(t, db.Where("client = ?", client.UUID).
		Delete(&models.Record{}).Error)
	require.NoError(t, db.Table("records_long_term").
		Where("client = ?", client.UUID).
		Delete(&models.Record{}).Error)

	usage, err = CurrentWithDB(db, client, now.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(15), usage.Up)
	assert.Equal(t, int64(27), usage.Down)
	assert.Equal(t, int64(42), usage.Used)

	insertTrafficRecord(t, db, client.UUID, now.Add(time.Minute), 108, 211, 3, 4)
	require.NoError(t, RecordReportWithDB(
		db,
		client,
		now.Add(time.Minute),
		108,
		211,
		3660,
		3,
		4,
	))

	usage, err = CurrentWithDB(db, client, now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(18), usage.Up)
	assert.Equal(t, int64(31), usage.Down)
	assert.Equal(t, int64(49), usage.Used)
}

func TestCurrentBootstrapsOfflineClientLedger(t *testing.T) {
	db := newTrafficStatsTestDB(t)
	location := models.GetAppLocation()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, location)
	client := models.Client{
		UUID:             "offline-ledger",
		TrafficLimitType: "sum",
		TrafficResetDay:  20,
	}

	insertTrafficRecord(t, db, client.UUID, now.Add(-time.Hour), 100, 200, 10, 20)
	insertTrafficRecord(t, db, client.UUID, now, 105, 207, 5, 7)

	usage, err := CurrentWithDB(db, client, now)
	require.NoError(t, err)
	assert.Equal(t, int64(42), usage.Used)

	var ledger models.ClientTrafficLedger
	require.NoError(t, db.Where("client = ?", client.UUID).
		First(&ledger).Error)
	assert.Equal(t, int64(15), ledger.CycleUp)
	assert.Equal(t, int64(27), ledger.CycleDown)
	assert.Equal(t, int64(105), ledger.LastNetTotalUp)
	assert.Equal(t, int64(207), ledger.LastNetTotalDown)

	require.NoError(t, db.Where("client = ?", client.UUID).
		Delete(&models.Record{}).Error)
	require.NoError(t, db.Table("records_long_term").
		Where("client = ?", client.UUID).
		Delete(&models.Record{}).Error)

	usage, err = CurrentWithDB(db, client, now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(15), usage.Up)
	assert.Equal(t, int64(27), usage.Down)
	assert.Equal(t, int64(42), usage.Used)
}

func TestLedgerKeepsManualStartingTrafficAndMultiplier(t *testing.T) {
	db := newTrafficStatsTestDB(t)
	location := models.GetAppLocation()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, location)
	initialAt := now.Add(-time.Hour)
	client := models.Client{
		UUID:              "durable-manual-ledger",
		TrafficLimitType:  "sum",
		TrafficMultiplier: 1,
		TrafficResetDay:   20,
		TrafficInitial:    100,
		TrafficInitialAt:  models.FromTime(initialAt),
	}

	insertTrafficRecord(t, db, client.UUID, initialAt.Add(time.Minute), 10, 20, 10, 20)
	insertTrafficRecord(t, db, client.UUID, now, 15, 27, 5, 7)
	require.NoError(t, RecordReportWithDB(
		db,
		client,
		now,
		15,
		27,
		3600,
		5,
		7,
	))

	require.NoError(t, db.Where("client = ?", client.UUID).
		Delete(&models.Record{}).Error)
	require.NoError(t, db.Table("records_long_term").
		Where("client = ?", client.UUID).
		Delete(&models.Record{}).Error)

	usage, err := CurrentWithDB(db, client, now.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(15), usage.Up)
	assert.Equal(t, int64(27), usage.Down)
	assert.Equal(t, int64(184), usage.Used)
}

func TestReportDeltaUsesCurrentCountersAfterRestart(t *testing.T) {
	up, down := ReportDelta(
		ReportCounters{NetTotalUp: 10, NetTotalDown: 20, Uptime: 5},
		ReportCounters{NetTotalUp: 1000, NetTotalDown: 2000, Uptime: 3600},
	)
	assert.Equal(t, int64(10), up)
	assert.Equal(t, int64(20), down)
}

func TestLedgerStartsFreshAtMonthlyReset(t *testing.T) {
	db := newTrafficStatsTestDB(t)
	location := models.GetAppLocation()
	beforeReset := time.Date(2026, 8, 19, 23, 59, 0, 0, location)
	afterReset := time.Date(2026, 8, 20, 0, 1, 0, 0, location)
	client := models.Client{
		UUID:             "monthly-reset-ledger",
		TrafficLimitType: "sum",
		TrafficResetDay:  20,
	}

	insertTrafficRecord(t, db, client.UUID, beforeReset, 100, 200, 10, 20)
	require.NoError(t, RecordReportWithDB(
		db,
		client,
		beforeReset,
		100,
		200,
		3600,
		10,
		20,
	))

	insertTrafficRecord(t, db, client.UUID, afterReset, 103, 204, 3, 4)
	require.NoError(t, RecordReportWithDB(
		db,
		client,
		afterReset,
		103,
		204,
		3720,
		3,
		4,
	))

	usage, err := CurrentWithDB(db, client, afterReset)
	require.NoError(t, err)
	assert.Equal(t, int64(3), usage.Up)
	assert.Equal(t, int64(4), usage.Down)
	assert.Equal(t, int64(7), usage.Used)
}

func newTrafficStatsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Record{},
		&models.ClientTrafficLedger{},
	))
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
