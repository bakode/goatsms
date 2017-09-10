package db

import (
	"database/sql"
	"time"

	// cos its cgo...
	_ "github.com/mattn/go-sqlite3"
)

// DB is a wrapper around sql.DB.
type DB struct {
	*sql.DB
}

// SMSStatus indicates the state of the SMS.
type SMSStatus int

const (
	// SMSPending indicates the SMS is waiting to be sent.
	SMSPending SMSStatus = iota // 0
	// SMSSent indicates the SMS was successfully sent.
	SMSSent // 1
	// SMSErrored indicates the SMS was attempted to be sent but failed.
	SMSErrored // 2
	// SMSCanceled indicates the SMS was canceled prior to being sent.
	SMSCanceled // 3
)

// SMS represents an SMS, as stored in the db.
type SMS struct {
	UUID      string    `json:"uuid"`
	Mobile    string    `json:"mobile"`
	Body      string    `json:"body"`
	Status    SMSStatus `json:"status"`
	Retries   int       `json:"retries"`
	Device    string    `json:"device"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// SMSRetryLimit specifies the number of attempts to send an SMS before
// marking it as SMSErrored.
//TODO: should be configurable (in the DB??  Per modem?  Modems in the DB??)
const SMSRetryLimit = 3

const schemaVersion string = "goatsms v1"

// New creates a database client.
// If it does not already exist then it is created and initialised.
// If it does exist then  it checks that it has the correct schema version.
func New(driver, dbname string) (*DB, error) {
	init := true
	sqldb, err := sql.Open(driver, dbname)
	if err != nil {
		return nil, err
	}
	if rows, err := sqldb.Query("SELECT version FROM schema_version"); err == nil {
		if rows.Next() {
			var version string
			if err = rows.Scan(&version); err == nil {
				if version == schemaVersion {
					init = false
				}
			}
		}
		rows.Close()
	}
	db := &DB{sqldb}
	if init {
		if err := db.init(); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

// init initialises the database, creating tables and setting the schema version.
func (db *DB) init() error {
	cmds := []string{
		`CREATE TABLE messages (
	                id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	                uuid char(32) UNIQUE NOT NULL,
	                message char(160)   NOT NULL,
	                mobile   char(15)    NOT NULL,
	                status  INTEGER DEFAULT 0,
	                retries INTEGER DEFAULT 0,
	                device string NULL,
	                created_at TIMESTAMP default CURRENT_TIMESTAMP,
	                updated_at TIMESTAMP
	            );`,
		"CREATE INDEX IF NOT EXISTS messages_status ON messages (status)",
		`CREATE TABLE schema_version (
		version char(16)   NOT NULL,
		created_at TIMESTAMP default CURRENT_TIMESTAMP
		);`,
		"INSERT INTO schema_version(version) VALUES('" + schemaVersion + "')",
	}
	for _, cmd := range cmds {
		_, err := db.Exec(cmd, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

// InsertMessage inserts an SMS into the database.
func (db *DB) InsertMessage(sms SMS) error {
	_, err := db.Exec("INSERT INTO messages(uuid, message, mobile) VALUES(?, ?, ?)", sms.UUID, sms.Body, sms.Mobile)
	return err
}

// UpdateMessageStatus updates the mutable fields of the SMS.
func (db *DB) UpdateMessageStatus(sms SMS) error {
	_, err := db.Exec("UPDATE messages SET status=?, retries=?, device=?, updated_at=DATETIME('now') WHERE uuid=?", sms.Status, sms.Retries, sms.Device, sms.UUID)
	return err
}

// GetPendingMessages gets the set of SMSs waiting to be sent.
func (db *DB) GetPendingMessages(limit int) ([]SMS, error) {
	rows, err := db.Query("SELECT uuid, message, mobile, status, retries FROM messages WHERE status=? LIMIT ?", SMSPending, limit)
	if err != nil {
		return nil, err
	}
	var messages []SMS
	for rows.Next() {
		sms := SMS{}
		rows.Scan(&sms.UUID, &sms.Body, &sms.Mobile, &sms.Status, &sms.Retries)
		messages = append(messages, sms)
	}
	rows.Close()
	return messages, nil
}

// GetMessages gets the set of SMSs corresponding to the filter.
// Expecting filter as empty string or WHERE clauses,
// simply appended to the query to get desired set from the database
func (db *DB) GetMessages(filter string) ([]SMS, error) {
	query := "SELECT uuid, message, mobile, status, retries, device, created_at, updated_at FROM messages " + filter
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	var messages []SMS
	for rows.Next() {
		sms := SMS{}
		rows.Scan(&sms.UUID, &sms.Body, &sms.Mobile, &sms.Status, &sms.Retries, &sms.Device, &sms.CreatedAt, &sms.UpdatedAt)
		messages = append(messages, sms)
	}
	rows.Close()
	return messages, nil
}

// GetLast7DaysMessageCount determines the number of SMSs added on each of the
// past 7 days.
func (db *DB) GetLast7DaysMessageCount() (map[string]int, error) {
	now := time.Now()
	lastWeekDate := time.Date(now.Year(), now.Month(), now.Day()-7, 1, 0, 0, 0, time.UTC)
	lastWeek := lastWeekDate.Format("2006-01-02")
	rows, err := db.Query(`SELECT strftime('%Y-%m-%d', created_at) as datestamp,
    COUNT(id) as messagecount FROM messages WHERE datestamp > '` + lastWeek + `'
    GROUP BY datestamp`)
	if err != nil {
		return nil, err
	}
	dayCount := make(map[string]int, 7)
	var day string
	var count int
	for rows.Next() {
		rows.Scan(&day, &count)
		dayCount[day] = count
	}
	rows.Close()
	return dayCount, nil
}

// GetStatusSummary determines the number of SMSs in each state.
func (db *DB) GetStatusSummary() ([]int, error) {
	rows, err := db.Query(`SELECT status, COUNT(id) as messagecount
    FROM messages GROUP BY status ORDER BY status`)
	if err != nil {
		return nil, err
	}
	var status, count int
	statusSummary := make([]int, 4)
	for rows.Next() {
		rows.Scan(&status, &count)
		statusSummary[status] = count
	}
	rows.Close()
	return statusSummary, nil
}
