package records

import (
	"sort"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const trafficDayLayout = "2006-01-02"

// AccumulateTrafficDailyTotals persists exact per-report traffic deltas in a
// compact daily ledger. The caller should use the same transaction that saves
// the corresponding monitoring records.
func AccumulateTrafficDailyTotals(db *gorm.DB, records []models.Record) error {
	totals := make(map[string]models.TrafficDailyTotal)
	for _, record := range records {
		if record.Client == "" {
			continue
		}
		up := nonNegativeTraffic(record.TrafficUp)
		down := nonNegativeTraffic(record.TrafficDown)
		if up == 0 && down == 0 {
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

// InitializeTrafficDailyTotals performs a one-time best-effort backfill from
// retained monitoring rows. Future totals are independent of monitoring data
// retention because every new report updates the ledger directly.
func InitializeTrafficDailyTotals(db *gorm.DB, now time.Time) error {
	var count int64
	if err := db.Model(&models.TrafficDailyTotal{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
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
		if err := tx.Model(&models.TrafficDailyTotal{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
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
		return AccumulateTrafficDailyTotals(tx, backfill)
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
