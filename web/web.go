// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tool for directly communicating with Delphin ExpertKey DAQs

package main

import (
	"compress/gzip"
	"container/ring"
	"encoding/json"
	"fmt"
	"github.com/nsf/termbox-go"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	STD_DEV_RED      = 32
	SLOW_BUFFER_SIZE = 20000
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	sniffDone bool
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.sniffDone {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.sniffDone = true
	}
	return w.Writer.Write(b)
}

// Wrap a http.Handler to support transparent gzip encoding.
func gzHandler(h http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		h.ServeHTTP(&gzipResponseWriter{Writer: gz, ResponseWriter: w}, r)
	})
}

//N version of ring.Do in reverse
func doNr(r *ring.Ring, n int, f func(interface{})) int {
	e := r
	i := 0
	for ; i < n && e.Value != nil; i++ {
		f(e.Value)
		e = e.Prev()
	}
	return i
}

func httpserver(del []*DelphinReceiver, slow_buffer [][]*ring.Ring, std_dev [][]*ring.Ring) {

	//Calculate x sec average
	//Calculate standard deviation every 10 seconds
	go func() {
		t := time.NewTicker(10000 * time.Millisecond)
		for {
			for di, d := range del {
				for i := 0; i < 31; i++ {
					avg := float64(0)
					num := doNr(d.ValueBuffer[i], STD_DEV_RED, func(e interface{}) { avg += e.(ChannelData).Value })
					if num > 0 {
						avg = avg / float64(num)
						last := d.ValueBuffer[i].Value.(ChannelData).Timestamp
						std := float64(0)
						num2 := doNr(d.ValueBuffer[i], num, func(e interface{}) { std += math.Pow(e.(ChannelData).Value-avg, 2) })
						std = math.Sqrt(std / float64(num2))
						std_dev[di][i] = std_dev[di][i].Next()
						std_dev[di][i].Value = ChannelData{last, std}
						slow_buffer[di][i] = slow_buffer[di][i].Next()
						slow_buffer[di][i].Value = ChannelData{last, avg}
					}

				}
			}
			<-t.C
		}
	}()

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/json/slow", gzHandler(func(w http.ResponseWriter, r *http.Request) {
		_, zone_offset := time.Now().Zone() //Javascript is dumb
		r.ParseForm()
		enc := json.NewEncoder(w)
		requested_values := 100
		unit := 0 // Default delphin unit is 0
		if ch, err := strconv.Atoi(r.FormValue("channel")); err == nil {
			if unit, err = strconv.Atoi(r.FormValue("unit")); err != nil {
				unit = 0
			}
			if requested_values, err = strconv.Atoi(r.FormValue("values")); err != nil {
				requested_values = 100
			}
			if requested_values > SLOW_BUFFER_SIZE {
				requested_values = SLOW_BUFFER_SIZE
			}
			data := make([]interface{}, 0, requested_values)
			e := slow_buffer[unit][ch]
			for i := 0; i < requested_values && e.Value != nil; i++ {
				data = append(data, []interface{}{int64(e.Value.(ChannelData).Timestamp.UnixNano()/1000/1000) + int64(zone_offset*1000), int64(e.Value.(ChannelData).Value + 0.5)})
				e = e.Prev()
			}
			stddev := make([]interface{}, 0, requested_values)
			e = std_dev[unit][ch]
			for i := 0; i < requested_values && e.Value != nil; i++ {
				stddev = append(stddev, []interface{}{int64(e.Value.(ChannelData).Timestamp.UnixNano()/1000/1000) + int64(zone_offset*1000), float64(int64(e.Value.(ChannelData).Value*1000)) / 1000})
				e = e.Prev()
			}
			enc.Encode(map[string]interface{}{"values": data, "stddev": stddev, "error": false})
			data = nil
		} else {
			enc.Encode(map[string]interface{}{"error": true, "error_msg": "No channel defined", "error_num": 440})
		}
	}))
	http.HandleFunc("/json/fast", gzHandler(func(w http.ResponseWriter, r *http.Request) {
		_, zone_offset := time.Now().Zone() //Javascript is dumb
		r.ParseForm()
		unit := 0
		enc := json.NewEncoder(w)
		if ch, err := strconv.Atoi(r.FormValue("channel")); err == nil {
			if unit, err = strconv.Atoi(r.FormValue("unit")); err != nil {
				unit = 0
			}
			e := del[unit].ValueBuffer[ch]
			data := make([]interface{}, 0, 100)
			for i := 0; i < 100 && e.Value != nil; i++ {
				data = append(data, []interface{}{int64(e.Value.(ChannelData).Timestamp.UnixNano()/1000/1000) + int64(zone_offset*1000), e.Value.(ChannelData).Value})
				e = e.Prev()
			}
			enc.Encode(data)
			data = nil
		}
	}))

	err := http.ListenAndServe(":12345", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

}

