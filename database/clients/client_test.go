package clients

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
)

func TestNormalizeExpirationDatePreservesCalendarDate(t *testing.T) {
	tests := []string{
		"2026-09-05",
		"2026-09-05T00:00:00+08:00",
		"2026-09-05 00:00:00",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			value, err := normalizeExpirationDate(input)
			if err != nil {
				t.Fatalf("normalizeExpirationDate(%q): %v", input, err)
			}
			expiredAt, ok := value.(models.LocalTime)
			if !ok {
				t.Fatalf("normalizeExpirationDate(%q) returned %T", input, value)
			}
			date := expiredAt.ToTime()
			if date.Year() != 2026 || date.Month() != time.September || date.Day() != 5 {
				t.Fatalf("normalizeExpirationDate(%q) = %s, want 2026-09-05", input, date)
			}
			if date.Hour() != 0 || date.Minute() != 0 || date.Second() != 0 {
				t.Fatalf("normalizeExpirationDate(%q) = %s, want local midnight", input, date)
			}
		})
	}
}

func TestNormalizeExpirationDateRejectsInvalidValues(t *testing.T) {
	for _, input := range []interface{}{"2026-02-30", "2026/09/05", 20260905} {
		if _, err := normalizeExpirationDate(input); err == nil {
			t.Fatalf("normalizeExpirationDate(%v) unexpectedly succeeded", input)
		}
	}
}

func TestNormalizeExpirationDateAllowsNull(t *testing.T) {
	value, err := normalizeExpirationDate(nil)
	if err != nil {
		t.Fatalf("normalizeExpirationDate(nil): %v", err)
	}
	if value != nil {
		t.Fatalf("normalizeExpirationDate(nil) = %v, want nil", value)
	}
}

func TestNormalizeExpirationDateAcceptsAutoRenewalTime(t *testing.T) {
	input := models.FromTime(time.Date(2027, time.October, 6, 15, 30, 0, 0, models.GetAppLocation()))
	value, err := normalizeExpirationDate(input)
	if err != nil {
		t.Fatalf("normalizeExpirationDate(LocalTime): %v", err)
	}
	expiredAt, ok := value.(models.LocalTime)
	if !ok {
		t.Fatalf("normalizeExpirationDate(LocalTime) returned %T", value)
	}
	date := expiredAt.ToTime()
	if date.Year() != 2027 || date.Month() != time.October || date.Day() != 6 {
		t.Fatalf("normalizeExpirationDate(LocalTime) = %s, want 2027-10-06", date)
	}
	if date.Hour() != 0 || date.Minute() != 0 || date.Second() != 0 {
		t.Fatalf("normalizeExpirationDate(LocalTime) = %s, want local midnight", date)
	}
}

func TestNewClientBillingDefaults(t *testing.T) {
	now := time.Date(2026, time.August, 10, 15, 30, 0, 0, models.GetAppLocation())
	client := newClient("client-id", "token", "test", now)

	if client.Price != 0 {
		t.Fatalf("Price = %v, want 0", client.Price)
	}
	if client.Currency != "¥" {
		t.Fatalf("Currency = %q, want ¥", client.Currency)
	}
	if !client.AutoRenewal {
		t.Fatal("AutoRenewal = false, want true")
	}
	expiredAt := client.ExpiredAt.ToTime()
	if expiredAt.Year() != 2026 || expiredAt.Month() != time.August || expiredAt.Day() != 10 {
		t.Fatalf("ExpiredAt = %s, want 2026-08-10", expiredAt)
	}
	if expiredAt.Hour() != 0 || expiredAt.Minute() != 0 || expiredAt.Second() != 0 {
		t.Fatalf("ExpiredAt = %s, want local midnight", expiredAt)
	}
}
