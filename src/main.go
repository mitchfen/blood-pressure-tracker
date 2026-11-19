package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

type BloodPressure struct {
	ID         int       `json:"id"`
	Systolic   int       `json:"systolic"`
	Diastolic  int       `json:"diastolic"`
	RecordedAt time.Time `json:"recorded_at"`
}

type HealthZone struct {
	Name        string `json:"name"`
	Class       string `json:"class"`
	BgClass     string `json:"bg_class"`
	TextClass   string `json:"text_class"`
	BorderClass string `json:"border_class"`
	Description string `json:"description"`
}

type LatestBP struct {
	Systolic      int        `json:"systolic"`
	Diastolic     int        `json:"diastolic"`
	PulsePressure int        `json:"pulse_pressure"`
	RecordedDate  string     `json:"recorded_date"`
	RecordedAt    time.Time  `json:"recorded_at"`
	Zone          HealthZone `json:"zone"`
}

type UserStats struct {
	XP              int       `json:"xp"`
	Level           int       `json:"level"`
	LevelTitle      string    `json:"level_title"`
	CurrentStreak   int       `json:"current_streak"`
	LongestStreak   int       `json:"longest_streak"`
	XPInLevel       int       `json:"xp_in_level"`
	XPNeeded        int       `json:"xp_needed"`
	ProgressPercent int       `json:"progress_percent"`
	LastRecordedDate string   `json:"last_recorded_date"`
	LatestReading   *LatestBP `json:"latest_reading,omitempty"`
}

