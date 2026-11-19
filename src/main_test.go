package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestCalculateLevel(t *testing.T) {
	tests := []struct {
		xp           int
		expectedLvl  int
		expectedNext int
	}{
		{xp: 0, expectedLvl: 1, expectedNext: 100},
		{xp: 50, expectedLvl: 1, expectedNext: 100},
		{xp: 100, expectedLvl: 2, expectedNext: 200},
		{xp: 299, expectedLvl: 2, expectedNext: 200},
		{xp: 300, expectedLvl: 3, expectedNext: 300},
		{xp: 600, expectedLvl: 4, expectedNext: 400},
	}

	for _, tt := range tests {
		lvl, xpInLevel, xpNeeded, pct := calculateLevel(tt.xp)
		if lvl != tt.expectedLvl {
			t.Errorf("For XP %d, expected level %d, got %d", tt.xp, tt.expectedLvl, lvl)
		}
		if xpNeeded != tt.expectedNext {
			t.Errorf("For XP %d, expected next level req %d, got %d", tt.xp, tt.expectedNext, xpNeeded)
		}
		if pct < 0 || pct > 100 {
			t.Errorf("For XP %d, progress pct %d out of bounds", tt.xp, pct)
		}
		if xpInLevel < 0 || xpInLevel >= xpNeeded {
			t.Errorf("For XP %d, xpInLevel %d invalid for xpNeeded %d", tt.xp, xpInLevel, xpNeeded)
		}
	}
}

func TestGetHealthZone(t *testing.T) {
	tests := []struct {
		systolic  int
		diastolic int
		expected  string
	}{
		{systolic: 115, diastolic: 75, expected: "Normal"},
		{systolic: 125, diastolic: 78, expected: "Elevated"},
		{systolic: 135, diastolic: 75, expected: "Stage 1"},
		{systolic: 120, diastolic: 85, expected: "Stage 1"},
		{systolic: 145, diastolic: 80, expected: "Stage 2"},
		{systolic: 130, diastolic: 95, expected: "Stage 2"},
		{systolic: 185, diastolic: 100, expected: "Hypertensive Crisis"},
		{systolic: 130, diastolic: 125, expected: "Hypertensive Crisis"},
	}

	for _, tt := range tests {
		zone := getHealthZone(tt.systolic, tt.diastolic)
		if zone.Name != tt.expected {
			t.Errorf("For %d/%d, expected %s, got %s", tt.systolic, tt.diastolic, tt.expected, zone.Name)
		}
	}
}

func TestDaysBetween(t *testing.T) {
	d1 := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)

	if diff := daysBetween(d1, d2); diff != 1 {
		t.Errorf("Expected 1 day between d1 and d2, got %d", diff)
	}
	if diff := daysBetween(d1, d3); diff != 5 {
		t.Errorf("Expected 5 days between d1 and d3, got %d", diff)
	}
	if diff := daysBetween(d1, d1); diff != 0 {
		t.Errorf("Expected 0 days between d1 and d1, got %d", diff)
	}
}

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test_bp_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp db: %v", err)
	}
	tmpFile.Close()

	os.Setenv("DB_PATH", tmpFile.Name())
	testDB, err := initDB()
	if err != nil {
		t.Fatalf("Failed to init test db: %v", err)
	}

	cleanup := func() {
		testDB.Close()
		os.Remove(tmpFile.Name())
	}
	return testDB, cleanup
}

