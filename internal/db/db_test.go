/*
  Test suite for db package.
*/
package db

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	os.Remove("testdb")
	defer os.Remove("testdb")

	// new
	db, err := New("sqlite3", "testdb")
	if err != nil {
		t.Error("unexpected error:", err)
	}
	db.Close()

	// bad db type
	db, err = New("mysql", "testdb")
	if err == nil {
		t.Error("unexpected success")
		db.Close()
	}

	// existing - ok
	db, err = New("sqlite3", "testdb")
	if err != nil {
		t.Error("unexpected error:", err)
	}
	db.Close()

	// existing - bad access - read only

	// existing - bad schema
	db, err = New("sqlite3", "db_test.go")
	if err == nil {
		t.Fatal("unexpected success")
		db.Close()
	}
}

func TestInsertMessage(t *testing.T) {
	db := setup(t)
	defer teardown(db)

	// new
	smss := []SMS{
		SMS{UUID: "one", Mobile: "+1", Body: "a message"},
		SMS{UUID: "two", Mobile: "+2", Body: "another message"},
	}
	for _, sms := range smss {
		if err := db.InsertMessage(sms); err != nil {
			t.Error("unexpected error:", err)
		}
	}

	// existing
	if err := db.InsertMessage(smss[0]); err == nil {
		t.Error("unexpected success")
	}

	// check messages have been inserted.
	expected := map[string]SMS{}
	for _, sms := range smss {
		expected[sms.UUID] = sms
	}
	rows, err := db.Query("SELECT uuid,mobile,message FROM messages", nil)
	if err != nil {
		t.Error("unexpected error:", err)
	}
	for rows.Next() {
		var uuid, mobile, body string
		rows.Scan(&uuid, &mobile, &body)
		if expected[uuid].UUID != uuid {
			t.Errorf("expected uuid %s but got %s", expected[uuid].UUID, uuid)
		}
		if expected[uuid].Mobile != mobile {
			t.Errorf("expected mobile %s but got %s", expected[uuid].Mobile, mobile)
		}
		if expected[uuid].Body != body {
			t.Errorf("expected body %s but got %s", expected[uuid].Body, body)
		}
		delete(expected, uuid)
	}
}

func TestUpdateMessageStatus(t *testing.T) {

	db := setup(t)
	defer teardown(db)

	sms := SMS{UUID: "two", Mobile: "+2", Body: "another message"}
	if err := db.InsertMessage(sms); err != nil {
		t.Error("unexpected error:", err)
	}

	// existing
	sms.Status = SMSCanceled
	sms.Retries = 5
	sms.Device = "phone"
	if err := db.UpdateMessageStatus(sms); err != nil {
		t.Error("unexpected error:", err)
	}

	// non-existent (does nothing)
	nosms := SMS{UUID: "one", Mobile: "+1", Body: "a message", Status: SMSSent, Retries: 2, Device: "cell"}
	if err := db.UpdateMessageStatus(nosms); err != nil {
		t.Error("unexpected error:", err)
	}

	// check messages have been updated.
	expected := map[string]SMS{"two": sms}
	rows, err := db.Query("SELECT uuid,status,retries,device FROM messages", nil)
	if err != nil {
		t.Error("unexpected error:", err)
	}
	for rows.Next() {
		var uuid, device string
		var status SMSStatus
		var retries int
		rows.Scan(&uuid, &status, &retries, &device)
		if expected[uuid].UUID != uuid {
			t.Errorf("expected uuid %s but got %s", expected[uuid].UUID, uuid)
		}
		if expected[uuid].Status != status {
			t.Errorf("expected status %d but got %d", expected[uuid].Status, status)
		}
		if expected[uuid].Retries != retries {
			t.Errorf("expected retries %d but got %d", expected[uuid].Retries, retries)
		}
		if expected[uuid].Device != device {
			t.Errorf("expected device %s but got %s", expected[uuid].Device, device)
		}
		delete(expected, uuid)
	}
}