func printfAt(x int, y int, format string, a ...interface{}) {
	s := ""
	if len(a) > 0 {
		s = fmt.Sprintf(format, a...)
	} else {
		s = format
	}
	for _, r := range s {
		termbox.SetCell(x, y, r, termbox.ColorDefault, termbox.ColorDefault)
		x++
	}
	termbox.Flush()
}

func main() {
	file, err := os.OpenFile("web.log", os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	log.SetOutput(file)

	units := 2
	unit_address := [2]string{"192.168.251.252:1034", "192.168.251.253:1034"}

	slow_buffer := make([][]*ring.Ring, units)
	std_dev := make([][]*ring.Ring, units)
	del := make([]*DelphinReceiver, units)

	for d := 0; d < units; d++ { // Initilize buffers and start collectors
		del[d] = NewDelphinReceiver(unit_address[d])

		slow_buffer[d] = make([]*ring.Ring, 31)
		std_dev[d] = make([]*ring.Ring, 31)
		for i := 0; i < 31; i++ {
			slow_buffer[d][i] = ring.New(SLOW_BUFFER_SIZE)
			std_dev[d][i] = ring.New(SLOW_BUFFER_SIZE)
		}
		LoadRingBuffer(slow_buffer[d], fmt.Sprintf("slow_buffer%d", d))
		LoadRingBuffer(std_dev[d], fmt.Sprintf("std_dev%d", d))

		go DatabaseCollector(slow_buffer[d], 1)
		go del[d].Start()
	}
	go func() {
		c := time.Tick(1 * time.Minute)
		for now := range c {
			for ds := 0; ds < units; ds++ { // Initilize buffers and start collectors
				SaveRingBuffer(slow_buffer[ds], fmt.Sprintf("slow_buffer%d", ds))
				SaveRingBuffer(std_dev[ds], fmt.Sprintf("std_dev%d", ds))
				log.Printf("Saved buffers in %v", time.Now().Sub(now))
			}
		}
	}()
	go httpserver(del, slow_buffer, std_dev)
	time.Sleep(5 * time.Second)
	/*	err = termbox.Init()
		if err != nil {
			panic(err)
		}
		defer termbox.Close()

		go func() {
			t := time.NewTicker(1000 * time.Millisecond)
			for {
				for d := 0; d < units; d++ {
					if del[d].ValueBufferRaw != nil {
						for i := 0; i < 31; i++ {
							if del[d].ValueBuffer[i] != nil && del[d].ValueBuffer[i].Value != nil {
								printfAt(i%2*20+(40*(d+1)), i/2, "%2d: %12.5f", i, del[d].ValueBuffer[i].Value.(ChannelData).Value)
							} else {
								printfAt(i%2*20+(40*(d+1)), i/2, "%2d: No data ...", i)

							}
						}
					} else {
						printfAt(0, 31+d, "No data for unit %d", d)
					}
				}
				<-t.C
			}
		}()
	*/
	//loop:
	for {
		/*		switch ev := termbox.PollEvent(); ev.Type {
				case termbox.EventKey:
					printfAt(0, 31, "%#v", ev)
					switch ev.Key {
					case termbox.KeyEsc:
						now := time.Now()
						for d := 0; d < units; d++ {
							SaveRingBuffer(slow_buffer[d], fmt.Sprintf("slow_buffer%d", d))
							SaveRingBuffer(std_dev[d], fmt.Sprintf("std_dev%d", d))
						}
						log.Printf("Saved buffers in %v on exit", time.Now().Sub(now))
						break loop
					}
				}
		*/
		time.Sleep(1 * time.Second)
	}
}
