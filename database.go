package main

import (
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

// Main message metadata storage.
type MessageLog struct {
	UUID        string    `gorm:"primary_key" json:"uuid"`
	MessageID   string    `json:"message_id"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Subject     string    `json:"subject"`
	PlainText   bool      `json:"plain_text"`
	HTML        bool      `json:"html"`
	Attachments bool      `json:"attachments"`
	SpamScore   int       `json:"spam_score"`
	SourceIP    string    `default:"" json:"source_ip"`
	Size        int       `json:"size"`
	Received    time.Time `json:"received"`
	Status      string    `json:"status"`
}

// Database storage of message data.
type Messages struct {
	UUID    string `gorm:"primary_key"`
	Message []byte
}

// Syslog message storage.
type SysLogMessage struct {
	ID        int64 `gorm:"primary_key"`
	Hostname  string
	Timestamp time.Time
	Tag       string
	SID       string
	Content   string
}

// Map of syslog message ids to email message ids with information on email status.
type SysLogIDInfo struct {
	ID        int64 `gorm:"primary_key"`
	Hostname  string
	SID       string
	MessageID string
	Status    string
	Ignore    bool
}

// Configure the database and add tables/adjust tables to match structures above.
func initDB(db *gorm.DB) {
	db.LogMode(app.config.DBDebug)
	db.AutoMigrate(&MessageLog{})
	db.AutoMigrate(&Messages{})
	db.AutoMigrate(&SysLogMessage{})
	db.AutoMigrate(&SysLogIDInfo{})
}
