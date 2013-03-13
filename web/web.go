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
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	STD_DEV_RED = 100
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

func httpserver(d *DelphinReceiver) {

	_, zone_offset := time.Now().Zone() //Javascript is dumb

	slow_buffer := make([]*ring.Ring, 31)
	std_dev := make([]*ring.Ring, 31)
	for i := 0; i < 31; i++ {
		slow_buffer[i] = ring.New(1440)
		std_dev[i] = ring.New(1440)
	}

	//Sample buffer evey 10 seconds
	/*
		go func() {
			t := time.NewTicker(10000 * time.Millisecond)
			for {
				for i := 0; i < 31; i++ {
					slow_buffer[i] = slow_buffer[i].Next()
					slow_buffer[i].Value = d.ValueBuffer[i].Value
				}
				<-t.C
			}
		}()
	*/
	//Calculate x sec average 
	//Calculate standard deviation every 10 seconds
	go func() {
		t := time.NewTicker(10000 * time.Millisecond)
		for {
			for i := 0; i < 31; i++ {
				avg := float64(0)
				num := doNr(d.ValueBuffer[i], STD_DEV_RED, func(e interface{}) { avg += e.(ChannelData).Value })
				if num > 0 {
					avg = avg / float64(num)
					last := d.ValueBuffer[i].Value.(ChannelData).Timestamp
					std := float64(0)
					num2 := doNr(d.ValueBuffer[i], num, func(e interface{}) { std += math.Pow(e.(ChannelData).Value-avg, 2) })
					std = math.Sqrt(std / float64(num2))
					std_dev[i] = std_dev[i].Next()
					std_dev[i].Value = ChannelData{last, std}
					slow_buffer[i] = slow_buffer[i].Next()
					slow_buffer[i].Value = ChannelData{last, avg}
					fmt.Printf("%d %d %f %f\n", num, num2, avg, std)
				}
			}
			<-t.C
		}
	}()

	fmt.Printf("Zone offsett = %d\n", zone_offset)

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/json/slow", gzHandler(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		enc := json.NewEncoder(w)
		requested_values := 100
		if ch, err := strconv.Atoi(r.FormValue("channel")); err == nil {
			if requested_values, err = strconv.Atoi(r.FormValue("values")); err != nil {
				requested_values = 100
			}
			if requested_values > 1440 {
				requested_values = 1440
			}
			data := make([]interface{}, 0, requested_values)
			e := slow_buffer[ch]
			for i := 0; i < requested_values && e.Value != nil; i++ {
				data = append(data, []interface{}{int64(e.Value.(ChannelData).Timestamp.UnixNano()/1000/1000) + int64(zone_offset*1000), int64(e.Value.(ChannelData).Value + 0.5)})
				e = e.Prev()
			}
			stddev := make([]interface{}, 0, requested_values)
			e = std_dev[ch]
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
	http.HandleFunc("/json/fast", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
	})

	err := http.ListenAndServe(":12345", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

}

func main() {
	d := NewDelphinReceiver("192.168.251.252:1034")
	d.ReductionFactor = 10

	//Print the 25 last values for channel 5.. Just testing.
	go func() {
		t := time.NewTicker(1000 * time.Millisecond)
		for {
			if d.ValueBufferRaw != nil {
				if d.ValueBuffer[22] != nil {
					r := d.ValueBuffer[22]
					for i := 0; i < 10; i++ {
						if r.Value != nil {
							fmt.Printf("%12.5f+", r.Value.(ChannelData).Value)
							r = r.Prev()
						} else {
							break
						}

					}
					fmt.Printf(" R\n")
				}
			} else {
				fmt.Printf("No data\n")
			}
			<-t.C
		}
	}()
	go httpserver(d)
	d.Start()

}
