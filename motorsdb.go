package main

import (
	"database/sql"
	"log"
	"strconv"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"

	srvConfig "github.com/CHESSComputing/golib/config"
)

// Safe to use concurrently
type SafeDb struct {
	db *sql.DB
	mu sync.Mutex
}

var MotorsDb SafeDb

type MotorRecord struct {
	DatasetId      string
	MotorMnes      []string
	MotorPositions []float64
}

func InitMotorsDb() {
	db, err := sql.Open("sqlite3", srvConfig.Config.SpecScans.DBFile)
	if err != nil {
		log.Fatal(err)
	}
	// Test db accessibility
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Pinged motors db")

	MotorsDb = SafeDb{db: db}
}

func InsertMotors(r MotorRecord) (int64, error) {
	// Insert the given motor record to the three tables that compose the static
	// motor positions database.
	log.Printf("Inserting motor record: %v", r)
	result, err := MotorsDb.db.Exec("INSERT INTO DID (did) VALUES (?)", r.DatasetId)
	if err != nil {
		log.Printf("Could not insert record to DID table; error: %v", err)
		return -1, err
	}
	dataset_id, err := result.LastInsertId()
	if err != nil {
		log.Printf("Could not get ID of new record in DID; error: %v", err)
		return dataset_id, err
	}
	var motor_id int64
	for i := 0; i < len(r.MotorMnes); i++ {
		result, err = MotorsDb.db.Exec("INSERT INTO MotorMnes (dataset_id, motor_mne) VALUES (?, ?)", dataset_id, r.MotorMnes[i])
		if err != nil {
			log.Printf("Could not insert record to MotorMnes table; error: %v", err)
			continue
		}
		motor_id, err = result.LastInsertId()
		if err != nil {
			log.Printf("Could not get ID of new record in MotorMnes; error: %v", err)
			continue
		}
		result, err = MotorsDb.db.Exec("INSERT INTO MotorPositions (motor_id, motor_position) VALUES (?, ?)", motor_id, r.MotorPositions[i])
		if err != nil {
			log.Printf("Could not insert record to MotorPositions table; error: %v", err)
		}
	}
	return dataset_id, nil
}

func QueryMotorPosition(mne string, pos float64) []MotorRecord {
	// Return a slice of complete motor position records for all the datasets
	// which included the given motor mnemonic and match at the given position.
	var motor_records []MotorRecord
	rows, err := MotorsDb.db.Query(`
SELECT D.did, group_concat(M.motor_mne), group_concat(P.motor_position)
FROM MotorPositions as P
JOIN MotorMnes AS M ON M.motor_id=P.motor_id
JOIN DID AS D ON D.dataset_id=M.dataset_id
WHERE D.dataset_id IN (
	SELECT D.dataset_id
	FROM MotorPositions as P
	JOIN MotorMnes AS M ON M.motor_id=P.motor_id
	JOIN DID AS D ON D.dataset_id=M.dataset_id
	WHERE M.motor_mne=? AND P.motor_position=?)
GROUP BY D.did;`, mne, pos)
	if err != nil {
		log.Printf("Could not query motor positions database; error: %v", err)
		return motor_records
	}
	return getMotorRecords(rows)
}

func GetMotorRecord(did string) (MotorRecord, error) {
	rows, err := MotorsDb.db.Query("SELECT D.did, group_concat(M.motor_mne), group_concat(P.motor_position) FROM MotorPositions as P JOIN MotorMnes AS M ON M.motor_id=P.motor_id JOIN DID AS D ON D.dataset_id=M.dataset_id WHERE D.did=? GROUP BY D.did", did)
	if err != nil {
		log.Printf("Could not query motor positions database; error: %v", err)
		return MotorRecord{}, err
	}
	return getMotorRecords(rows)[0], nil
}

func getMotorRecords(rows *sql.Rows) []MotorRecord {
	// Helper for parsing grouped results of sql query
	var motor_records []MotorRecord
	rows.Next()
	motor_record := getMotorRecord(rows)
	motor_records = append(motor_records, motor_record)
	for rows.Next() {
		motor_record := getMotorRecord(rows)
		motor_records = append(motor_records, motor_record)
	}
	return motor_records
}

func getMotorRecord(rows *sql.Rows) MotorRecord {
	// Helper for parsing grouped results of sql query at the current cursor position only
	motor_record := MotorRecord{}
	_motor_mnes, _motor_positions := "", ""
	err := rows.Scan(&motor_record.DatasetId, &_motor_mnes, &_motor_positions)
	if err != nil {
		log.Printf("Could not get a MotorRecord from a row of SQL results. error: %v", err)
		return motor_record
	}
	motor_record.MotorMnes = strings.Split(_motor_mnes, ",")
	motor_positions := make([]float64, 0, len(motor_record.MotorMnes))
	for _, position := range strings.Split(_motor_positions, ",") {
		position, _ := strconv.ParseFloat(position, 64)
		motor_positions = append(motor_positions, position)
	}
	motor_record.MotorPositions = motor_positions
	return motor_record
}
