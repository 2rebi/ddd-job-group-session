package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/format/flv"
	"github.com/nareix/joy4/format/flv/flvio"
	"github.com/nareix/joy4/format/rtmp"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

var (
	addr = flag.String("l", ":8080", "host:port of the go-rtmp-server")
)

type writeFlusher struct {
	httpflusher http.Flusher
	io.Writer
}

func (self writeFlusher) Flush() error {
	self.httpflusher.Flush()
	return nil
}

type (
	channel struct {
		conn *rtmp.Conn
		roomName string
		viewers map[*viewer]bool
		join chan *viewer
		exit chan *viewer
	}
	channelRooms struct {
		channels map[string]*channel
		viewers map[string]map[*viewer]bool
		register chan *channel
		unregister chan *channel
		join chan *viewer
		exit chan *viewer
	}
	viewer struct {
		roomName string
		writer writeFlusher
		sendPacket chan av.Packet
		setStream chan []av.CodecData
	}
)

func parseRoom(url *url.URL) (roomName string, err error) {
	path := strings.Split(url.Path, "/")
	if len(path) != 2 {
		err = errors.New("wrong path")
		return
	}
	roomName = path[1]
	return
}

func newViewer(room string, writer http.ResponseWriter) *viewer {
	return &viewer{room, writeFlusher{
		httpflusher: writer.(http.Flusher),
		Writer:      writer,
	}, make(chan av.Packet, 256), make(chan []av.CodecData)}
}

func (v *viewer) runReceiving(cr *channelRooms) (err error) {
	defer func() {
		cr.exit <- v
	}()

	var streams []av.CodecData
	var b []byte
	prevTime := int32(0)
	offset := int32(0)
	//clientTimestamp := int32(0)
	for {
		select {
		case pkt := <-v.sendPacket:
			if streams != nil {
				stream := streams[pkt.Idx]

				tag, timestamp := flv.PacketToTag(pkt, stream)
				if prevTime < timestamp {
					prevTime = timestamp
				}
				//delta := timestamp - prevTime
				//if delta > 0 {
				//	clientTimestamp = clientTimestamp + delta
				//} else {
				//	clientTimestamp = clientTimestamp + timestamp
				//}

				//log.Printf("clientTimestamp : %d, timestamp : %d, delta : %d\n", clientTimestamp, timestamp, delta)

				if err = flvio.WriteTag(v.writer, tag, timestamp+offset, b); err != nil {
					log.Println(err)
					return
				}
				//log.Println(b)
			}
			break
		case bufstreams := <-v.setStream:
			if bufstreams != nil {
				streams = bufstreams
				offset = prevTime
				v.writer.Flush()
				b = make([]byte, 256)
				var flags uint8
				for _, stream := range streams {
					if stream.Type().IsVideo() {
						flags |= flvio.FILE_HAS_VIDEO
					} else if stream.Type().IsAudio() {
						flags |= flvio.FILE_HAS_AUDIO
					}
				}

				n := flvio.FillFileHeader(b, flags)
				if _, err = v.writer.Write(b[:n]); err != nil {
					return
				}

				for _, stream := range streams {
					var tag flvio.Tag
					var ok bool
					if tag, ok, err = flv.CodecDataToTag(stream); err != nil {
						return
					}
					if ok {
						if err = flvio.WriteTag(v.writer, tag, offset, b); err != nil {
							return
						}
					}
				}

			}
			break
		}
	}
}

func newChannel(conn *rtmp.Conn) *channel {
	if room, err := parseRoom(conn.URL); err != nil {
		return nil
	} else {
		return &channel{conn, room, make(map[*viewer]bool), make(chan *viewer), make(chan *viewer)}
	}
}

