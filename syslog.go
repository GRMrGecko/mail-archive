package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"gopkg.in/mcuadros/go-syslog.v2"
)

// Message connection information for keeping track of when a
//  disconnection happens and properly associate the log messages.
type SysLogMailConnection struct {
	sourceAddr   string
	sid          string
	started      time.Time
	shouldIgnore bool
}

// Buffer of log messages not yet associated with an email queue id (syslog id)
//  and information of connections made with their associated syslog id.
type SysLogBuffer struct {
	sourceAddr        string
	logMessages       []map[string]interface{}
	activeConnections []SysLogMailConnection
}

var sysLogBuffer *SysLogBuffer

// If the buffer has a new connection, we associate it with the next received syslog id.
func SysLogCheckIfNewConnection(sid string) {
	if sysLogBuffer.sourceAddr != "" {
		// Save the new connection to the active connections buffer.
		newConnection := SysLogMailConnection{}
		newConnection.started = time.Now()
		newConnection.sourceAddr = sysLogBuffer.sourceAddr
		newConnection.sid = sid
		sysLogBuffer.activeConnections = append(sysLogBuffer.activeConnections, newConnection)

		// Any log messages buffered for the new connection are now associated to this syslog id.
		for _, logMessage := range sysLogBuffer.logMessages {
			SysLogStoreMessage(logMessage, sid)
		}

		// We can reset the buffer for the next new connection.
		sysLogBuffer.logMessages = nil
		sysLogBuffer.sourceAddr = ""
	}
}

// Store a syslog message to the database.
func SysLogStoreMessage(logMessage map[string]interface{}, sid string) {
	content := logMessage["content"].(string)
	hostname := logMessage["hostname"].(string)

	// Check to see if the message received matches any ignore strings set.
	for _, ignore := range app.config.SysLogIgnoreContaining {
		if strings.Contains(content, ignore) {
			// If we match, look in the database for syslog id information entries.
			var match SysLogIDInfo
			app.db.Where("s_id = ? AND hostname = ?", sid, hostname).First(&match)
			if match.SID != "" {
				// If we found the syslog id information, we can update it to ignore.
				match.Ignore = true
				app.db.Save(&match)
			}
		}
	}

	// Below we check to see if the message changes the delivery status of the message.
	if strings.Contains(content, "quarantine") {
		// If this message queue id was quarantined, find the database entry and update.
		var match SysLogIDInfo
		app.db.Where("s_id = ? AND hostname = ?", sid, hostname).First(&match)
		if match.SID != "" {
			match.Status = "quarantined"
			// As the queue id could be the original message which could be ignored,
			//  we want to not ignore this message as a quarantine means another message queue id
			//  was never generated for the delivery of the message.
			match.Ignore = false
			app.db.Save(&match)
			log.Println("Syslog:", match.SID, match.Status)
			// The status was updated, so we can save to the queue for procoessing.
			app.sysLogMailUpdateQueue[sid+":"+hostname] = true
		}
	} else if strings.Contains(content, "status=sent") {
		// If this message queue id was sent, we update the database.
		var match SysLogIDInfo
		app.db.Where("s_id = ? AND hostname = ?", sid, hostname).First(&match)
		if match.SID != "" {
			match.Status = "sent"
			app.db.Save(&match)
			log.Println("Syslog:", match.SID, match.Status)
			// The status was updated, so we can save to the queue for procoessing.
			app.sysLogMailUpdateQueue[sid+":"+hostname] = true
		}
	} else if strings.Contains(content, "250 2.5.0 OK") {
		// If this message queue id was sent, we update the database.
		var match SysLogIDInfo
		app.db.Where("s_id = ? AND hostname = ?", sid, hostname).First(&match)
		// For 250 status, only update if not quarantined.
		if match.SID != "" && match.Status != "quarantined" {
			match.Status = "sent"
			app.db.Save(&match)
			log.Println("Syslog:", match.SID, match.Status)
			// The status was updated, so we can save to the queue for procoessing.
			app.sysLogMailUpdateQueue[sid+":"+hostname] = true
		}
	} else if strings.Contains(content, "status=deferred") {
		// If this message queue id was deferred, we update the database.
		var match SysLogIDInfo
		app.db.Where("s_id = ? AND hostname = ?", sid, hostname).First(&match)
		if match.SID != "" {
			match.Status = "deferred"
			app.db.Save(&match)
			log.Println("Syslog:", match.SID, match.Status)
			// The status was updated, so we can save to the queue for procoessing.
			app.sysLogMailUpdateQueue[sid+":"+hostname] = true
		}
	} else if strings.Contains(content, "status=bounced") {
		// If this message queue id was bounced, we update the database.
		var match SysLogIDInfo
		app.db.Where("s_id = ? AND hostname = ?", sid, hostname).First(&match)
		if match.SID != "" {
			match.Status = "bounced"
			app.db.Save(&match)
			log.Println("Syslog:", match.SID, match.Status)
			// The status was updated, so we can save to the queue for procoessing.
			app.sysLogMailUpdateQueue[sid+":"+hostname] = true
		}
	}

	// Save the message to the database.
	log := SysLogMessage{}
	log.Hostname = hostname
	log.Timestamp = logMessage["timestamp"].(time.Time)
	log.Tag = logMessage["tag"].(string)
	log.SID = sid
	log.Content = content
	app.db.Create(&log)
}

