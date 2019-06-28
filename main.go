// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	// Time allowed to write the file to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client.
	pongWait = 60 * time.Second

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Poll file for changes with this period.
	filePeriod = 100 * time.Millisecond
)

var (
	addr      = flag.String("addr", ":8080", "http service address")
	homeTempl = template.Must(template.New("").Parse(homeHTML))
	dataTempl = template.Must(template.New("").Parse(dataHTML))
	filename  string
	upgrader  = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

type XReadArgs struct {
	Streams []string
	Count   int64
	Block   time.Duration
}

func getRedisConf() string {
	redishost := "localhost"
	redisport := "6379"
	if len(os.Getenv("REDIS_HOST")) > 0 {
		redishost = os.Getenv("REDIS_HOST")
	}
	if len(os.Getenv("REDIS_PORT")) > 0 {
		redishost = os.Getenv("REDIS_PORT")
	}
	return (fmt.Sprintf("%s:%s", redishost, redisport))
}

func readStream() ([]byte, time.Time, error) {
	updates := []byte("")
	client := redis.NewClient(&redis.Options{
		Addr: getRedisConf(),
	})
	res, _ := client.XRead(&redis.XReadArgs{
		Streams: []string{"stream", "0"},
		Count:   25,
		Block:   100 * time.Millisecond,
	}).Result()

	for _, r := range res {
		for _, j := range r.Messages {
			tick := fmt.Sprintf("%s", j.Values["tick"])
			updates = append(updates, j.ID...)
			updates = append(updates, " => "...)
			updates = append(updates, tick...)
			updates = append(updates, "\n"...)
			client.XDel("stream", j.ID)
		}
	}
	if len(updates) < 1 {
		updates = []byte("waiting for updates...")
	}
	return updates, time.Now(), nil
}

func reader(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetReadLimit(512)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			break
		}
	}
}

func writer(ws *websocket.Conn, lastMod time.Time) {
	lastError := ""
	pingTicker := time.NewTicker(pingPeriod)
	fileTicker := time.NewTicker(filePeriod)
	defer func() {
		pingTicker.Stop()
		fileTicker.Stop()
		ws.Close()
	}()
	for {
		select {
		case <-fileTicker.C:
			var p []byte
			var err error

			p, lastMod, err = readStream()

			if err != nil {
				if s := err.Error(); s != lastError {
					lastError = s
					p = []byte(lastError)
				}
			} else {
				lastError = ""
			}

			if p != nil {
				ws.SetWriteDeadline(time.Now().Add(writeWait))
				if err := ws.WriteMessage(websocket.TextMessage, p); err != nil {
					return
				}
			}
		case <-pingTicker.C:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if _, ok := err.(websocket.HandshakeError); !ok {
			log.Println(err)
		}
		return
	}

	var lastMod time.Time
	if n, err := strconv.ParseInt(r.FormValue("lastMod"), 16, 64); err == nil {
		lastMod = time.Unix(0, n)
	}

	go writer(ws, lastMod)
	reader(ws)
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var v = struct{}{}
	homeTempl.Execute(w, &v)
}

func loadData(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/fire", 302)
}

func setData(w http.ResponseWriter, r *http.Request) {
	client := redis.NewClient(&redis.Options{
		Addr: getRedisConf(),
	})
	for i := 0; i <= 2000; i += 1 {

		_, err := client.XAdd(&redis.XAddArgs{
			Stream: "stream",
			ID:     "*",
			Values: map[string]interface{}{"tick": i},
		}).Result()
		time.Sleep(1 * time.Millisecond)
		if err != nil {
			log.Fatal(err)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<html><form action=\"/load\"><input type=\"submit\" value=\"Populate Stream\"></form></html>")
}

func serveData(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p, lastMod, err := readStream()
	if err != nil {
		p = []byte(err.Error())
		lastMod = time.Unix(0, 0)
	}
	var v = struct {
		Host    string
		Data    string
		LastMod string
	}{
		r.Host,
		string(p),
		strconv.FormatInt(lastMod.UnixNano(), 16),
	}
	dataTempl.Execute(w, &v)
}

func main() {
	//http.HandleFunc("/", serveHome)
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/data", serveData)
	http.HandleFunc("/fire", setData)
	http.HandleFunc("/load", loadData)
	http.HandleFunc("/ws", serveWs)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

const dataHTML = `<!DOCTYPE html>
<html lang="en">
    <head>
        <title>WebSocket Example</title>
    </head>
    <body>
        <pre id="fileData">{{.Data}}</pre>
        <script type="text/javascript">
            (function() {
                var data = document.getElementById("fileData");
                var conn = new WebSocket("ws://{{.Host}}/ws?lastMod={{.LastMod}}");
                conn.onclose = function(evt) {
                    data.textContent = 'Connection closed';
                }
                conn.onmessage = function(evt) {
                    console.log('file updated');
                    data.textContent = evt.data;
                }
            })();
        </script>
    </body>
</html>
`

const homeHTML = `
<!DOCTYPE html>
<html lang="en">
<FRAMESET ROWS="80%,20%">
    <FRAME SRC="/data" NAME="frm1" ID="frm1">
    <FRAME SRC="/fire" NAME="frm1" ID="frm1">
</FRAMESET>
</html>
`
