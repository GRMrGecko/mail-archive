package main

import (
	"log"
	"os"

	"github.com/emersion/go-smtp"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/urfave/cli"
	"gopkg.in/mcuadros/go-syslog.v2"
)

// Global application structure for communicating between servers and storing information.
type App struct {
	context               *cli.Context
	config                Config
	db                    *gorm.DB
	httpServer            *HTTPServer
	smtpServer            *smtp.Server
	sysLogServer          *syslog.Server
	sysLogMailUpdateQueue map[string]bool
	messageCount          uint
}

var app *App

// Main start of the application.
func appInit(c *cli.Context) {
	app = new(App)
	app.context = c
	app.config = initConfig(c)

	// Connect to the database.
	db, err := gorm.Open(app.config.DBType, app.config.DBConnection)
	if err != nil {
		log.Fatal(err)
	}
	initDB(db)
	app.db = db

	// Get message count.
	db.Model(&MessageLog{}).Count(&app.messageCount)

	// Automatically clean up old email every 30 minutes.
	go RunDatabaseCleanup()

	// Start SysLog servers.
	app.sysLogMailUpdateQueue = make(map[string]bool) // Must initialize maps.
	go SysLogServe()
	// As syslog updates email status, we need to also update related messages.
	go RunSysLogMailUpdateQueue()

	// Start SNMTP server.
	go SMTPServe()
	HTTPServe()
}

func main() {
	capp := cli.NewApp()
	capp.Name = "mail-archive"
	capp.Usage = "Email Archive Server with SysLog support and web interface."
	capp.EnableBashCompletion = true
	capp.Version = "0.1"
	capp.Action = appInit // By default, we start the initialize function.

	capp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Load configuration from `FILE`",
		},
		cli.StringFlag{Name: "http-bind"},
		cli.UintFlag{Name: "http-port"},
		cli.StringFlag{Name: "smtp-bind"},
		cli.UintFlag{Name: "smtp-port"},
		cli.StringFlag{Name: "smtp-domain"},
		cli.StringFlag{Name: "syslog-bind"},
		cli.UintFlag{Name: "syslog-port"},
	}

	err := capp.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
