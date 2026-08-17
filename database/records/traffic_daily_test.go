package records

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTrafficOverviewUsesDailyLedgerAfterMonitoringRowsAreDeleted(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Client{},
		&models.Record{},
		&models.TrafficDailyTotal{},
	))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))
	require.NoError(t, db.Create(&models.Client{UUID: "client-a", Token: "token-a"}).Error)

	location := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, location)
	rows := []models.Record{
		{
			Client:      "client-a",
			Time:        models.FromTime(time.Date(2026, 8, 1, 1, 0, 0, 0, location)),
			TrafficUp:   100,
			TrafficDown: 200,
		},
		{
			Client:      "client-a",
			Time:        models.FromTime(time.Date(2026, 8, 16, 23, 0, 0, 0, location)),
			TrafficUp:   50,
			TrafficDown: 60,
		},
		{
			Client:      "client-a",
			Time:        models.FromTime(time.Date(2026, 8, 17, 1, 0, 0, 0, location)),
			TrafficUp:   10,
			TrafficDown: 20,
		},
	}
	for index, row := range rows {
		target := db
		if index < 2 {
			target = db.Table("records_long_term")
		}
		require.NoError(t, target.Create(&row).Error)
	}

	require.NoError(t, InitializeTrafficDailyTotals(db, now))
	require.NoError(t, db.Exec("DELETE FROM records").Error)
	require.NoError(t, db.Exec("DELETE FROM records_long_term").Error)

	overview, err := GetTrafficOverviewWithDB(db, []string{"client-a"}, now)
	require.NoError(t, err)
	assert.Equal(t, TrafficPeriodTotals{Up: 10, Down: 20, Total: 30}, overview.Today)
	assert.Equal(t, TrafficPeriodTotals{Up: 160, Down: 280, Total: 440}, overview.Month)

	// Startup initialization is idempotent and must not duplicate totals.
	require.NoError(t, InitializeTrafficDailyTotals(db, now.Add(time.Minute)))
	overview, err = GetTrafficOverviewWithDB(db, []string{"client-a"}, now)
	require.NoError(t, err)
	assert.Equal(t, TrafficPeriodTotals{Up: 160, Down: 280, Total: 440}, overview.Month)
}

func TestAccumulateTrafficDailyTotalsUsesShanghaiDayAndAddsDeltas(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TrafficDailyTotal{}))

	require.NoError(t, AccumulateTrafficDailyTotals(db, []models.Record{
		{
			Client:      "client-a",
			Time:        models.FromTime(time.Date(2026, 8, 16, 15, 59, 0, 0, time.UTC)),
			TrafficUp:   1,
			TrafficDown: 2,
		},
		{
			Client:      "client-a",
			Time:        models.FromTime(time.Date(2026, 8, 16, 16, 0, 0, 0, time.UTC)),
			TrafficUp:   3,
			TrafficDown: 4,
		},
	}))
	require.NoError(t, AccumulateTrafficDailyTotals(db, []models.Record{
		{
			Client:      "client-a",
			Time:        models.FromTime(time.Date(2026, 8, 17, 1, 0, 0, 0, time.UTC)),
			TrafficUp:   5,
			TrafficDown: 6,
		},
	}))

	var rows []models.TrafficDailyTotal
	require.NoError(t, db.Order("day ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, "2026-08-16", rows[0].Day)
	assert.Equal(t, int64(1), rows[0].TrafficUp)
	assert.Equal(t, int64(2), rows[0].TrafficDown)
	assert.Equal(t, "2026-08-17", rows[1].Day)
	assert.Equal(t, int64(8), rows[1].TrafficUp)
	assert.Equal(t, int64(10), rows[1].TrafficDown)
}

func TestTrafficOverviewUsesCurrentMonthManualBaselineWithoutChangingToday(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Record{},
		&models.TrafficDailyTotal{},
	))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	location := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, location)
	initialAt := time.Date(2026, 8, 15, 12, 0, 0, 0, location)
	require.NoError(t, db.Create(&[]models.TrafficDailyTotal{
		{
			Client:      "client-a",
			Day:         "2026-08-01",
			TrafficUp:   100,
			TrafficDown: 200,
		},
		{
			Client:      "client-a",
			Day:         "2026-08-17",
			TrafficUp:   10,
			TrafficDown: 20,
		},
		{
			Client:      "client-b",
			Day:         "2026-08-17",
			TrafficUp:   5,
			TrafficDown: 15,
		},
	}).Error)
	require.NoError(t, db.Create(&[]models.Record{
		{
			Client:      "client-a",
			Time:        models.FromTime(initialAt.Add(time.Minute)),
			TrafficUp:   30,
			TrafficDown: 70,
		},
	}).Error)

	overview, err := GetTrafficOverviewForClientsWithDB(
		db,
		[]models.Client{
			{
				UUID:             "client-a",
				TrafficInitial:   1_000,
				TrafficInitialAt: models.FromTime(initialAt),
			},
			{UUID: "client-b"},
		},
		now,
	)
	require.NoError(t, err)

	assert.Equal(t, TrafficPeriodTotals{Up: 15, Down: 35, Total: 50}, overview.Today)
	assert.Equal(t, TrafficPeriodTotals{Up: 335, Down: 785, Total: 1_120}, overview.Month)
}

func TestTrafficOverviewIgnoresManualBaselineFromPreviousMonth(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TrafficDailyTotal{}))

	location := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, location)
	require.NoError(t, db.Create(&models.TrafficDailyTotal{
		Client:      "client-a",
		Day:         "2026-08-17",
		TrafficUp:   10,
		TrafficDown: 20,
	}).Error)

	overview, err := GetTrafficOverviewForClientsWithDB(
		db,
		[]models.Client{
			{
				UUID:             "client-a",
				TrafficInitial:   1_000,
				TrafficInitialAt: models.FromTime(time.Date(2026, 7, 31, 12, 0, 0, 0, location)),
			},
		},
		now,
	)
	require.NoError(t, err)
	assert.Equal(t, TrafficPeriodTotals{Up: 10, Down: 20, Total: 30}, overview.Month)
}
