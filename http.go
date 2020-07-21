package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

// Basic HTTP server structure.
type HTTPServer struct {
	ws          *WS
	wsInterface *WSInterface
}

// This functions starts the HTTP server.
func HTTPServe() {
	// Get the configuration/
	httpBindAddr := app.config.HTTPBindAddr
	httpPort := app.config.HTTPPort
	if app.context.String("http-bind") != "" {
		httpBindAddr = app.context.String("http-bind")
	}
	if app.context.Uint("http-port") != 0 {
		httpPort = app.context.Uint("http-port")
	}

	// Create the server.
	httpServer := &HTTPServer{}
	app.httpServer = httpServer
	// Intitialize the websocket handler.
	httpServer.wsInterface = new(WSInterface)
	httpServer.ws = WSInit(httpServer.wsInterface)
	httpServer.wsInterface.ws = httpServer.ws

	// Set the handlers.
	r := mux.NewRouter()
	httpServer.RegisterAPIRoutes(r)
	r.HandleFunc("/ws", httpServer.ws.Handler)
	fs := http.FileServer(http.Dir(app.config.StaticContentPath))
	r.PathPrefix("/").Handler(fs)

	// The http server handler will be the mux router by default.
	var handler http.Handler
	handler = r
	// If the debug log is enabled, we'll add a middleware handler to log then pass the request to mux router.
	if app.config.HTTPDebug {
		handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			log.Println(req.Method + " " + req.URL.String())
			r.ServeHTTP(w, req)
		})
	}

	// Start the server.
	log.Println("Starting http server on port", httpPort)
	err := http.ListenAndServe(fmt.Sprintf("%s:%d", httpBindAddr, httpPort), handler)
	if err != nil {
		log.Fatal(err)
	}
}
