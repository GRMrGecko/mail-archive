package main

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/jinzhu/configor"
	"github.com/urfave/cli"
)

// Configuration Structure.
type Config struct {
	HTTPBindAddr   string `default:"" json:"http_bind_addr"`
	HTTPPort       uint   `default:"80" json:"http_port"`
	HTTPDebug      bool   `default:"false" json:"http_debug"`
	SMTPBindAddr   string `default:"" json:"smtp_bind_addr"`
	SMTPPort       uint   `default:"25" json:"smtp_port"`
	SMTPDomain     string `default:"localhost" json:"smtp_domain"`
	SysLogBindAddr string `default:"" json:"syslog_bind_addr"`
	SysLogPort     uint   `default:"514" json:"syslog_port"`
	SysLogUDP      bool   `default:"true" json:"syslog_udp"`
	SysLogTCP      bool   `default:"false" json:"syslog_tcp"`
	// There are some syslog ids which you may want to ignore because they belong to
	//  the original receiving message which is to be sent out, or because
	//  they belong to the message which is sent to this mail archive tool.
	// This configuration allows you to specify strings which identify the syslog ids
	//  you wish to ignore.
	// The ignore list only pertains to messages sent on or after the message-id message is received.
	SysLogIgnoreContaining []string `json:"syslog_ignore_containing"`

	StaticContentPath string `default:"./www/" json:"static_content_path"`

	DBType       string `default:"sqlite3" json:"database_type"` // Review documentation at http://gorm.io/docs/connecting_to_the_database.html
	DBConnection string `default:"MailArchive.db" json:"database_connection"`
	DBDebug      bool   `default:"false" json:"database_debug"`

	MailPath string `default:"db" json:"mail_path"`

	MaxAge         time.Duration `default:"1209600" json:"max_age"`          // Used for cleanup of old messages. Default is 2 weeks.
	MaxMessageSize int           `default:"5242880" json:"max_message_size"` // Default of 5 MB

	MessagesPerPage int `default:"100" json:"messages_per_page"`

	SpamReportingAPIBaseURLS []string `json:"spam_reporting_api_base_urls"`
	SpamReportingAuthHeader  string   `json:"spam_reporting_auth_header"` // If your spam reporting tool uses a header for authentication. In `Header: Value` format
	SpamReportingAuthKey     string   `json:"spam_reporting_auth_key"`    // If your spam reporting tool uses a post variable.
	SpamReportingAuthValue   string   `json:"spam_reporting_auth_value"`  // If your spam reporting tool uses a post variable.
	SpamReportingUploadName  string   `json:"spam_reporting_upload_name"`
	SpamReportingSpamURI     string   `default:"learn_spam" json:"spam_reporting_spam_uri"`
	SpamReportingHamURI      string   `default:"learn_ham" json:"spam_reporting_ham_uri"`

	UICustomBrand          string `defualt:"Mail Archive" json:"ui_custom_brand"`
	UIDisableSpamReporting bool   `defualt:"false" json:"ui_disable_spam_reporting"`
	UIDisableLogs          bool   `defualt:"false" json:"ui_disable_logs"`
}

// Load the configuration.
func initConfig(c *cli.Context) Config {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	// Configuration paths.
	localConfig, _ := filepath.Abs("./config.json")
	homeDirConfig := usr.HomeDir + "/.config/mail-archive/config.json"
	etcConfig := "/etc/mail-archive/config.json"

	// Determine which configuration to use.
	var configFile string
	if _, err := os.Stat(c.String("config")); err == nil {
		configFile = c.String("config")
	} else if _, err := os.Stat(localConfig); err == nil {
		configFile = localConfig
	} else if _, err := os.Stat(homeDirConfig); err == nil {
		configFile = homeDirConfig
	} else if _, err := os.Stat(etcConfig); err == nil {
		configFile = etcConfig
	} else {
		log.Fatal("Unable to find a configuration file.")
	}

	// Load the configuration file.
	config := Config{}
	err = configor.Load(&config, configFile)
	if config.HTTPPort == 0 {
		fmt.Println(err)
		log.Fatal("Unable to load the configuration file.")
	}
	return config
}

// Flags for the server command.
func configTestFlags() []cli.Flag {
	return []cli.Flag{}
}
