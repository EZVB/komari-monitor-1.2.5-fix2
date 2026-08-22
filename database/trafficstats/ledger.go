package trafficstats

import (
	"errors"
	"time"

	"github.com/komari-monitor/komari/database/models"
	recordstore "github.com/komari-monitor/komari/database/records"
	"github.com/komari-monitor/komari/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReportCounters struct {
	NetTotalUp   int64
	NetTotalDown int64
	Uptime       int64
}

// LastReportCountersWithDB returns the durable counter baseline used to keep
// traffic continuity even when all monitoring records have been deleted.
func LastReportCountersWithDB(
	db *gorm.DB,
	clientUUID string,
) (ReportCounters, bool, error) {
	var ledger models.ClientTrafficLedger
	err := db.Where("client = ?", clientUUID).First(&ledger).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ReportCounters{}, false, nil
	}
	if err != nil {
		return ReportCounters{}, false, err
	}
	if ledger.LastReportAt.ToTime().IsZero() {
		return ReportCounters{}, false, nil
	}
	return ReportCounters{
		NetTotalUp:   ledger.LastNetTotalUp,
		NetTotalDown: ledger.LastNetTotalDown,
		Uptime:       ledger.LastUptime,
	}, true, nil
}

// RecordReportWithDB appends one exact Agent delta to the durable billing
// ledger. Period changes are bootstrapped once from monitoring history; after
// that, monitoring retention no longer affects the accumulated value.
func RecordReportWithDB(
	db *gorm.DB,
	client models.Client,
	reportAt time.Time,
	netTotalUp int64,
	netTotalDown int64,
	uptime int64,
	deltaUp int64,
	deltaDown int64,
) error {
	reportAt = reportAt.In(models.GetAppLocation())
	cycleStart := CycleStartForDay(reportAt, ResolveResetDay(client))
	accountStart, _ := resolveAccountStart(client, cycleStart, reportAt)

	var ledger models.ClientTrafficLedger
	err := db.Where("client = ?", client.UUID).First(&ledger).Error
	found := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	cycleMatches := found && ledger.CycleStart.ToTime().Equal(cycleStart)
	accountMatches := found && ledger.AccountStart.ToTime().Equal(accountStart)
	if cycleMatches {
		ledger.CycleUp = saturatingAdd(ledger.CycleUp, deltaUp)
		ledger.CycleDown = saturatingAdd(ledger.CycleDown, deltaDown)
	} else {
		ledger.CycleUp, ledger.CycleDown, err =
			recordstore.GetTrafficTotalsInRangeWithDB(
				db,
				client.UUID,
				cycleStart,
				reportAt,
			)
		if err != nil {
			return err
		}
		ledger.CycleStart = models.FromTime(cycleStart)
	}

	if accountMatches {
		ledger.AccountUp = saturatingAdd(ledger.AccountUp, deltaUp)
		ledger.AccountDown = saturatingAdd(ledger.AccountDown, deltaDown)
	} else if accountStart.Equal(cycleStart) && !cycleMatches {
		ledger.AccountUp = ledger.CycleUp
		ledger.AccountDown = ledger.CycleDown
		ledger.AccountStart = models.FromTime(accountStart)
	} else {
		ledger.AccountUp, ledger.AccountDown, err =
			recordstore.GetTrafficTotalsInRangeWithDB(
				db,
				client.UUID,
				accountStart,
				reportAt,
			)
		if err != nil {
			return err
		}
		ledger.AccountStart = models.FromTime(accountStart)
	}

	ledger.Client = client.UUID
	ledger.LastNetTotalUp = netTotalUp
	ledger.LastNetTotalDown = netTotalDown
	ledger.LastUptime = uptime
	ledger.LastReportAt = models.FromTime(reportAt)
	return db.Save(&ledger).Error
}

