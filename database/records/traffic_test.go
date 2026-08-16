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

func TestMergeTrafficDeltaRecordsDeduplicatesLongTermBuckets(t *testing.T) {
	start := time.Date(2026, 8, 12, 7, 30, 0, 0, time.UTC)
	duplicate := trafficDeltaRecord{
		Time:         models.FromTime(start),
		NetTotalUp:   1_000,
		NetTotalDown: 2_000,
		TrafficUp:    100,
		TrafficDown:  200,
	}
	merged := mergeTrafficDeltaRecords(nil, []trafficDeltaRecord{
		duplicate,
		duplicate,
		duplicate,
		{
			Time:         models.FromTime(start.Add(15 * time.Minute)),
			NetTotalUp:   1_050,
			NetTotalDown: 2_075,
			TrafficUp:    50,
			TrafficDown:  75,
		},
	})

	require.Len(t, merged, 2)
	up, down := sumTrafficDeltas(merged, nil)
	assert.Equal(t, int64(150), up)
	assert.Equal(t, int64(275), down)
}

func TestMergeTrafficDeltaRecordsPrefersRawOverlap(t *testing.T) {
	start := time.Date(2026, 8, 12, 7, 30, 0, 0, time.UTC)
	recent := []trafficDeltaRecord{
		{Time: models.FromTime(start.Add(time.Minute)), TrafficUp: 10, TrafficDown: 20},
		{Time: models.FromTime(start.Add(2 * time.Minute)), TrafficUp: 30, TrafficDown: 40},
	}
	longTerm := []trafficDeltaRecord{
		{Time: models.FromTime(start), TrafficUp: 500, TrafficDown: 600},
	}

	merged := mergeTrafficDeltaRecords(recent, longTerm)
	require.Len(t, merged, 2)
	up, down := sumTrafficDeltas(merged, nil)
	assert.Equal(t, int64(40), up)
	assert.Equal(t, int64(60), down)
}

func TestTrafficRangeIncludesCompactedBucketContainingManualBaseline(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Record{}))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	start := time.Date(2026, 8, 12, 7, 35, 0, 0, time.UTC)
	rows := []models.Record{
		{Client: uuid, Time: models.FromTime(start.Truncate(15 * time.Minute)), TrafficUp: 100, TrafficDown: 200},
		{Client: uuid, Time: models.FromTime(start.Truncate(15 * time.Minute)), TrafficUp: 100, TrafficDown: 200},
		{Client: uuid, Time: models.FromTime(start.Truncate(15 * time.Minute).Add(15 * time.Minute)), TrafficUp: 50, TrafficDown: 75},
	}
	for _, row := range rows {
		require.NoError(t, db.Table("records_long_term").Create(&row).Error)
	}

	up, down, err := GetTrafficTotalsInRangeWithDB(
		db,
		uuid,
		start,
		start.Add(time.Hour),
	)
	require.NoError(t, err)
	assert.Equal(t, int64(150), up)
	assert.Equal(t, int64(275), down)
}

func TestSumTrafficDeltasUsesPersistedExactDeltas(t *testing.T) {
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	previous := &trafficDeltaRecord{
		Time:         models.FromTime(start),
		NetTotalUp:   1_000,
		NetTotalDown: 2_000,
		Uptime:       3_600,
	}
	records := []trafficDeltaRecord{
		{
			Time:         models.FromTime(start.Add(15 * time.Minute)),
			NetTotalUp:   80,
			NetTotalDown: 120,
			TrafficUp:    350,
			TrafficDown:  480,
			Uptime:       300,
		},
	}

	up, down := sumTrafficDeltas(records, previous)
	assert.Equal(t, int64(350), up)
	assert.Equal(t, int64(480), down)
}

func TestSumTrafficDeltasCountsPersistedFirstRecordWithoutBaseline(t *testing.T) {
	records := []trafficDeltaRecord{
		{
			Time:        models.FromTime(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)),
			TrafficUp:   75,
			TrafficDown: 120,
		},
	}

	up, down := sumTrafficDeltas(records, nil)
	assert.Equal(t, int64(75), up)
	assert.Equal(t, int64(120), down)
}

func TestSumTrafficDeltasPreservesExactDeltaAcrossMonotonicEndpoints(t *testing.T) {
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	previous := &trafficDeltaRecord{
		Time:         models.FromTime(start),
		NetTotalUp:   1_000,
		NetTotalDown: 2_000,
		Uptime:       3_600,
	}
	res := []trafficDeltaRecord{
		{
			Time:         models.FromTime(start.Add(15 * time.Minute)),
			NetTotalUp:   1_100,
			NetTotalDown: 2_100,
			TrafficUp:    350,
			TrafficDown:  480,
			Uptime:       4_500,
		},
	}

	up, down := sumTrafficDeltas(res, previous)
	assert.Equal(t, int64(350), up)
	assert.Equal(t, int64(480), down)
}

