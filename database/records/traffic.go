package records

import (
	"errors"
	"math"
	"sort"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils"
	"gorm.io/gorm"
)

type trafficDeltaRecord struct {
	Time         models.LocalTime `gorm:"column:time"`
	NetTotalUp   int64            `gorm:"column:net_total_up"`
	NetTotalDown int64            `gorm:"column:net_total_down"`
	TrafficUp    int64            `gorm:"column:traffic_up"`
	TrafficDown  int64            `gorm:"column:traffic_down"`
	Uptime       int64            `gorm:"column:uptime"`
}

// GetTrafficTotalsInRange returns reset-aware upload and download deltas.
func GetTrafficTotalsInRange(clientUUID string, start, end time.Time) (int64, int64, error) {
	return GetTrafficTotalsInRangeWithDB(
		dbcore.GetDBInstance(),
		clientUUID,
		start,
		end,
	)
}

// GetTrafficTotalsInRangeWithDB is the testable form of GetTrafficTotalsInRange.
func GetTrafficTotalsInRangeWithDB(
	db *gorm.DB,
	clientUUID string,
	start time.Time,
	end time.Time,
) (int64, int64, error) {
	if !start.Before(end) {
		return 0, 0, nil
	}

	var recentRecords []trafficDeltaRecord
	if err := db.Table("records").
		Select(trafficDeltaSelectColumns).
		Where(
			"client = ? AND time >= ? AND time <= ?",
			clientUUID,
			models.FromTime(start),
			models.FromTime(end),
		).
		Find(&recentRecords).Error; err != nil {
		return 0, 0, err
	}

	var longTermRecords []trafficDeltaRecord
	longTermStart := start.In(models.GetAppLocation()).Truncate(15 * time.Minute)
	if err := db.Table("records_long_term").
		Select(trafficDeltaSelectColumns).
		Where(
			"client = ? AND time >= ? AND time <= ?",
			clientUUID,
			models.FromTime(longTermStart),
			models.FromTime(end),
		).
		Find(&longTermRecords).Error; err != nil {
		return 0, 0, err
	}

	merged := mergeTrafficDeltaRecords(recentRecords, longTermRecords)
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Time.ToTime().Before(merged[j].Time.ToTime())
	})

	previous, err := getPreviousTrafficDeltaRecord(db, clientUUID, start)
	if err != nil {
		return 0, 0, err
	}

	up, down := sumTrafficDeltas(merged, previous)
	return up, down, nil
}

const trafficDeltaSelectColumns = "time, net_total_up, net_total_down, traffic_up, traffic_down, uptime"

func mergeTrafficDeltaRecords(
	recentRecords []trafficDeltaRecord,
	longTermRecords []trafficDeltaRecord,
) []trafficDeltaRecord {
	recentRecords = deduplicateTrafficRecords(recentRecords, false)
	longTermRecords = deduplicateTrafficRecords(longTermRecords, true)

	rawSlots := make(map[int64]struct{}, len(recentRecords))
	for _, record := range recentRecords {
		rawSlots[trafficRecordSlot(record)] = struct{}{}
	}

	merged := make(
		[]trafficDeltaRecord,
		0,
		len(longTermRecords)+len(recentRecords),
	)
	for _, record := range longTermRecords {
		slot := trafficRecordSlot(record)
		if _, hasRawSlot := rawSlots[slot]; hasRawSlot {
			continue
		}
		merged = append(merged, record)
	}

	merged = append(merged, recentRecords...)

	return merged
}

func deduplicateTrafficRecords(
	records []trafficDeltaRecord,
	bySlot bool,
) []trafficDeltaRecord {
	unique := make(map[int64]trafficDeltaRecord, len(records))
	for _, record := range records {
		key := record.Time.ToTime().UnixNano()
		if bySlot {
			key = trafficRecordSlot(record)
		}
		if existing, ok := unique[key]; !ok || preferTrafficRecord(record, existing) {
			unique[key] = record
		}
	}

	result := make([]trafficDeltaRecord, 0, len(unique))
	for _, record := range unique {
		result = append(result, record)
	}
	return result
}