func (ch *channel) onAir(cr *channelRooms) (err error) {
	setAllStream := func(streams []av.CodecData) {
		for v := range ch.viewers {
			v.setStream <- streams
		}
	}

	defer func() {
		cr.unregister <- ch
		setAllStream(nil)
	}()

	src := ch.conn
	stream, err := src.Streams()
	if err != nil {
		return
	}

	setAllStream(stream)
onAir:
	for {
		select {
		case v := <-ch.join:
			v.setStream <- stream
			ch.viewers[v] = true
			break
		case v := <-ch.exit:
			delete(ch.viewers, v)
			break
		default:
			var pkt av.Packet
			if pkt, err = src.ReadPacket(); err != nil {
				if err == io.EOF {
					break onAir
				}
				return
			}

			ch.broadcastPacket(pkt)
		}

	}
	return
}

func (ch *channel) broadcastPacket(pkt av.Packet) {
	//log.Printf("broadcastPacket : %s\n", ch.roomName)
	for v := range ch.viewers {
		select {
		case v.sendPacket <- pkt:
			break
		}
	}
}

func (cr *channelRooms) handleConnection(conn *rtmp.Conn) {
	cr.register <- newChannel(conn)
}

func (cr *channelRooms) run() {
	log.Printf("run channel room")
	for {
		select {
		case ch := <-cr.register:
			if ch != nil {
				log.Printf("channel register : %s\n", ch.roomName)
				if _, ok := cr.channels[ch.roomName]; !ok {
					if room, ok := cr.viewers[ch.roomName]; ok {
						log.Printf("have room : %s\n", ch.roomName)
						ch.viewers = room
					} else {
						log.Printf("have not room : %s\n", ch.roomName)
						cr.viewers[ch.roomName] = make(map[*viewer]bool)
						ch.viewers = make(map[*viewer]bool)
					}
					go func() {
						err := ch.onAir(cr)
						log.Println(err)
					}()
					cr.channels[ch.roomName] = ch
					log.Printf("channel register success : %s\n", ch.roomName)
				} else {
					log.Printf("channel exists : %s\n", ch.roomName)
				}
			}
			break
		case ch := <-cr.unregister:
			log.Printf("channel unregister : %s\n", ch.roomName)
			delete(cr.channels, ch.roomName)
			break
		case v := <-cr.join:
			log.Printf("view join : %s\n", v.roomName)
			//go v.runReceiving(cr)
			if ch, ok := cr.channels[v.roomName]; ok {
				log.Printf("view join to channel sucess : %s\n", v.roomName)
				ch.join <- v
			} else {
				log.Printf("view join to channel waiting : %s\n", v.roomName)
			}
			if room, ok := cr.viewers[v.roomName]; ok {
				log.Printf("view join to room sucess : %s\n", v.roomName)
				room[v] = true
			} else {
				log.Printf("make room : %s\n", v.roomName)
				cr.viewers[v.roomName] = make(map[*viewer]bool)
				log.Printf("view join to room sucess : %s\n", v.roomName)
				cr.viewers[v.roomName][v] = true
			}
			break
		case v := <-cr.exit:
			log.Printf("view exit : %s\n", v.roomName)
			if ch, ok := cr.channels[v.roomName]; ok {
				log.Printf("view exit from channel : %s\n", v.roomName)
				ch.exit <- v
			}
			delete(cr.viewers[v.roomName], v)
			break
		}
	}
}


func newChannelRooms(server *rtmp.Server) (cr *channelRooms) {
	cr = &channelRooms{
		channels:   make(map[string]*channel),
		viewers:    make(map[string]map[*viewer]bool),
		register:   make(chan *channel),
		unregister: make(chan *channel),
		join:       make(chan *viewer),
		exit:       make(chan *viewer),
	}
	server.HandlePublish = cr.handleConnection
	return
}
func main() {
	flag.Parse()
	server := &rtmp.Server{}
	cr := newChannelRooms(server)
	go cr.run()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Split(r.URL.Path, "/")
		if len(path) == 3 && path[2] == "stream" {
			room := path[1]

			w.Header().Set("Content-Type", "video/x-flv")
			w.Header().Set("Transfer-Encoding", "chunked")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			v := newViewer(room, w)
			go func () {
				cr.join <- v
			}()
			v.runReceiving(cr)
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