func resolveAccountStart(
	client models.Client,
	cycleStart time.Time,
	now time.Time,
) (time.Time, int64) {
	initialAt := client.TrafficInitialAt.ToTime()
	if !initialAt.IsZero() {
		initialAt = initialAt.In(now.Location())
	}
	if !initialAt.IsZero() &&
		!initialAt.Before(cycleStart) &&
		!initialAt.After(now) {
		return initialAt, client.TrafficInitial
	}
	return cycleStart, 0
}

func ledgerTotalsWithDB(
	db *gorm.DB,
	clientUUID string,
	cycleStart time.Time,
	accountStart time.Time,
) (int64, int64, int64, int64, bool, error) {
	var ledger models.ClientTrafficLedger
	err := db.Where("client = ?", clientUUID).First(&ledger).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, 0, 0, false, err
	}
	if !ledger.CycleStart.ToTime().Equal(cycleStart) ||
		!ledger.AccountStart.ToTime().Equal(accountStart) {
		return 0, 0, 0, 0, false, nil
	}
	return ledger.CycleUp,
		ledger.CycleDown,
		ledger.AccountUp,
		ledger.AccountDown,
		true,
		nil
}

func bootstrapLedgerWithDB(
	db *gorm.DB,
	client models.Client,
	cycleStart time.Time,
	accountStart time.Time,
	cycleUp int64,
	cycleDown int64,
	accountUp int64,
	accountDown int64,
) error {
	lastCounters, lastReportAt, found, err := latestReportCountersWithDB(
		db,
		client.UUID,
	)
	if err != nil || !found {
		return err
	}

	ledger := models.ClientTrafficLedger{
		Client:           client.UUID,
		CycleStart:       models.FromTime(cycleStart),
		AccountStart:     models.FromTime(accountStart),
		CycleUp:          cycleUp,
		CycleDown:        cycleDown,
		AccountUp:        accountUp,
		AccountDown:      accountDown,
		LastNetTotalUp:   lastCounters.NetTotalUp,
		LastNetTotalDown: lastCounters.NetTotalDown,
		LastUptime:       lastCounters.Uptime,
		LastReportAt:     models.FromTime(lastReportAt),
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&ledger).Error
}

func latestReportCountersWithDB(
	db *gorm.DB,
	clientUUID string,
) (ReportCounters, time.Time, bool, error) {
	recent, recentFound, err := latestRecordWithDB(
		db.Table("records"),
		clientUUID,
	)
	if err != nil {
		return ReportCounters{}, time.Time{}, false, err
	}
	longTerm, longTermFound, err := latestRecordWithDB(
		db.Table("records_long_term"),
		clientUUID,
	)
	if err != nil {
		return ReportCounters{}, time.Time{}, false, err
	}

	var latest models.Record
	switch {
	case recentFound && longTermFound:
		latest = recent
		if longTerm.Time.ToTime().After(recent.Time.ToTime()) {
			latest = longTerm
		}
	case recentFound:
		latest = recent
	case longTermFound:
		latest = longTerm
	default:
		return ReportCounters{}, time.Time{}, false, nil
	}

	return ReportCounters{
		NetTotalUp:   latest.NetTotalUp,
		NetTotalDown: latest.NetTotalDown,
		Uptime:       latest.Uptime,
	}, latest.Time.ToTime(), true, nil
}

func latestRecordWithDB(
	query *gorm.DB,
	clientUUID string,
) (models.Record, bool, error) {
	var record models.Record
	err := query.
		Select("time, net_total_up, net_total_down, uptime").
		Where("client = ?", clientUUID).
		Order("time DESC").
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Record{}, false, nil
	}
	if err != nil {
		return models.Record{}, false, err
	}
	return record, true, nil
}

// ReportDelta returns the upload and download added by one Agent report.
func ReportDelta(current, previous ReportCounters) (int64, int64) {
	restarted := current.Uptime > 0 &&
		previous.Uptime > 0 &&
		current.Uptime < previous.Uptime
	return utils.ComputeTrafficDeltaWithReset(
			current.NetTotalUp,
			previous.NetTotalUp,
			restarted,
		),
		utils.ComputeTrafficDeltaWithReset(
			current.NetTotalDown,
			previous.NetTotalDown,
			restarted,
		)
}
