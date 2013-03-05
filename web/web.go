// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tool for directly communicating with Delphin ExpertKey DAQs

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

func httpserver(d *DelphinReceiver) {
	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/json/slow", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		enc := json.NewEncoder(w)
		requested_values := 100
		if ch, err := strconv.Atoi(r.FormValue("channel")); err == nil {
			if requested_values, err = strconv.Atoi(r.FormValue("values")); err != nil {
				requested_values = 100
			}
			data := make([]interface{}, 0, requested_values)
			e := d.ValueBufferFiltered[ch]
			for i := 0; i < requested_values && e.Value != nil; i++ {
				data = append(data, []interface{}{int64(e.Value.(ChannelData).Timestamp.UnixNano() / 1000 / 1000), int64(e.Value.(ChannelData).Value + 0.5)})
				e = e.Prev()
			}
			enc.Encode(map[string]interface{}{"values": data, "error": false})
			data = nil
		} else {
			enc.Encode(map[string]interface{}{"error": true, "error_msg": "No channel defined", "error_num": 440})
		}
	})
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
	d.ReductionFactor = 100

	//Print the 25 last values for channel 5.. Just testing.
	go func() {
		t := time.NewTicker(10000 * time.Millisecond)
		for {
			if d.ValueBufferRaw != nil {
				if d.ValueBufferRaw[5] != nil {
					r := d.ValueBufferRaw[5]
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
			if d.ValueBufferFiltered != nil {
				if d.ValueBufferFiltered[5] != nil {
					r := d.ValueBufferFiltered[5]
					for i := 0; i < 10; i++ {
						if r.Value != nil {
							fmt.Printf("%12.5f+", r.Value.(ChannelData).Value)
							r = r.Prev()
						} else {
							break
						}

					}
					fmt.Printf(" F\n")
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
