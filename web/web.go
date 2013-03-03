// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tool for directly communicating with Delphin ExpertKey DAQs

package main

import (
	"fmt"
	"time"
)

func main() {
	d := NewDelphinReceiver("192.168.251.252:1034")
	d.ReductionFactor = 100

	//Print the 25 last values for channel 5.. Just testing.
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
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
	/*
		go func() {
			t := time.NewTicker(550 * time.Millisecond)
			for {
				if d.ValueBufferRaw != nil {
					if d.ValueBufferRaw[5] != nil {
						r := d.ValueBufferRaw[5]
						for i := 0; i < 10; i++ {
							if r.Value != nil {
								fmt.Printf("%8.2f", r.Value.(ChannelData).Value)
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
	*/

	d.Start()

}
