package dbcore

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureLongTermRecordIntegrityRepairsDuplicates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	record := models.Record{
		Client:      "client-1",
		Time:        models.FromTime(time.Date(2026, 8, 12, 7, 30, 0, 0, time.UTC)),
		TrafficUp:   100,
		TrafficDown: 200,
	}
	for range 3 {
		require.NoError(t, db.Table("records_long_term").Create(&record).Error)
	}

	require.NoError(t, ensureLongTermRecordIntegrity(db))

	var count int64
	require.NoError(t, db.Table("records_long_term").Count(&count).Error)
	assert.Equal(t, int64(1), count)
	assert.Error(t, db.Table("records_long_term").Create(&record).Error)
}