func TestSumTrafficDeltasRepairsLegacyZeroDelta(t *testing.T) {
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	previous := &trafficDeltaRecord{
		Time:         models.FromTime(start),
		NetTotalUp:   100,
		NetTotalDown: 200,
		Uptime:       100,
	}
	records := []trafficDeltaRecord{
		{
			Time:         models.FromTime(start.Add(time.Minute)),
			NetTotalUp:   140,
			NetTotalDown: 260,
			Uptime:       160,
		},
	}

	up, down := sumTrafficDeltas(records, previous)
	assert.Equal(t, int64(40), up)
	assert.Equal(t, int64(60), down)
}

func TestSumTrafficDeltasRecoversConfirmedRestartBaseline(t *testing.T) {
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	previous := &trafficDeltaRecord{
		Time:         models.FromTime(start),
		NetTotalUp:   1_000,
		NetTotalDown: 2_000,
		Uptime:       3_600,
	}
	records := []trafficDeltaRecord{
		{
			Time:         models.FromTime(start.Add(time.Minute)),
			NetTotalUp:   35,
			NetTotalDown: 55,
			Uptime:       30,
		},
	}

	up, down := sumTrafficDeltas(records, previous)
	assert.Equal(t, int64(35), up)
	assert.Equal(t, int64(55), down)
}

func TestSumTrafficDeltasPreservesPersistedRollbackDeltaWithoutRestartEvidence(t *testing.T) {
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	previous := &trafficDeltaRecord{
		Time:         models.FromTime(start),
		NetTotalUp:   1_000,
		NetTotalDown: 2_000,
		Uptime:       3_600,
	}
	records := []trafficDeltaRecord{
		{
			Time:         models.FromTime(start.Add(time.Minute)),
			NetTotalUp:   35,
			NetTotalDown: 55,
			TrafficUp:    35,
			TrafficDown:  55,
			Uptime:       3_660,
		},
	}

	up, down := sumTrafficDeltas(records, previous)
	assert.Equal(t, int64(35), up)
	assert.Equal(t, int64(55), down)
}

func TestSumTrafficDeltasRejectsLegacyZeroDeltaRollbackWithoutRestartEvidence(t *testing.T) {
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	previous := &trafficDeltaRecord{
		Time:         models.FromTime(start),
		NetTotalUp:   1_000,
		NetTotalDown: 2_000,
		Uptime:       3_600,
	}
	records := []trafficDeltaRecord{
		{
			Time:         models.FromTime(start.Add(time.Minute)),
			NetTotalUp:   35,
			NetTotalDown: 55,
			Uptime:       3_660,
		},
	}

	up, down := sumTrafficDeltas(records, previous)
	assert.Equal(t, int64(0), up)
	assert.Equal(t, int64(0), down)
}

func TestTrafficOverviewAggregatesAllSelectedClientsByCalendarPeriod(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Record{}))
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

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
			Time:        models.FromTime(time.Date(2026, 8, 16, 23, 59, 0, 0, location)),
			TrafficUp:   50,
			TrafficDown: 60,
		},
		{
			Client:      "client-a",
			Time:        models.FromTime(time.Date(2026, 8, 17, 1, 0, 0, 0, location)),
			TrafficUp:   10,
			TrafficDown: 20,
		},
		{
			Client:      "client-b",
			Time:        models.FromTime(time.Date(2026, 8, 17, 2, 0, 0, 0, location)),
			TrafficUp:   30,
			TrafficDown: 40,
		},
		{
			Client:      "not-selected",
			Time:        models.FromTime(time.Date(2026, 8, 17, 3, 0, 0, 0, location)),
			TrafficUp:   500,
			TrafficDown: 600,
		},
	}
	for _, row := range rows {
		require.NoError(t, db.Create(&row).Error)
	}

	overview, err := GetTrafficOverviewWithDB(
		db,
		[]string{"client-a", "client-b", "client-a"},
		now,
	)
	require.NoError(t, err)
	assert.Equal(t, TrafficPeriodTotals{Up: 40, Down: 60, Total: 100}, overview.Today)
	assert.Equal(t, TrafficPeriodTotals{Up: 190, Down: 320, Total: 510}, overview.Month)
}
