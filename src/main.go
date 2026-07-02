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

	// Create table if it doesn't exist
	createTable := `
	CREATE TABLE IF NOT EXISTS blood_pressure (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		systolic INTEGER NOT NULL,
		diastolic INTEGER NOT NULL,
		recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		recorded_date DATE UNIQUE NOT NULL
	);
	`
	_, err = database.Exec(createTable)
	if err != nil {
		return nil, err
	}

	return database, nil
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "templates/index.html")
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

	// Record only for today
	today := time.Now().Format("2006-01-02")
	_, err = db.Exec(
		"INSERT INTO blood_pressure (systolic, diastolic, recorded_date) VALUES (?, ?, ?) ON CONFLICT(recorded_date) DO UPDATE SET systolic=excluded.systolic, diastolic=excluded.diastolic",
		systolic,
		diastolic,
		today,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to record BP: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Blood pressure recorded"})
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "success", "imported": imported})
}