func main() {
	var err error
	db, err = initDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/api/bp", handleBP)
	http.HandleFunc("/api/bps", handleBPs)
	http.HandleFunc("/api/bps/export", handleExport)
	http.HandleFunc("/api/bps/import", handleImport)
	http.HandleFunc("/api/stats", handleStats)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	port := ":8080"
	log.Printf("Server starting on %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

func initDB() (*sql.DB, error) {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "bp.db"
	}
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Create tables if they don't exist
	schema := `
	CREATE TABLE IF NOT EXISTS blood_pressure (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		systolic INTEGER NOT NULL,
		diastolic INTEGER NOT NULL,
		recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		recorded_date DATE UNIQUE NOT NULL
	);
	CREATE TABLE IF NOT EXISTS user_stats (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		xp INTEGER NOT NULL DEFAULT 0,
		level INTEGER NOT NULL DEFAULT 1,
		current_streak INTEGER NOT NULL DEFAULT 0,
		longest_streak INTEGER NOT NULL DEFAULT 0,
		last_recorded_date DATE
	);
	`
	_, err = database.Exec(schema)
	if err != nil {
		return nil, err
	}

	// Calculate / sync user stats from existing blood pressure history
	if err := recalculateUserStats(database); err != nil {
		log.Printf("Notice: initial user stats recalculation: %v", err)
	}

	return database, nil
}

func getHealthZone(systolic, diastolic int) HealthZone {
	if systolic > 180 || diastolic > 120 {
		return HealthZone{
			Name:        "Hypertensive Crisis",
			Class:       "crisis",
			BgClass:     "bg-rose-900",
			TextClass:   "text-rose-100",
			BorderClass: "border-rose-600",
			Description: "Emergency care may be needed",
		}
	}
	if systolic >= 140 || diastolic >= 90 {
		return HealthZone{
			Name:        "Stage 2",
			Class:       "stage2",
			BgClass:     "bg-rose-950/80",
			TextClass:   "text-rose-300",
			BorderClass: "border-rose-800/80",
			Description: "Hypertension Stage 2",
		}
	}
	if (systolic >= 130 && systolic <= 139) || (diastolic >= 80 && diastolic <= 89) {
		return HealthZone{
			Name:        "Stage 1",
			Class:       "stage1",
			BgClass:     "bg-orange-950/80",
			TextClass:   "text-orange-300",
			BorderClass: "border-orange-800/80",
			Description: "Hypertension Stage 1",
		}
	}
	if (systolic >= 120 && systolic <= 129) && diastolic < 80 {
		return HealthZone{
			Name:        "Elevated",
			Class:       "elevated",
			BgClass:     "bg-amber-950/80",
			TextClass:   "text-amber-300",
			BorderClass: "border-amber-800/80",
			Description: "Elevated Blood Pressure",
		}
	}
	return HealthZone{
		Name:        "Normal",
		Class:       "normal",
		BgClass:     "bg-emerald-950/80",
		TextClass:   "text-emerald-300",
		BorderClass: "border-emerald-800/80",
		Description: "Optimal Blood Pressure",
	}
}

func getLevelTitle(level int) string {
	switch {
	case level >= 25:
		return "Master of Flow"
	case level >= 20:
		return "Cardio Titan"
	case level >= 15:
		return "Vascular Legend"
	case level >= 10:
		return "Arterial Champion"
	case level >= 7:
		return "Cardio Knight"
	case level >= 5:
		return "Arterial Warden"
	case level >= 3:
		return "Pulse Keeper"
	case level >= 2:
		return "Rhythm Scout"
	default:
		return "Heart Initiate"
	}
}

func calculateLevel(totalXP int) (level int, xpInLevel int, xpNeeded int, progressPct int) {
	level = 1
	for {
		threshold := 50 * (level + 1) * level
		if totalXP >= threshold {
			level++
		} else {
			break
		}
	}
	baseXP := 50 * level * (level - 1)
	nextLevelXP := 50 * (level + 1) * level
	xpInLevel = totalXP - baseXP
	xpNeeded = nextLevelXP - baseXP
	if xpNeeded > 0 {
		progressPct = (xpInLevel * 100) / xpNeeded
	}
	return
}

func daysBetween(a, b time.Time) int {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	t1 := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
	return int(t2.Sub(t1).Hours() / 24)
}

func recalculateUserStats(database *sql.DB) error {
	rows, err := database.Query("SELECT recorded_date FROM blood_pressure ORDER BY recorded_date ASC")
	if err != nil {
		return err
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var dateStr string
		if err := rows.Scan(&dateStr); err != nil {
			continue
		}
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}
		t, err := time.Parse("2006-01-02", dateStr)
		if err == nil {
			dates = append(dates, t)
		}
	}

	totalXP := 0
	streak := 0
	longestStreak := 0
	var prevDate time.Time
	hasPrev := false

	for _, d := range dates {
		if !hasPrev {
			streak = 1
			totalXP += 50
			prevDate = d
			hasPrev = true
		} else {
			diff := daysBetween(prevDate, d)
			if diff == 0 {
				continue
			} else if diff == 1 {
				streak++
				bonus := (streak - 1) * 10
				if bonus > 50 {
					bonus = 50
				}
				totalXP += (50 + bonus)
			} else {
				streak = 1
				totalXP += 50
			}
			prevDate = d
		}
		if streak > longestStreak {
			longestStreak = streak
		}
	}

	currentStreak := 0
	var lastDateStr string
	if hasPrev {
		lastDateStr = prevDate.Format("2006-01-02")
		today := time.Now()
		diff := daysBetween(prevDate, today)
		if diff <= 1 {
			currentStreak = streak
		} else {
			currentStreak = 0
		}
	}

	level, _, _, _ := calculateLevel(totalXP)

	upsertQuery := `
	INSERT INTO user_stats (id, xp, level, current_streak, longest_streak, last_recorded_date)
	VALUES (1, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		xp = excluded.xp,
		level = excluded.level,
		current_streak = excluded.current_streak,
		longest_streak = excluded.longest_streak,
		last_recorded_date = excluded.last_recorded_date;
	`
	_, err = database.Exec(upsertQuery, totalXP, level, currentStreak, longestStreak, lastDateStr)
	return err
}