// As the syslog server sends messages, we parse them.
func SysLogRunner(channel syslog.LogPartsChannel) {
	// Below are all regular expressions used to match message data.

	// Daemon tags we accept messages from.
	rxMailMessage := regexp.MustCompile("(?i)postfix|exim|smtp-filter")
	// New connection messages.
	rxConnection := regexp.MustCompile("^connect from (.*\\[[0-9A-Fa-f:.]+\\])")
	// SMTP disconnection message.
	rxDisconnect := regexp.MustCompile("^disconnect from (.*\\[[0-9A-Fa-f:.]+\\])")
	// Message queue id.
	rxMailID := regexp.MustCompile("^([A-Za-z0-9]+): ")
	// End of message ok with message queue id.
	rxMailIDOk := regexp.MustCompile("OK \\(([A-Za-z0-9]+)\\)")
	// A message queue id association with message id header.
	rxMailMessageID := regexp.MustCompile("^([A-Za-z0-9]+):.*message-id=<(.*)>")

	// When a log message is received.
	for logParts := range channel {
		// Check to see if the received tag is one associated with emails.
		tag := logParts["tag"].(string)
		if !rxMailMessage.MatchString(tag) {
			continue
		}
		content := logParts["content"].(string)

		// Parse content to see if a message id association is being provided.
		matches := rxMailMessageID.FindStringSubmatch(content)
		if len(matches) == 3 {
			// If we received a message id association with the queue id,
			//  add it to the database for syslog id information.
			match := SysLogIDInfo{}
			match.Hostname = logParts["hostname"].(string)
			match.SID = matches[1]
			match.MessageID = matches[2]
			match.Status = "queued"
			app.db.Create(&match)
			log.Println("Syslog:", match.SID, match.Status)
			// The status was updated, so we can save to the queue for procoessing.
			app.sysLogMailUpdateQueue[match.SID+":"+match.Hostname] = true

			// Check if this is the first message queue id received for the connection.
			SysLogCheckIfNewConnection(matches[1])
			// Save this message to the syslog database.
			SysLogStoreMessage(logParts, matches[1])
			continue
		}

		// If message contains a queue id, we can just store it. Ignore NOQUEUE messages.
		matches = rxMailID.FindStringSubmatch(content)
		if len(matches) == 2 && matches[1] != "NOQUEUE" {
			// Check if this is the first message queue id received for the connection.
			SysLogCheckIfNewConnection(matches[1])
			// Save this message to the syslog database.
			SysLogStoreMessage(logParts, matches[1])
			continue
		}

		// If this is a end of message ok message, we can store it.
		matches = rxMailIDOk.FindStringSubmatch(content)
		if len(matches) == 2 {
			// Check if this is the first message queue id received for the connection.
			SysLogCheckIfNewConnection(matches[1])
			// Save this message to the syslog database.
			SysLogStoreMessage(logParts, matches[1])
			continue
		}

		// If this is a new connection message, update the buffer.
		matches = rxConnection.FindStringSubmatch(content)
		if len(matches) == 2 {
			// Save source address.
			sysLogBuffer.sourceAddr = matches[1]
			// Reset and store current message in message buffer.
			sysLogBuffer.logMessages = nil
			sysLogBuffer.logMessages = append(sysLogBuffer.logMessages, logParts)
			continue
		}

		// If this is a disconnection message, we can check if it matches an existing connection and close ito ut.
		matches = rxDisconnect.FindStringSubmatch(content)
		if len(matches) == 2 {
			// Go through the active connections.
			for i := 0; i < len(sysLogBuffer.activeConnections); i++ {
				connection := sysLogBuffer.activeConnections[i]
				// If this connection is older than 1 minute... We can discard it.
				if time.Since(connection.started).Seconds() >= 60 {
					// Discard this connection.
					sysLogBuffer.activeConnections = append(sysLogBuffer.activeConnections[:i], sysLogBuffer.activeConnections[i+1:]...)
					i--
				} else if matches[1] == connection.sourceAddr {
					// If this connection matches our disconnection, we can log the message.
					SysLogStoreMessage(logParts, connection.sid)

					// We can now discard this message.
					sysLogBuffer.activeConnections = append(sysLogBuffer.activeConnections[:i], sysLogBuffer.activeConnections[i+1:]...)
					i--
				}
			}
			continue
		}
		// If there is a new connection, and this log message was not matched above.
		// We then store this message in the buffer for associating with a message queue id above.
		if sysLogBuffer.sourceAddr != "" {
			sysLogBuffer.logMessages = append(sysLogBuffer.logMessages, logParts)
		}
	}
}

// This functions tarts the syslog server.
func SysLogServe() {
	// If syslog is not enabled, stop here.
	if !app.config.SysLogUDP && !app.config.SysLogTCP {
		return
	}

	// Create a new syslog buffer.
	sysLogBuffer = new(SysLogBuffer)

	// Get the configuration/
	sysLogBindAddr := app.config.SysLogBindAddr
	sysLogPort := app.config.SysLogPort
	if app.context.String("syslog-bind") != "" {
		sysLogBindAddr = app.context.String("syslog-bind")
	}
	if app.context.Uint("syslog-port") != 0 {
		sysLogPort = app.context.Uint("syslog-port")
	}

	// Create the syslog server and message channel.
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)
	server := syslog.NewServer()
	app.sysLogServer = server

	// Configure the syslog server.
	server.SetFormat(syslog.RFC3164)
	server.SetHandler(handler)
	if app.config.SysLogUDP {
		server.ListenUDP(fmt.Sprintf("%s:%d", sysLogBindAddr, sysLogPort))
	}
	if app.config.SysLogTCP {
		server.ListenTCP(fmt.Sprintf("%s:%d", sysLogBindAddr, sysLogPort))
	}

	// Start the syslog server.
	log.Println("Starting system log server on port", sysLogPort)
	server.Boot()

	// Start the message queue channel reader.
	go SysLogRunner(channel)

	// Wait until the syslog server stops.
	server.Wait()
}