func trafficRecordSlot(record trafficDeltaRecord) int64 {
	return record.Time.ToTime().Truncate(15 * time.Minute).Unix()
}

func preferTrafficRecord(candidate, existing trafficDeltaRecord) bool {
	candidateDelta := nonNegativeTraffic(candidate.TrafficUp) +
		nonNegativeTraffic(candidate.TrafficDown)
	existingDelta := nonNegativeTraffic(existing.TrafficUp) +
		nonNegativeTraffic(existing.TrafficDown)
	if candidateDelta != existingDelta {
		return candidateDelta > existingDelta
	}
	return candidate.NetTotalUp+candidate.NetTotalDown >
		existing.NetTotalUp+existing.NetTotalDown
}

func getPreviousTrafficDeltaRecord(
	db *gorm.DB,
	clientUUID string,
	before time.Time,
) (*trafficDeltaRecord, error) {
	recent, err := latestTrafficDeltaRecordBefore(
		db.Table("records"),
		clientUUID,
		before,
	)
	if err != nil {
		return nil, err
	}

	longTerm, err := latestTrafficDeltaRecordBefore(
		db.Table("records_long_term"),
		clientUUID,
		before,
	)
	if err != nil {
		return nil, err
	}

	if recent == nil {
		return longTerm, nil
	}
	if longTerm == nil {
		return recent, nil
	}
	if longTerm.Time.ToTime().After(recent.Time.ToTime()) {
		return longTerm, nil
	}
	return recent, nil
}

func latestTrafficDeltaRecordBefore(
	query *gorm.DB,
	clientUUID string,
	before time.Time,
) (*trafficDeltaRecord, error) {
	var record trafficDeltaRecord
	err := query.
		Select(trafficDeltaSelectColumns).
		Where("client = ? AND time < ?", clientUUID, models.FromTime(before)).
		Order("time DESC").
		First(&record).Error
	if err == nil {
		return &record, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
}

func sumTrafficDeltas(
	records []trafficDeltaRecord,
	previous *trafficDeltaRecord,
) (int64, int64) {
	var totalUp int64
	var totalDown int64

	for i := range records {
		upDelta := nonNegativeTraffic(records[i].TrafficUp)
		downDelta := nonNegativeTraffic(records[i].TrafficDown)
		if previous != nil {
			reset := systemCounterRestarted(records[i], *previous)
			upDelta = trafficDeltaForRecord(
				records[i].TrafficUp,
				records[i].NetTotalUp,
				previous.NetTotalUp,
				reset,
			)
			downDelta = trafficDeltaForRecord(
				records[i].TrafficDown,
				records[i].NetTotalDown,
				previous.NetTotalDown,
				reset,
			)
		}
		totalUp = saturatingAddTraffic(totalUp, upDelta)
		totalDown = saturatingAddTraffic(totalDown, downDelta)
		previous = &records[i]
	}

	return totalUp, totalDown
}

func nonNegativeTraffic(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func trafficDeltaForRecord(stored, current, previous int64, reset bool) int64 {
	stored = nonNegativeTraffic(stored)
	if stored > 0 {
		// Persisted deltas are captured from every report before records are
		// compacted. They retain traffic across sub-samples and counter rebases
		// that cannot be reconstructed from two cumulative endpoint totals.
		return stored
	}

	// Older rows did not persist exact deltas. Fall back to cumulative totals
	// only for those rows so current data is never counted twice.
	return utils.ComputeTrafficDeltaWithReset(current, previous, reset)
}

func systemCounterRestarted(current, previous trafficDeltaRecord) bool {
	return current.Uptime > 0 && previous.Uptime > 0 && current.Uptime < previous.Uptime
}

func saturatingAddTraffic(left, right int64) int64 {
	if right <= 0 {
		return left
	}
	if left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}