func TestGetPendingMessages(t *testing.T) {
	db := setup2(t)
	defer teardown(db)

	// limit
	smss, err := db.GetPendingMessages(10)
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if len(smss) != 10 {
		t.Errorf("got %d SMSs, expected 10", len(smss))
	}
	for _, s := range smss {
		if s.Status != SMSPending {
			t.Errorf("unexpected status %d in sms %s", s.Status, s.UUID)
		}
	}

	// less than limit
	smss, err = db.GetPendingMessages(100)
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if len(smss) != 37 {
		t.Errorf("got %d SMSs, expected 37", len(smss))
	}
	for _, s := range smss {
		if s.Status != SMSPending {
			t.Errorf("unexpected status %d in sms %s", s.Status, s.UUID)
		}
	}

	// db error
	db.Close()
	smss, err = db.GetPendingMessages(100)
	if err == nil {
		t.Error("unexpected success")
	}
	if len(smss) != 0 {
		t.Error("unexpected result:", smss)
	}
}

func TestGetMessages(t *testing.T) {
	db := setup2(t)
	defer teardown(db)

	// unfiltered
	smss, err := db.GetMessages("")
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if len(smss) != 100 {
		t.Errorf("got %d SMSs, expected 100", len(smss))
	}

	// filtered
	smss, err = db.GetMessages("WHERE status=1")
	if err != nil {
		t.Error("unexpected error:", err)
	}
	if len(smss) != 28 {
		t.Errorf("got %d SMSs, expected 28", len(smss))
	}
	for _, s := range smss {
		if s.Status != SMSSent {
			t.Errorf("unexpected status %d in sms %s", s.Status, s.UUID)
		}
	}

	// bad sql
	smss, err = db.GetMessages("WHERE")
	if err == nil {
		t.Error("unexpected success")
	}
	if len(smss) > 0 {
		t.Error("unexpected smss:", smss)
	}

}

func TestGetLast7DaysMessageCount(t *testing.T) {
	db := setup2(t)
	defer teardown(db)

	result, err := db.GetLast7DaysMessageCount()
	if err != nil {
		t.Error("unexpected error:", err)
	}
	// based on distro in setup2
	expected := []int{12, 8, 6, 5, 4, 4, 3}
	now := time.Now()
	for d := 0; d < 7; d++ {
		dayDate := time.Date(now.Year(), now.Month(), now.Day()-d, 1, 0, 0, 0, time.UTC)
		day := dayDate.Format("2006-01-02")
		if expected[d] != result[day] {
			t.Errorf("unexpected %s to have %d but got %d", day, expected[d], result[day])
		}
	}

	// db error
	db.Close()
	result, err = db.GetLast7DaysMessageCount()
	if err == nil {
		t.Error("unexpected success")
	}
	if result != nil {
		t.Error("unexpected result:", result)
	}
}

func TestGetStatusSummary(t *testing.T) {
	db := setup2(t)
	defer teardown(db)

	summary, err := db.GetStatusSummary()
	if err != nil {
		t.Error("unexpected error:", err)
	}
	expected := []int{37, 28, 14, 21}
	if len(summary) != len(expected) {
		t.Fatalf("expected %v but got %v", expected, summary)
	}
	for idx, v := range expected {
		if summary[idx] != v {
			t.Fatalf("expected %v but got %v", expected, summary)
		}
	}

	// db error
	db.Close()
	summary, err = db.GetStatusSummary()
	if err == nil {
		t.Error("unexpected success")
	}
	if summary != nil {
		t.Error("unexpected result:", summary)
	}
}

func setup(t *testing.T) *DB {
	db, err := New("sqlite3", "testdb")
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	return db
}

func setup2(t *testing.T) *DB {
	db := setup(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal("Begin err:", err)
	}
	stmt, err := tx.Prepare("INSERT INTO messages(uuid,message,mobile,status,retries,device,created_at) VALUES(?,?,?,?,?,?,?)")
	if err != nil {
		t.Fatal("Prepare err:", err)
	}
	now := time.Now()
	// This creates status distribution [37 28 14 21]
	for idx := 0; idx < 100; idx++ {
		createdDate := time.Date(now.Year(), now.Month(), now.Day(), 11-(idx*idx/11), 0, 0, 0, time.UTC)
		created := createdDate.Format("2006-01-02")
		stmt.Exec(
			fmt.Sprintf("i%04d", idx),
			fmt.Sprintf("message%03d", idx),
			fmt.Sprintf("+1%04d", idx),
			(idx*idx/7)%4,
			idx,
			"cell",
			created)
	}
	tx.Commit()
	stmt.Close()
	return db
}

func teardown(db *DB) {
	db.Close()
	os.Remove("testdb")
}