func getUserStats(database *sql.DB) (UserStats, error) {
	_ = recalculateUserStats(database)

	var stats UserStats
	var lastDate sql.NullString
	err := database.QueryRow("SELECT xp, level, current_streak, longest_streak, last_recorded_date FROM user_stats WHERE id = 1").
		Scan(&stats.XP, &stats.Level, &stats.CurrentStreak, &stats.LongestStreak, &lastDate)
	if err != nil {
		if err == sql.ErrNoRows {
			stats.Level = 1
			stats.LevelTitle = getLevelTitle(1)
			stats.XPNeeded = 100
			return stats, nil
		}
		return stats, err
	}
	if lastDate.Valid {
		stats.LastRecordedDate = lastDate.String
	}

	stats.Level, stats.XPInLevel, stats.XPNeeded, stats.ProgressPercent = calculateLevel(stats.XP)
	stats.LevelTitle = getLevelTitle(stats.Level)

	// Fetch latest reading
	var systolic, diastolic int
	var recDate string
	var recAt time.Time
	row := database.QueryRow("SELECT systolic, diastolic, recorded_date, recorded_at FROM blood_pressure ORDER BY recorded_date DESC, recorded_at DESC LIMIT 1")
	if err := row.Scan(&systolic, &diastolic, &recDate, &recAt); err == nil {
		stats.LatestReading = &LatestBP{
			Systolic:      systolic,
			Diastolic:     diastolic,
			PulsePressure: systolic - diastolic,
			RecordedDate:  recDate,
			RecordedAt:    recAt,
			Zone:          getHealthZone(systolic, diastolic),
		}
	}

	return stats, nil
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "templates/index.html")
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := getUserStats(db)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch user stats: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleBP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		recordBP(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func recordBP(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	systolicStr := r.FormValue("systolic")
	diastolicStr := r.FormValue("diastolic")

	if systolicStr == "" || diastolicStr == "" {
		http.Error(w, "Both systolic and diastolic are required", http.StatusBadRequest)
		return
	}

	var systolic, diastolic int
	_, err = fmt.Sscanf(systolicStr, "%d", &systolic)
	if err != nil {
		http.Error(w, "Invalid systolic format", http.StatusBadRequest)
		return
	}
	_, err = fmt.Sscanf(diastolicStr, "%d", &diastolic)
	if err != nil {
		http.Error(w, "Invalid diastolic format", http.StatusBadRequest)
		return
	}

	today := time.Now().Format("2006-01-02")

	// Check if today was already recorded before updating
	var existingID int
	errExisting := db.QueryRow("SELECT id FROM blood_pressure WHERE recorded_date = ?", today).Scan(&existingID)
	isUpdate := (errExisting == nil)

	oldStats, _ := getUserStats(db)

	_, err = db.Exec(
		"INSERT INTO blood_pressure (systolic, diastolic, recorded_date, recorded_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP) ON CONFLICT(recorded_date) DO UPDATE SET systolic=excluded.systolic, diastolic=excluded.diastolic, recorded_at=CURRENT_TIMESTAMP",
		systolic,
		diastolic,
		today,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to record BP: %v", err), http.StatusInternalServerError)
		return
	}

	newStats, _ := getUserStats(db)

	xpGained := newStats.XP - oldStats.XP
	leveledUp := newStats.Level > oldStats.Level
	zone := getHealthZone(systolic, diastolic)
	pulsePressure := systolic - diastolic

	triggerData, _ := json.Marshal(map[string]interface{}{
		"xpGained":      xpGained,
		"leveledUp":     leveledUp,
		"newLevel":      newStats.Level,
		"currentStreak": newStats.CurrentStreak,
		"isUpdate":      isUpdate,
		"zone":          zone.Name,
	})

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"bpUpdated": %s}`, string(triggerData)))
	w.Header().Set("Content-Type", "text/html")

	var htmlResponse string
	if isUpdate {
		htmlResponse = fmt.Sprintf(`
		<div class="message success flex flex-col items-center gap-1.5 p-3.5 rounded-xl border border-emerald-200 bg-emerald-50 text-slate-800 shadow-sm animate-fadeIn">
			<div class="flex flex-wrap items-center justify-center gap-2 font-semibold">
				<span>✓ Today's reading updated: <strong>%d/%d</strong> mmHg</span>
				<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-bold %s %s border %s">%s</span>
			</div>
			<div class="text-xs text-slate-600 flex items-center gap-2">
				<span>Pulse Pressure: <strong>%d</strong> mmHg</span>
				<span>&bull;</span>
				<span>🔥 %d Day Streak Active</span>
			</div>
		</div>`,
			systolic, diastolic,
			zone.BgClass, zone.TextClass, zone.BorderClass, zone.Name,
			pulsePressure, newStats.CurrentStreak,
		)
	} else {
		levelUpBadge := ""
		if leveledUp {
			levelUpBadge = fmt.Sprintf(`<span>&bull;</span> <span class="text-rose-600 font-extrabold animate-pulse">🎉 LEVEL %d REACHED!</span>`, newStats.Level)
		}
		htmlResponse = fmt.Sprintf(`
		<div class="message success flex flex-col items-center gap-1.5 p-3.5 rounded-xl border border-emerald-200 bg-emerald-50 text-slate-800 shadow-sm animate-bounce-short">
			<div class="flex flex-wrap items-center justify-center gap-2 font-semibold">
				<span>✓ Blood pressure recorded: <strong>%d/%d</strong> mmHg</span>
				<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-bold %s %s border %s">%s</span>
			</div>
			<div class="flex flex-wrap items-center justify-center gap-2 text-xs font-semibold text-emerald-800 bg-white/70 px-3 py-1 rounded-full border border-emerald-200">
				<span>+%d XP</span>
				<span>&bull;</span>
				<span>🔥 %d Day Streak!</span>
				%s
			</div>
		</div>`,
			systolic, diastolic,
			zone.BgClass, zone.TextClass, zone.BorderClass, zone.Name,
			xpGained, newStats.CurrentStreak, levelUpBadge,
		)
	}

	w.Write([]byte(htmlResponse))
}

func handleBPs(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, systolic, diastolic, recorded_at FROM blood_pressure ORDER BY recorded_at")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch BP data: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	bps := []BloodPressure{}
	for rows.Next() {
		var bp BloodPressure
		err := rows.Scan(&bp.ID, &bp.Systolic, &bp.Diastolic, &bp.RecordedAt)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to scan BP: %v", err), http.StatusInternalServerError)
			return
		}
		bps = append(bps, bp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bps)
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT systolic, diastolic, recorded_date FROM blood_pressure ORDER BY recorded_date")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch BP data: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=blood_pressure.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	writer.Write([]string{"date", "systolic", "diastolic"})
	for rows.Next() {
		var systolic, diastolic int
		var date string
		if err := rows.Scan(&systolic, &diastolic, &date); err != nil {
			continue
		}
		writer.Write([]string{date, strconv.Itoa(systolic), strconv.Itoa(diastolic)})
	}
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(1 << 20) // 1MB max
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Skip header row
	if _, err := reader.Read(); err != nil {
		http.Error(w, "Failed to read CSV header", http.StatusBadRequest)
		return
	}

	// Wipe existing data before importing
	if _, err := db.Exec("DELETE FROM blood_pressure"); err != nil {
		http.Error(w, "Failed to clear existing data", http.StatusInternalServerError)
		return
	}

	imported := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) < 3 {
			continue
		}

		date := record[0]
		systolic, err1 := strconv.Atoi(record[1])
		diastolic, err2 := strconv.Atoi(record[2])
		if err1 != nil || err2 != nil {
			continue
		}

		_, err = db.Exec(
			"INSERT INTO blood_pressure (systolic, diastolic, recorded_date, recorded_at) VALUES (?, ?, ?, ?) ON CONFLICT(recorded_date) DO UPDATE SET systolic=excluded.systolic, diastolic=excluded.diastolic",
			systolic, diastolic, date, date,
		)
		if err == nil {
			imported++
		}
	}

	// Recalculate stats from newly imported records
	_ = recalculateUserStats(db)

	w.Header().Set("HX-Trigger", "bpUpdated")
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`<div class="message success">✓ Imported %d records & recalculated stats</div>`, imported)))
}
