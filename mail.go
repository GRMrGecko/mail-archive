package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DusanKasan/parsemail"
	"github.com/google/uuid"
)

// When a new message is received, this function is called to store it.
func MailSaveMessage(remoteAddr string, from string, to string, r io.Reader) error {
	// We need the message body in bytes to save.
	b, err := ioutil.ReadAll(r)
	if err != nil { // If we can't read, we have an issue.
		return err
	}
	// The email parser expects an io.Reader, but as we already read the reader passed. We must make a new one.
	reader := bytes.NewReader(b)
	// Parse the email
	email, err := parsemail.Parse(reader)
	if err != nil {
		return err
	}
	// Generate a UUID for this message.
	UUID := uuid.New().String()

	// Save message body using database or file if configured.
	if app.config.MailPath == "db" {
		// Save to database.
		message := Messages{}
		message.UUID = UUID
		message.Message = b
		app.db.Create(&message)
	} else {
		// If the directory configured in the config does not exist... We must fail.
		if _, err := os.Stat(app.config.MailPath); err != nil {
			return fmt.Errorf("Mail directory does not exist: %s", app.config.MailPath)
		}
		// Create the file.
		messagePath := path.Join(app.config.MailPath, UUID)
		fp, err := os.Create(messagePath)
		if err != nil {
			return err
		}
		// Write to the file
		fp.Write(b)
		fp.Close()
	}

	// Create a message log entry with parsed email.
	messageEntry := MessageLog{}
	messageEntry.UUID = UUID
	messageEntry.MessageID = email.MessageID
	if len(email.From) <= 0 {
		messageEntry.From = from
	} else {
		messageEntry.From = email.From[0].Address
	}
	if len(email.To) <= 0 {
		messageEntry.To = to
	} else {
		messageEntry.To = email.To[0].Address
	}
	messageEntry.Subject = email.Subject

	messageEntry.PlainText = email.TextBody != ""
	messageEntry.HTML = email.HTMLBody != ""
	messageEntry.Attachments = len(email.Attachments) > 0

	// If a spam level header exists, parse the score.
	spamScore := email.Header.Get("X-SPAM-LEVEL")
	rxScore := regexp.MustCompile("Spam detection results:\\s+([0-9]+)")
	matches := rxScore.FindStringSubmatch(spamScore)
	if len(matches) == 2 {
		spamScoreI, err := strconv.Atoi(matches[1])
		if err != nil {
			spamScoreI = 0
		}
		messageEntry.SpamScore = spamScoreI
	}

	// Get the source IP from the earliest received header. Default to remote address which is sending the message.
	// As messages are likely forwarded to this server, the earliest received header is what we want here.
	messageEntry.SourceIP = remoteAddr
	// Regex to parse the received header with hostname/ip address.
	rxAddr := regexp.MustCompile("from ([A-Za-z0-9-.]+) \\(.* \\[([0-9a-fA-F.:]+)\\]\\)")
	// Loop through all entires of received headers.
	for _, header := range email.Header["Received"] {
		// Parse the header.
		matches := rxAddr.FindStringSubmatch(header)
		if len(matches) == 3 {
			// If we got an source IP from the header, save it.
			messageEntry.SourceIP = matches[1] + " (" + matches[2] + ")"
		}
	}

	messageEntry.Size = len(b)
	messageEntry.Received = time.Now()
	messageEntry.Status = "unknown" // We start as unknown and the status is updated by syslog.

	// Save the message entry.
	app.db.Create(&messageEntry)
	log.Printf("SMTP: Received message from %s (%d bytes)", messageEntry.From, messageEntry.Size)

	// Notify websocket subscribers of new message.
	app.httpServer.wsInterface.sendMessage("receivedNewMessage", messageEntry)

	// Update message count.
	app.messageCount++
	app.httpServer.wsInterface.sendMessage("updateMessageCount", app.messageCount)
	return nil
}

// Finds and outputs a reader for the message body based on UUID.
func MailGetMessageData(UUID string) (r io.Reader, err error) {
	// If we are configured to use the database for storage, then we should check if the UUID is in the database.
	// Otherwise, we check the path set to see if a file exists with the UUID.
	if app.config.MailPath == "db" {
		// Search database for message body by UUID.
		var message Messages
		app.db.Where("uuid = ?", UUID).First(&message)
		// If not found, we provide an error.
		if message.UUID == "" {
			err = fmt.Errorf(APINoMessage)
			return
		}
		// Create a reader for the message data.
		r = bytes.NewReader(message.Message)
	} else {
		// Verify that the UUID exists in the file system.
		if _, err = os.Stat(path.Join(app.config.MailPath, UUID)); err != nil {
			return
		}
		// If the file exists, we open it to return.
		r, err = os.Open(path.Join(app.config.MailPath, UUID))
	}
	// Return reader.
	return
}

