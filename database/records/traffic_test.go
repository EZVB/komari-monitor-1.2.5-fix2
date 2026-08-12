package records

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"github.com/stretchr/testify/assert"
)

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
