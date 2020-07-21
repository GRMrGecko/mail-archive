package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DusanKasan/parsemail"
	"github.com/gorilla/mux"
)

// Commonly used strings.
const (
	APIOK          = "ok"
	APIERR         = "error"
	APINoEndpoint  = "No endpoint found"
	APIReadMessage = "Error reading message."
	APINoMessage   = "Messages was not found"
)

// Main response structure.
type APIGeneralResp struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// Response with configuration.
type APIConfigResp struct {
	APIGeneralResp
	CustomBrand          string `json:"custom_brand"`
	DisableSpamReporting bool   `json:"disable_spam_reporting"`
	DisableLogs          bool   `json:"disable_logs"`
	MessageCount         uint   `json:"message_count"`
}

// Response with message log entries.
type APIMessageLogResp struct {
	APIGeneralResp
	Messages []MessageLog `json:"messages"`
}

// Response with message entry.
type APIMessageEntryResp struct {
	APIGeneralResp
	Messages MessageLog `json:"message"`
}

// Response to spam report requests.
type APISpamReportResp struct {
	APIGeneralResp
	Requests []map[string]string `json:"requests"`
}

// Typical API responses are done with JSON. To make it easier to respond, this function will marshal/send json to a response writer.
func (s *HTTPServer) JSONResponse(w http.ResponseWriter, resp interface{}) {
	// Encode response as json.
	js, err := json.Marshal(resp)
	if err != nil {
		// Error should not happen normally...
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// If no error, we can set content type header and send response.
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// There are quite a few request that send a general response on error. This function is to make it easy to build/send a general response.
func (s *HTTPServer) APISendGeneralResp(w http.ResponseWriter, status, error string) {
	resp := APIGeneralResp{}
	resp.Status = status
	resp.Error = error
	s.JSONResponse(w, resp)
}

// Setup HTTP router with routes for the API calls.
func (s *HTTPServer) RegisterAPIRoutes(r *mux.Router) {
	api := r.PathPrefix("/api").Subrouter()
	// Just a test call.
	api.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		s.APISendGeneralResp(w, APIOK, "")
	})

	// Retrieve the configuration.
	api.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		resp := APIConfigResp{}
		resp.CustomBrand = app.config.UICustomBrand
		resp.DisableSpamReporting = app.config.UIDisableSpamReporting
		resp.DisableLogs = app.config.UIDisableLogs
		resp.MessageCount = app.messageCount
		s.JSONResponse(w, resp)
	})

	// Retrieve message logs matching criteria provided.
	api.HandleFunc("/message_log", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm() // r.Form isn't filled unless we first parse.
		query := r.Form.Get("q")

		// Page variable provided should be an integer.
		page, _ := strconv.Atoi(r.Form.Get("p"))
		if page <= 0 { // If page is lower than 1, we need it to be page 1.
			page = 1
		}
		// Offset based on page number and max messages per page set.
		offset := app.config.MessagesPerPage * (page - 1)
		if offset <= 0 { // If lower than 1, we can just set to -1 to unset the offset field in queries.
			offset = -1
		}

		var entries []MessageLog
		// If a query is not provided, we can just pull entires from the database without additional filters.
		if query == "" {
			app.db.Order("received desc").Offset(offset).Limit(app.config.MessagesPerPage).Find(&entries)
		} else {
			// As a query was provided, we need to parse the query out to a SQL where statement.
			// Splitting the query up by words to allow matches against 2 differen fields in the same query.
			// Example: test@example.com sent
			// The above will match both an email address and the status of sent.
			queryS := strings.Split(query, " ")
			var queries []string
			var statements []interface{} // Must be an interface to expand to arguments in a function call.
			// For each word, setup LIKE statements.
			for _, q := range queryS {
				likeStatement := "%" + q + "%"
				// Append like queries to slice.
				queries = append(queries, "(`from` LIKE ? OR `to` LIKE ? OR `subject` LIKE ? OR `source_ip` LIKE ? OR `message_id` LIKE ? OR `status` LIKE ?)")
				// Append statements to slice.
				statements = append(statements, likeStatement, likeStatement, likeStatement, likeStatement, likeStatement, likeStatement)
			}

			// Join queries with an AND, and also turn statements into arguments for the database WHERE statement.
			app.db.Where(strings.Join(queries, " AND "), statements...).Order("received desc").Offset(offset).Limit(app.config.MessagesPerPage).Find(&entries)
		}

		// Return found entries, if any.
		resp := APIMessageLogResp{}
		resp.Status = APIOK
		resp.Messages = entries
		s.JSONResponse(w, resp)
	})

	// Pull log entries for a message.
	api.HandleFunc("/message/{id}.log", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r) // Parses the variable matched in the request URI.
		UUID := vars["id"]

		// If message id provided is blank, the message cannot exist.
		if UUID == "" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		// Search the database for a message log entry for the message id to ensure that it exists.
		var messageEntry MessageLog
		app.db.Where("uuid = ?", UUID).First(&messageEntry)
		// If return UUID is blank, we didn't find an entry.
		if messageEntry.UUID == "" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		// Search for SysLog ID information that matches the message ID.
		var matches []SysLogIDInfo
		app.db.Where("message_id = ?", messageEntry.MessageID).Find(&matches)
		// If no matches, no logs.
		if len(matches) == 0 {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		// Place all SIDs into a slice so that we can run a single query to get all log messages.
		var sids []string
		hostname := matches[0].Hostname // The hostname of the first entry should suffice here. We shouldn't see another host with that message id.
		for _, match := range matches {
			sids = append(sids, match.SID)
		}

		// Find log messages orderd by timestamp matching the SIDs we gathered above.
		var messages []SysLogMessage
		app.db.Where("s_id IN (?) AND hostname = ?", sids, hostname).Order("timestamp").Find(&messages)
		// If no log entries... We just can't provide them.
		if len(messages) == 0 {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		// Log messages were found, we need content type to be plain text.
		w.Header().Set("Content-Type", "text/plain")
		// Print all logs to the response writer.
		for _, message := range messages {
			fmt.Fprintf(w, "%v %s %s: %s\n", message.Timestamp, message.Hostname, message.Tag, message.Content)
		}
	})

	api.HandleFunc("/message/{id}.{type}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r) // Parses the variable matched in the request URI.
		UUID := vars["id"]
		messageType := vars["type"]

		// If message id provided is blank, the message cannot exist.
		if UUID == "" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		// Get a io.Reader instance of the message data from either the database or file system, whichever is set.
		reader, err := MailGetMessageData(UUID)
		// If we could not get a reader, that is more than likely due to the message not existing.
		if err != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		// If we need to close after reading, defer the close to after this function call.
		if x, ok := reader.(io.Closer); ok {
			defer x.Close()
		}

		// Based on response type requested, parse the message data accordingly.
		if messageType == "eml" { // Original email source.
			// Provide standard email message mime type.
			w.Header().Set("Content-Type", "message/rfc822")
			// Copy message data to response writer.
			_, err := io.Copy(w, reader)
			if err != nil { // If error, return to client an error.
				s.APISendGeneralResp(w, APIERR, APIReadMessage)
				return
			}
		} else if messageType == "txt" { // Plain text format.
			// Provide plain text mime type.
			w.Header().Set("Content-Type", "text/plain")
			// Parse the email fields.
			email, err := parsemail.Parse(reader)
			if err != nil { // If error, return to client an error.
				s.APISendGeneralResp(w, APIERR, APIReadMessage)
				return
			}

			// If email transfer encoding is quoted-printable, we need to decode the message.
			encoding := email.Header.Get("Content-Transfer-Encoding")
			if encoding == "quoted-printable" {
				// Create a reader with the text body of the email.
				bodyR := strings.NewReader(email.TextBody)
				quotedR := quotedprintable.NewReader(bodyR)
				// Copy from the reader to the response writer.
				_, err = io.Copy(w, quotedR)
				if err != nil { // If error, return to client an error.
					s.APISendGeneralResp(w, APIERR, APIReadMessage)
					return
				}
			} else if encoding == "base64" { // If encoded with Base64
				// Create a reader with the text body of the email.
				bodyR := strings.NewReader(email.TextBody)
				base64R := base64.NewDecoder(base64.StdEncoding, bodyR)
				// Copy from the reader to the response writer.
				_, err = io.Copy(w, base64R)
				if err != nil { // If error, return to client an error.
					s.APISendGeneralResp(w, APIERR, APIReadMessage)
					return
				}
			} else {
				// As this is a standard plain text email, we can just write it to the response writer.
				w.Write([]byte(email.TextBody))
			}
		} else if messageType == "html" { // HTML body requested.
			// Set mime type to html.
			w.Header().Set("Content-Type", "text/html")
			// Parse the email fields.
			email, err := parsemail.Parse(reader)
			if err != nil { // If error, return to client an error.
				s.APISendGeneralResp(w, APIERR, APIReadMessage)
				return
			}

			// If email transfer encoding is quoted-printable, we need to decode the message.
			encoding := email.Header.Get("Content-Transfer-Encoding")
			if encoding == "quoted-printable" {
				// Create a reader with the html body of the email.
				bodyR := strings.NewReader(email.HTMLBody)
				quotedR := quotedprintable.NewReader(bodyR)
				// Copy from the reader to the response writer.
				_, err = io.Copy(w, quotedR)
				if err != nil { // If error, return to client an error.
					s.APISendGeneralResp(w, APIERR, APIReadMessage)
					return
				}
			} else if encoding == "base64" { // If encoded with Base64
				// Create a reader with the html body of the email.
				bodyR := strings.NewReader(email.HTMLBody)
				base64R := base64.NewDecoder(base64.StdEncoding, bodyR)
				// Copy from the reader to the response writer.
				_, err = io.Copy(w, base64R)
				if err != nil { // If error, return to client an error.
					s.APISendGeneralResp(w, APIERR, APIReadMessage)
					return
				}
			} else {
				// Standard HTML email body, we can just write it to the response writer.
				w.Write([]byte(email.HTMLBody))
			}
		} else {
			// No matching message type was found. Just provide a no endpoint response.
			s.APISendGeneralResp(w, APIERR, APINoEndpoint)
		}
	})

	// Spam reporting request must be called with a PUT request as an extra procaution to ensure we actually want to report.
	api.HandleFunc("/message/{id}/learn_{type}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r) // Parses the variable matched in the request URI.
		UUID := vars["id"]

		// If message id provided is blank, the message cannot exist.
		if UUID == "" {
			s.APISendGeneralResp(w, APIERR, APINoMessage)
			return
		}

		// The response type should only be spam or ham.
		reportType := vars["type"]
		var reportingURI string
		if reportType == "spam" {
			reportingURI = app.config.SpamReportingSpamURI
		} else if reportType == "ham" {
			reportingURI = app.config.SpamReportingHamURI
		}

		// If the reporting URI was not sent, we return a no endpoint found message.
		if reportingURI == "" {
			s.APISendGeneralResp(w, APIERR, APINoEndpoint)
			return
		}

		// To report spam, we need the message data. So we will find it in either the database or file system accordingly.
		reader, err := MailGetMessageData(UUID)
		if err != nil { // If no message found, we tell the client.
			s.APISendGeneralResp(w, APIERR, APINoMessage)
			return
		}
		// If we need to close after reading, defer the close to after this function call.
		if x, ok := reader.(io.Closer); ok {
			defer x.Close()
		}

		// Build a response for the request.
		resp := APISpamReportResp{}
		anySuccess := false // We rely on this variable to determine if a successful request was made.

		// Multipart Form Data writer/buffer for sending message via post.
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)

		// file form entry.
		fw, err := mw.CreateFormFile(app.config.SpamReportingUploadName, UUID+".eml")
		if err != nil {
			s.APISendGeneralResp(w, APIERR, "Unable to build report.")
			return
		}
		// Copy message data to multipart form entry.
		if _, err = io.Copy(fw, reader); err != nil {
			s.APISendGeneralResp(w, APIERR, "Unable to build report.")
			return
		}

		// If an authentication field is set, add it to the form data.
		if app.config.SpamReportingAuthKey != "" {
			mw.WriteField(app.config.SpamReportingAuthKey, app.config.SpamReportingAuthValue)
		}

		// We are done adding to the multipart form.
		mw.Close()

		// Go through the configured spam reporting URLs and submit.
		for _, baseURL := range app.config.SpamReportingAPIBaseURLS {
			url := baseURL + reportingURI

			// Request string map to provide feedback via API on what happend per reporting URL.
			request := make(map[string]string)
			request["url"] = url

			// Make request for the report using the form data.
			req, err := http.NewRequest("POST", url, &b)
			if err != nil { // If failed, just mark this request as failed and continue.
				request["success"] = "false"
				request["error"] = fmt.Sprintf("%v", err)
				resp.Requests = append(resp.Requests, request)
				continue
			}

			// Set the content type header to the multipart formdata header with proper boundary.
			req.Header.Set("Content-Type", mw.FormDataContentType())

			// If an authentication header is set in config, we need to send it.
			authHeaderS := strings.Split(app.config.SpamReportingAuthHeader, ": ")
			if len(authHeaderS) == 2 {
				req.Header.Set(authHeaderS[0], authHeaderS[1])
			}

			// Setup http client.
			client := &http.Client{
				Timeout: time.Second * 10,
			}
			// Send report.
			res, err := client.Do(req)
			if err != nil { // If error, just store that this errored and continue.
				request["success"] = "false"
				request["error"] = fmt.Sprintf("%v", err)
				resp.Requests = append(resp.Requests, request)
				continue
			}

			// If the status code is not ok, something went wrong... Store that this failed and continue.
			if res.StatusCode != http.StatusOK {
				request["success"] = "false"
				request["error"] = fmt.Sprintf("%v", res.StatusCode)
				resp.Requests = append(resp.Requests, request)
				continue
			}
			// If we made this this far, sending the report was a success.
			request["success"] = "true"

			// read the respons ebody.
			defer res.Body.Close()
			body, _ := ioutil.ReadAll(res.Body)
			request["response"] = string(body)
			// Add the request to the list of requests in the response.
			resp.Requests = append(resp.Requests, request)
			// A request was successful.
			anySuccess = true
		}
		// If no successful request, we return an erro.
		if !anySuccess {
			resp.Status = APIERR
			resp.Error = "No successful request was made."
		} else { // If a request was successful, return an ok.
			resp.Status = APIOK
		}
		// Send response.
		s.JSONResponse(w, resp)
	}).Methods("PUT") // Adds requirement of PUT method to the spam reporter request.

	// Pull message entry.
	api.HandleFunc("/message/{id}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r) // Parses the variable matched in the request URI.
		UUID := vars["id"]

		// The response structure.
		resp := APIMessageEntryResp{}

		// If message id provided is blank, the message cannot exist.
		if UUID == "" {
			resp.Status = APIERR
			resp.Error = APINoMessage
			s.JSONResponse(w, resp)
			return
		}

		// Search the database for a message log entry for the message id to ensure that it exists.
		var messageEntry MessageLog
		app.db.Where("uuid = ?", UUID).First(&messageEntry)
		// If return UUID is blank, we didn't find an entry.
		if messageEntry.UUID == "" {
			resp.Status = APIERR
			resp.Error = APINoMessage
			s.JSONResponse(w, resp)
			return
		}

		resp.Status = APIOK
		resp.Messages = messageEntry
		s.JSONResponse(w, resp)
	})

	// If nothing else, we return a not found response.
	api.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.APISendGeneralResp(w, APIERR, APINoEndpoint)
	})
}
