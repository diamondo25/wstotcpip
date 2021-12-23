package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var upstreamPorts []int

func main() {
	addr := flag.String("addr", ":9000", "http service address")
	upstreamPortsString := flag.String("accepted-upstream-ports", "8484,8585", "ports that can be accessed")

	flag.Parse()

	for _, str := range strings.Split(*upstreamPortsString, ",") {
		port, err := strconv.Atoi(str)
		if err != nil {
			log.Fatal("Unparsable port ", str, " in portlist")
		}
		upstreamPorts = append(upstreamPorts, port)
	}

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "test.html")
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		portStrArr, ok := r.URL.Query()["port"]
		if !ok {
			http.Error(w, "no port query argument", http.StatusPreconditionRequired)
			return
		}

		port, err := strconv.Atoi(portStrArr[0])
		if err != nil {
			http.Error(w, "invalid port query argument", http.StatusNotAcceptable)
			return
		}

		portOK := false
		for _, acceptedPort := range upstreamPorts {
			if acceptedPort == port {
				portOK = true
				break
			}
		}

		if !portOK {
			http.Error(w, "port not allowed", http.StatusNotAcceptable)
			return
		}

		// Make this a WebSocket

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Upgrade:", err)
			return
		}
		defer wsConn.Close()

		serverConn, err := net.Dial("tcp",  fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			log.Println("Unable to connect to server:", err)
			return
		}
		defer serverConn.Close()

		serverToWsDone := make(chan interface{})
		wsToServerDone := make(chan interface{})

		// server to ws
		go func() {
			buff := make([]byte, 1400)
			for {
				n, err := serverConn.Read(buff)
				if err != nil {
					log.Println("Unable to read data from the server: ", err)
					break
				}

				tmp := buff[:n]

				err = wsConn.WriteMessage(websocket.BinaryMessage, tmp)
				if err != nil {
					log.Println("Unable to write data to websocket: ", err)
					break
				}
			}

			serverToWsDone <- nil
		}()

		// ws to server
		go func() {

			for {
				mt, b, err := wsConn.ReadMessage()
				if err != nil {
					if err != io.EOF {
						log.Println("NextReader:", err)
					}
					return
				}

				if mt == websocket.TextMessage {
					// unused
				} else if mt == websocket.BinaryMessage {
					_, err := serverConn.Write(b)
					if err != nil {
						log.Println("Unable to write data to the server: ", err)
						break
					}
				}
			}

			wsToServerDone <- nil
		}()

		select {
		case <-wsToServerDone:
		case <-serverToWsDone:
		}
	})

	log.Println("Starting server...")
	log.Fatal("Unable to launch server: ", http.ListenAndServe(*addr, nil))
}