// To try and make the syslog code light weight, this function was created
//  to update the status of messages to what was parsed in the syslog.
// This function will read an update queue map of syslog ids with updated statuses.
func RunSysLogMailUpdateQueue() {
	ticker := time.NewTicker(5 * time.Second)
	for _ = range ticker.C { // Every 5 seconds.
		// We want to keep track as to rather statuses were updated to notify subscribers.
		updated := false

		// Copy the update queue so that we can empty the main update queue.
		updateQueue := make(map[string]bool)
		for key, val := range app.sysLogMailUpdateQueue {
			updateQueue[key] = val
		}
		// We empty the main update queue as the syslog may have status changes during this run.
		// We want to ensure that those changes do not get lost.
		app.sysLogMailUpdateQueue = make(map[string]bool)

		// Loop the update queue.
		for sidHostname, _ := range updateQueue {
			// Update queue should contain syslog id and hostname separated by a colon.
			// We pair the syslog id with the hostname just incase different hosts use the same syslog id.
			s := strings.Split(sidHostname, ":")
			// If there is more or less than 2 parts, this is invalid.
			if len(s) != 2 {
				continue
			}
			sid := s[0]
			hostname := s[1]

			// Pull the syslog id information from the database.
			var match SysLogIDInfo
			app.db.Where("s_id = ? AND hostname = ?", sid, hostname).First(&match)
			// If nothing was returned, or this one is set to be ignored... We will stop here.
			// When a syslog id is ignored, it is likely due to it being either the main message received before sending out,
			//  or it is the message forwarded to Mail Archive which is not the main mail delivery status.
			if match.SID == "" || match.Ignore {
				continue
			}

			// Pull the message log entry matching the message id associated with the syslog id.
			var messageEntry MessageLog
			app.db.Where("message_id = ?", match.MessageID).First(&messageEntry)
			// If we found the message log entry, we can update the status to match our syslog id delivery status.
			if messageEntry.UUID != "" {
				messageEntry.Status = match.Status
				app.db.Save(&messageEntry)
				// As we updated a message log entry, we want to inform the subscribers that an update occurred.
				updated = true
			}
		}
		// If we updated the status of a message log entry, we need to inform subscribers connected to websocket.
		if updated {
			app.httpServer.wsInterface.sendMessage("messageStatusesUpdated", true)
		}
	}
}

// This function will run a database cleanup of old messages every 30 minutes.
func RunDatabaseCleanup() {
	ticker := time.NewTicker(30 * time.Minute)
	for _ = range ticker.C {
		// Get the oldest date we will allow at this point in time based on the configured maximum age.
		maxAge := time.Now().Add(app.config.MaxAge * time.Second * -1)

		// We want to just pull UUID and message id of the old messages to be cleaned up.
		type MessageIDs struct {
			UUID      string
			MessageID string
		}
		var messageIDs []MessageIDs
		app.db.Table("message_logs").Select("uuid,message_id").Where("received <= ?", maxAge).Scan(&messageIDs)

		// Loop through all found old messages to clean up the database.
		for _, message := range messageIDs {
			// Find syslog id information entries matching this message.
			var matches []SysLogIDInfo
			app.db.Where("message_id = ?", message.MessageID).Find(&matches)
			// With each found syslog id, we need to delete the syslog messages and the syslog id information.
			for _, match := range matches {
				app.db.Where("s_id = ? AND hostname = ?", match.SID, match.Hostname).Delete(SysLogMessage{})
				app.db.Delete(&match)
			}

			// Delete the message log entry for this message.
			app.db.Where("uuid = ?", message.UUID).Delete(MessageLog{})
			// Delete message data matching the UUID for the message.
			app.db.Where("uuid = ?", message.UUID).Delete(Messages{})
			// If the configured mail storage path is not the database, remove it from the file system.
			if app.config.MailPath != "db" {
				if _, err := os.Stat(path.Join(app.config.MailPath, message.UUID)); err == nil {
					os.Remove(path.Join(app.config.MailPath, message.UUID))
				}
			}
			// Update message count.
			app.messageCount--
		}

		// Send updated message count.
		app.httpServer.wsInterface.sendMessage("updateMessageCount", app.messageCount)
	}
}
