// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"container/ring"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	influx "github.com/influxdb/influxdb/client"
	"log"
	"time"
)

func InfluxCollector(buf []*ring.Ring, unit int) {
	c, err := influx.NewClient(&influx.ClientConfig{Database: "voltlog"})

	t := time.Tick(1 * time.Second)
	for {
		<-t
		series := []*influx.Series{}
		for i := 0; i < 31; i++ {
			if buf[i] == nil || buf[i].Value == nil {
				continue
			}
			points := [][]interface{}{
				{buf[i].Value.(ChannelData).Timestamp.UnixNano() / 1000000, buf[i].Value.(ChannelData).Value},
			}
			s := &influx.Series{
				Name:    fmt.Sprintf("d%02dc%02d", unit, i),
				Columns: []string{"time", "value"},
				Points:  points}
			series = append(series, s)
		}
		if err = c.WriteSeries(series); err != nil {
			log.Println(err)
		}
	}

}

func DatabaseCollector(buf []*ring.Ring, unit int) {
	db, err := sql.Open("mysql", "vmr:vmr@tcp(192.168.0.13:3306)/ovnsvolt")
	if err != nil {
		panic(err)
	}

	stmtIns, err := db.Prepare("INSERT INTO fast (time,value,type,ch,unit) VALUES(?,?,?,?,?)")
	if err != nil {
		panic(err)
	}

	c := time.Tick(10 * time.Minute)
	for {
		now := <-c
		for i := 0; i < 31; i++ {
			e := buf[i]
			var max, min, avg, last ChannelData
			min.Value = 10000
			max.Value = -10000
			x := 0
			for x = 0; x < SLOW_BUFFER_SIZE && e.Value != nil; x++ {
				cd := e.Value.(ChannelData)
				if buf[i].Value.(ChannelData).Timestamp.Sub(cd.Timestamp) > (10 * time.Minute) {
					break
				}
				if cd.Value < min.Value {
					min = cd
				}
				if cd.Value > max.Value {
					max = cd
				}
				if x == 0 {
					avg.Timestamp = cd.Timestamp
					last = cd
				}
				avg.Value = avg.Value + cd.Value
				e = e.Prev()
			}
			stmtIns.Exec(min.Timestamp, min.Value, 1, i, unit)
			stmtIns.Exec(max.Timestamp, max.Value, 2, i, unit)
			stmtIns.Exec(avg.Timestamp, float64(avg.Value)/float64(x), 3, i, unit)
			stmtIns.Exec(last.Timestamp, last.Value, 4, i, unit)
		}
		log.Printf("Saved values to db in %v", time.Now().Sub(now))
	}
}
