package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/nareix/joy4/av/pubsub"
	"github.com/nareix/joy4/format/rtmp"
)

var (
	addr = flag.String("l", ":8089", "host:port of the go-rtmp-server")
)

type writeFlusher struct {
	httpflusher http.Flusher
	io.Writer
}

func (self writeFlusher) Flush() error {
	self.httpflusher.Flush()
	return nil
}

func main() {
	flag.Parse()
	server := &rtmp.Server{}

	l := &sync.RWMutex{}
	type Channel struct {
		que *pubsub.Queue
	}
	channels := map[string]*Channel{}

	server.HandlePublish = func(conn *rtmp.Conn) {
		/*streams, _ := */conn.Streams()

		l.Lock()
		path := strings.Split(conn.URL.Path, "/")
		if len(path) != 2 {
			fmt.Println("wrong path->", conn.URL.RequestURI())
			return
		}

		_ := path[1]
		fmt.Println("request string->", conn.URL.RequestURI())
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Split(r.URL.Path, "/")
		if len(path) == 3 && path[2] == "stream" {
			l.RLock()
			ch := channels[path[1]]
			l.RUnlock()
			if ch != nil {
			}
		} else {
			fmt.Println("Request url: ", r.URL.Path)
			content, err := ioutil.ReadFile("index.html")
			if err != nil {
				w.WriteHeader(404)
				w.Write([]byte(http.StatusText(404)))
				return
			}
			w.Header().Add("Content-Type", "text/html")
			w.Write(content)
		}

	})

	go http.ListenAndServe(*addr, nil)
	fmt.Println("Listen and serve ", *addr)

	server.ListenAndServe()
}