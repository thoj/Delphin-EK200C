// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tool for directly communicating with Delphin ExpertKey DAQs

package main

import (
	"container/ring"
	"encoding/gob"
	"fmt"
	"log"
	"os"
)

//Functions for saving and loading ring buffers. Really slow and hacky.

func SaveRingBuffer(buf []*ring.Ring, prefix string) {
	for i := 0; i < 31; i++ {
		fh, err := os.OpenFile(fmt.Sprintf("data/%s%d.bin", prefix, i), os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			panic(err)
		}
		e := buf[i]
		en := gob.NewEncoder(fh)
		for i := 0; i < SLOW_BUFFER_SIZE && e.Value != nil; i++ {
			en.Encode(e.Value)
			e = e.Prev()
		}
		fh.Close()
	}
}

func LoadRingBuffer(buf []*ring.Ring, prefix string) {
	for i := 0; i < 31; i++ {
		fh, err := os.Open(fmt.Sprintf("data/%s%d.bin", prefix, i))
		if err != nil {
			log.Printf("%s", err)
			continue
		}
		e := buf[i]
		en := gob.NewDecoder(fh)
		x := 0
		var cd ChannelData
		for x = 0; x < SLOW_BUFFER_SIZE; x++ {
			err = en.Decode(&cd)
			if err != nil {
				log.Printf("%s", err)
				break
			}
			e.Value = cd
			e = e.Prev()
		}
		log.Printf("Loaded %d Datapoints for Channel %d", x, i)
		fh.Close()
	}
}
