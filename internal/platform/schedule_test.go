package platform

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateNextRun_Hourly(t *testing.T) {
	next := calculateNextRun("hourly", nil)
	diff := next.Sub(time.Now())
	assert.GreaterOrEqual(t, diff, time.Duration(0), "next should be in the future")
	assert.LessOrEqual(t, diff, time.Hour, "next should be within 1 hour (at top of next hour)")
}

func TestCalculateNextRun_Daily(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"time": "09:00"})
	next := calculateNextRun("daily", params)
	assert.Equal(t, 9, next.Hour())
	assert.Equal(t, 0, next.Minute())
	assert.NotZero(t, next.Day())
}

func TestCalculateNextRun_Weekly(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"day": "monday", "time": "09:00"})
	next := calculateNextRun("weekly", params)
	assert.Equal(t, 9, next.Hour())
	assert.Equal(t, 0, next.Minute())
}

func TestCalculateNextRun_Monthly(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"date": 15, "time": "09:00"})
	next := calculateNextRun("monthly", params)
	assert.Equal(t, 15, next.Day())
	assert.Equal(t, 9, next.Hour())
	assert.Equal(t, 0, next.Minute())
}

func TestCalculateNextRun_Now(t *testing.T) {
	next := calculateNextRun("now", nil)
	diff := next.Sub(time.Now())
	assert.Less(t, diff, time.Second*10)
}

func TestCalculateNextRun_Daily_Noon(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"time": "12:00"})
	next := calculateNextRun("daily", params)
	assert.Equal(t, 12, next.Hour())
}

func TestCalculateNextRun_Weekly_Friday(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"day": "friday", "time": "14:30"})
	next := calculateNextRun("weekly", params)
	assert.Equal(t, 14, next.Hour())
	assert.Equal(t, 30, next.Minute())
}

func TestCalculateNextRun_Monthly_25th(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"date": 25, "time": "08:00"})
	next := calculateNextRun("monthly", params)
	assert.Equal(t, 25, next.Day())
	assert.Equal(t, 8, next.Hour())
}

func TestCalculateNextRun_Daily_Tomorrow(t *testing.T) {
	now := time.Now()
	params, _ := json.Marshal(map[string]interface{}{"time": "00:00"})
	next := calculateNextRun("daily", params)
	if now.Hour() > 0 {
		assert.Greater(t, next.Day(), now.Day())
	}
}

func TestParseTimeOfDay(t *testing.T) {
	t1 := parseTimeOfDay("09:30")
	assert.Equal(t, 9, t1.Hour())
	assert.Equal(t, 30, t1.Minute())
}

func TestParseTimeOfDay_Default(t *testing.T) {
	t1 := parseTimeOfDay("invalid")
	assert.Equal(t, 9, t1.Hour())
	assert.Equal(t, 0, t1.Minute())
}

func TestParseTimeOfDay_Midnight(t *testing.T) {
	t1 := parseTimeOfDay("00:00")
	assert.Equal(t, 0, t1.Hour())
	assert.Equal(t, 0, t1.Minute())
}

func TestParseTimeOfDay_Evening(t *testing.T) {
	t1 := parseTimeOfDay("18:30")
	assert.Equal(t, 18, t1.Hour())
	assert.Equal(t, 30, t1.Minute())
}

func TestCalculateNextRun_Daily_PastTime(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"time": "00:00"})
	next := calculateNextRun("daily", params)
	assert.NotZero(t, next.Day())
}

func TestCalculateNextRun_Weekly_Tomorrow(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"day": "tuesday", "time": "10:00"})
	next := calculateNextRun("weekly", params)
	assert.NotZero(t, next.Day())
}

func TestCalculateNextRun_Monthly_NextMonth(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"date": 1, "time": "09:00"})
	next := calculateNextRun("monthly", params)
	assert.NotZero(t, next.Day())
	assert.NotZero(t, next.Month())
}

func TestCalculateNextRun_Daily_Timezone(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"time": "15:00"})
	next := calculateNextRun("daily", params)
	assert.Equal(t, 15, next.Hour())
	assert.Equal(t, 0, next.Minute())
}

func TestCalculateNextRun_Hourly_Midnight(t *testing.T) {
	now := time.Now()
	next := calculateNextRun("hourly", nil)
	// Next run must be in the future and on a clean hour boundary
	assert.True(t, next.After(now), "next run must be after now")
	assert.Equal(t, 0, next.Minute())
	// Handle day wrap: next hour is now.Hour()+1, or 0 if now.Hour() is 23
	expectedHour := (now.Hour() + 1) % 24
	assert.Equal(t, expectedHour, next.Hour(), "next run should be next hour")
}

func TestCalculateNextRun_Weekly_Sunday(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"day": "sunday", "time": "08:00"})
	next := calculateNextRun("weekly", params)
	assert.Equal(t, 8, next.Hour())
	assert.Equal(t, 0, next.Minute())
}

func TestCalculateNextRun_Monthly_EndOfMonth(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{"date": 28, "time": "12:00"})
	next := calculateNextRun("monthly", params)
	assert.Equal(t, 28, next.Day())
	assert.Equal(t, 12, next.Hour())
}
