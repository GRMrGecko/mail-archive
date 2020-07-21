package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 1024 * 1024
)

// Setup an upgrader with our configuration.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WSMessageInterface interface {
	handleMessage(message []byte, c *WSClient)
}

// The websocket server structure.
type WS struct {
	clients          map[*WSClient]bool
	message          chan []byte
	register         chan *WSClient
	unregister       chan *WSClient
	messageInterface WSMessageInterface
}

// Setup the websocket
func WSInit(messageInterface WSMessageInterface) *WS {
	ws := &WS{
		message:          make(chan []byte),
		register:         make(chan *WSClient),
		unregister:       make(chan *WSClient),
		clients:          make(map[*WSClient]bool),
		messageInterface: messageInterface,
	}
	go ws.run()
	return ws
}

// Main websocket channel handler.
func (ws *WS) run() {
	for {
		select {
		case client := <-ws.register:
			// If we have a new client, add it to the client map.
			ws.clients[client] = true
		case client := <-ws.unregister:
			// If a client is unregistering and in the client map, remove it.
			if _, ok := ws.clients[client]; ok {
				delete(ws.clients, client)
				close(client.send)
			}
		case message := <-ws.message:
			// A message is being sent to all clients.
			for client := range ws.clients {
				// Send message to client if possible.
				select {
				case client.send <- message:
				default:
					// If we were unable to send, the client is no longer connected.
					close(client.send)
					delete(ws.clients, client)
				}
			}
		}
	}
}

// Websocket client structure.
type WSClient struct {
	ws *WS

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

// Read messages from the client.
func (c *WSClient) reader() {
	// When we are unable to read from the client, we need to unregister and close the connection.
	defer func() {
		c.ws.unregister <- c
		c.conn.Close()
	}()

	// Set the max size of a message from the client.
	c.conn.SetReadLimit(maxMessageSize)
	// The client must send us keep alive messages, otherwise the connection is dead.
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	// When we receive a pong from the client, the keep alive timeout is extended.
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	// Until connection is close, read messages fromt eh client.
	for {
		_, message, err := c.conn.ReadMessage()
		// If we received an error, something is wrong and we need to close the connection.
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		// We got a connection, so we can pass it to the message interface for handling.
		c.ws.messageInterface.handleMessage(message, c)
	}
}

// Watches the message send channel for new messages to send to the client.
func (c *WSClient) writer() {
	// We need to pink the client every now and then.
	ticker := time.NewTicker(pingPeriod)
	// If the writer channel closes, we need to stop the ping ticker and close the connection.
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	// Check for new message or ping ticker, whichever happens first.
	for {
		select {
		case message, ok := <-c.send:
			// If a new message was added tot he channel, we need to write it.
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			// If the message received is closing the channel, we need to send a close message.
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Get the next writer for a text message.
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			// Write the message to the client.
			w.Write(message)

			// Close the writer. If an error, we stop the write channel.
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			// Send a ping message.
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// New connection handler for websockets that creates a client and upgrades the connection.
func (ws *WS) Handler(w http.ResponseWriter, r *http.Request) {
	log.Println("New websocket connection from ", r.RemoteAddr)
	// Upgade the connection to a websocket connection.
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Create a new client and register it.
	client := &WSClient{ws: ws, conn: conn, send: make(chan []byte, 256)}
	ws.register <- client

	// Start the reader and writer.
	go client.reader()
	go client.writer()
}