func TestRecordBPAndStats(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	// Assign global db
	db = testDB

	// Initial stats should be empty
	stats, err := getUserStats(db)
	if err != nil {
		t.Fatalf("Failed to get initial stats: %v", err)
	}
	if stats.XP != 0 || stats.CurrentStreak != 0 || stats.Level != 1 {
		t.Errorf("Expected initial empty stats, got %+v", stats)
	}

	// 1. Record BP for today
	form := url.Values{}
	form.Set("systolic", "118")
	form.Set("diastolic", "76")

	req := httptest.NewRequest(http.MethodPost, "/api/bp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	recordBP(w, req)
	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	triggerHeader := resp.Header.Get("HX-Trigger")
	if !strings.Contains(triggerHeader, "bpUpdated") {
		t.Errorf("Expected HX-Trigger to contain bpUpdated, got %s", triggerHeader)
	}

	// Check updated stats
	stats, err = getUserStats(db)
	if err != nil {
		t.Fatalf("Failed to get updated stats: %v", err)
	}
	if stats.CurrentStreak != 1 {
		t.Errorf("Expected streak 1, got %d", stats.CurrentStreak)
	}
	if stats.XP != 50 {
		t.Errorf("Expected 50 XP, got %d", stats.XP)
	}
	if stats.LatestReading == nil {
		t.Fatalf("Expected LatestReading to be non-nil")
	}
	if stats.LatestReading.Zone.Name != "Normal" {
		t.Errorf("Expected zone Normal, got %s", stats.LatestReading.Zone.Name)
	}
	if stats.LatestReading.PulsePressure != 42 {
		t.Errorf("Expected pulse pressure 42, got %d", stats.LatestReading.PulsePressure)
	}

	// 2. Updating today's reading should update values without duplicate XP
	form.Set("systolic", "120")
	form.Set("diastolic", "78")
	req2 := httptest.NewRequest(http.MethodPost, "/api/bp", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()

	recordBP(w2, req2)
	stats2, _ := getUserStats(db)

	if stats2.XP != 50 {
		t.Errorf("Expected XP to remain 50 after same-day update, got %d", stats2.XP)
	}
	if stats2.CurrentStreak != 1 {
		t.Errorf("Expected streak to remain 1 after update, got %d", stats2.CurrentStreak)
	}
	if stats2.LatestReading.Systolic != 120 {
		t.Errorf("Expected systolic to update to 120, got %d", stats2.LatestReading.Systolic)
	}
}

func TestMultiDayStreakCalculation(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert readings for 3 consecutive days: 2 days ago, yesterday, today
	now := time.Now()
	day0 := now.AddDate(0, 0, -2).Format("2006-01-02")
	day1 := now.AddDate(0, 0, -1).Format("2006-01-02")
	day2 := now.Format("2006-01-02")

	_, err := testDB.Exec("INSERT INTO blood_pressure (systolic, diastolic, recorded_date) VALUES (118, 76, ?)", day0)
	if err != nil {
		t.Fatalf("Failed to insert day0: %v", err)
	}
	_, err = testDB.Exec("INSERT INTO blood_pressure (systolic, diastolic, recorded_date) VALUES (120, 78, ?)", day1)
	if err != nil {
		t.Fatalf("Failed to insert day1: %v", err)
	}
	_, err = testDB.Exec("INSERT INTO blood_pressure (systolic, diastolic, recorded_date) VALUES (115, 75, ?)", day2)
	if err != nil {
		t.Fatalf("Failed to insert day2: %v", err)
	}

	stats, err := getUserStats(testDB)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	// Day 1: 50 XP
	// Day 2: 50 + 10 = 60 XP
	// Day 3: 50 + 20 = 70 XP
	// Total: 180 XP -> Level 2
	if stats.CurrentStreak != 3 {
		t.Errorf("Expected current streak 3, got %d", stats.CurrentStreak)
	}
	if stats.LongestStreak != 3 {
		t.Errorf("Expected longest streak 3, got %d", stats.LongestStreak)
	}
	if stats.XP != 180 {
		t.Errorf("Expected 180 XP, got %d", stats.XP)
	}
	if stats.Level != 2 {
		t.Errorf("Expected Level 2, got %d", stats.Level)
	}
	if stats.XPInLevel != 80 {
		t.Errorf("Expected 80 XP in level 2, got %d", stats.XPInLevel)
	}
	if stats.XPNeeded != 200 {
		t.Errorf("Expected 200 XP needed for level 3, got %d", stats.XPNeeded)
	}
}
