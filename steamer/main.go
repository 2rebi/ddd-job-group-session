package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/nareix/joy4/av/avutil"
	"github.com/nareix/joy4/av/pubsub"
	"github.com/nareix/joy4/format/flv"
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
		streams, _ := conn.Streams()

		l.Lock()
		path := strings.Split(conn.URL.Path, "/")
		if len(path) != 2 {
			fmt.Println("wrong path->", conn.URL.RequestURI())
			return
		}

		key := path[1] // key
		fmt.Println("request string->", conn.URL.RequestURI())
		ch := channels[key]
		if ch == nil {
			// 채널 생성
			ch = &Channel{}
			ch.que = pubsub.NewQueue()
			ch.que.WriteHeader(streams)
			channels[key] = ch
		} else {
			// 중복 접속 차단
			ch = nil
		}
		l.Unlock()

		// 중복 접속 차단
		if ch == nil {
			return
		}

		// 데이터 큐에 복사
		avutil.CopyPackets(ch.que, conn)

		// 채널 종료
		l.Lock()
		delete(channels, key)
		l.Unlock()
		ch.que.Close()
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Split(r.URL.Path, "/")
		if len(path) == 3 && path[2] == "stream" {
			l.RLock()
			ch := channels[path[1]]
			l.RUnlock()
			if ch != nil {
				//http 헤더 셋팅
				w.Header().Set("Content-Type", "video/x-flv")
				w.Header().Set("Transfer-Encoding", "chunked")
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.WriteHeader(200)
				flusher := w.(http.Flusher)
				flusher.Flush()

				// 스트림 구성
				muxer := flv.NewMuxerWriteFlusher(writeFlusher{httpflusher: flusher, Writer: w})
				cursor := ch.que.Latest()

				// 아웃 바운드
				avutil.CopyFile(muxer, cursor)
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