package records

import (
	"errors"
	"sort"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	trafficDayLayout             = "2006-01-02"
	trafficDailyLedgerVersionKey = "calendar_traffic_ledger_version"
	trafficDailyLedgerVersion    = "natural-calendar-v2"
)

// AccumulateTrafficDailyTotals persists exact per-report traffic deltas in a
// compact daily ledger. The caller should use the same transaction that saves
// the corresponding monitoring records.
func AccumulateTrafficDailyTotals(db *gorm.DB, records []models.Record) error {
	totals := aggregateTrafficDailyTotals(records, false)
	for _, total := range totals {
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "client"},
				{Name: "day"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"traffic_up": gorm.Expr(
					"traffic_up + ?",
					total.TrafficUp,
				),
				"traffic_down": gorm.Expr(
					"traffic_down + ?",
					total.TrafficDown,
				),
			}),
		}).Create(&total).Error; err != nil {
			return err
		}
	}
	return nil
}

func aggregateTrafficDailyTotals(
	records []models.Record,
	includeZero bool,
) []models.TrafficDailyTotal {
	totals := make(map[string]models.TrafficDailyTotal)
	for _, record := range records {
		if record.Client == "" {
			continue
		}
		up := nonNegativeTraffic(record.TrafficUp)
		down := nonNegativeTraffic(record.TrafficDown)
		if !includeZero && up == 0 && down == 0 {
			continue
		}

		day := record.Time.ToTime().In(trafficOverviewLocation).Format(trafficDayLayout)
		key := record.Client + "\x00" + day
		total := totals[key]
		total.Client = record.Client
		total.Day = day
		total.TrafficUp = saturatingAddTraffic(total.TrafficUp, up)
		total.TrafficDown = saturatingAddTraffic(total.TrafficDown, down)
		totals[key] = total
	}

	result := make([]models.TrafficDailyTotal, 0, len(totals))
	for _, total := range totals {
		result = append(result, total)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Client != result[j].Client {
			return result[i].Client < result[j].Client
		}
		return result[i].Day < result[j].Day
	})
	return result
}

// InitializeTrafficDailyTotals performs a versioned repair from retained
// monitoring rows. Only recoverable client/day buckets are replaced, so exact
// ledger rows remain available when older monitoring rows were already pruned.
// Future totals are independent of monitoring retention because every new
// report updates the ledger directly.
func InitializeTrafficDailyTotals(db *gorm.DB, now time.Time) error {
	if err := db.AutoMigrate(&models.TrafficDailyMeta{}); err != nil {
		return err
	}

	var state models.TrafficDailyMeta
	err := db.First(&state, "key = ?", trafficDailyLedgerVersionKey).Error
	if err == nil && state.Value == trafficDailyLedgerVersion {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	now = now.In(trafficOverviewLocation)
	start := time.Date(
		now.Year(), now.Month(), 1, 0, 0, 0, 0, trafficOverviewLocation,
	).AddDate(0, -1, 0)

	var clientUUIDs []string
	if err := db.Model(&models.Client{}).Pluck("uuid", &clientUUIDs).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var current models.TrafficDailyMeta
		err := tx.First(&current, "key = ?", trafficDailyLedgerVersionKey).Error
		if err == nil && current.Value == trafficDailyLedgerVersion {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		backfill, err := trafficDailyBackfillRecords(
			tx,
			uniqueTrafficClientUUIDs(clientUUIDs),
			start,
			now,
		)
		if err != nil {
			return err
		}

		for _, total := range aggregateTrafficDailyTotals(backfill, true) {
			if err := tx.Where(
				"client = ? AND day = ?",
				total.Client,
				total.Day,
			).Delete(&models.TrafficDailyTotal{}).Error; err != nil {
				return err
			}
			if err := tx.Create(&total).Error; err != nil {
				return err
			}
		}

		state = models.TrafficDailyMeta{
			Key:   trafficDailyLedgerVersionKey,
			Value: trafficDailyLedgerVersion,
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&state).Error
	})
}

func trafficDailyBackfillRecords(
	db *gorm.DB,
	clientUUIDs []string,
	start time.Time,
	end time.Time,
) ([]models.Record, error) {
	backfill := make([]models.Record, 0)
	for _, clientUUID := range clientUUIDs {
		var recent []trafficDeltaRecord
		if err := db.Table("records").
			Select(trafficOverviewSelectColumns).
			Where(
				"client = ? AND time >= ? AND time < ?",
				clientUUID,
				models.FromTime(start),
				models.FromTime(end),
			).
			Find(&recent).Error; err != nil {
			return nil, err
		}

		var longTerm []trafficDeltaRecord
		if err := db.Table("records_long_term").
			Select(trafficOverviewSelectColumns).
			Where(
				"client = ? AND time >= ? AND time < ?",
				clientUUID,
				models.FromTime(start.Truncate(15*time.Minute)),
				models.FromTime(end),
			).
			Find(&longTerm).Error; err != nil {
			return nil, err
		}

		merged := mergeTrafficDeltaRecords(recent, longTerm)
		sort.Slice(merged, func(i, j int) bool {
			return merged[i].Time.ToTime().Before(merged[j].Time.ToTime())
		})

		previous, err := getPreviousTrafficDeltaRecord(db, clientUUID, start)
		if err != nil {
			return nil, err
		}
		for index := range merged {
			current := &merged[index]
			up := nonNegativeTraffic(current.TrafficUp)
			down := nonNegativeTraffic(current.TrafficDown)
			if previous != nil {
				reset := systemCounterRestarted(*current, *previous)
				up = trafficDeltaForRecord(
					current.TrafficUp,
					current.NetTotalUp,
					previous.NetTotalUp,
					reset,
				)
				down = trafficDeltaForRecord(
					current.TrafficDown,
					current.NetTotalDown,
					previous.NetTotalDown,
					reset,
				)
			}
			backfill = append(backfill, models.Record{
				Client:      clientUUID,
				Time:        current.Time,
				TrafficUp:   up,
				TrafficDown: down,
			})
			previous = current
		}
	}
	return backfill, nil
}
