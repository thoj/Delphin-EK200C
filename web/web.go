// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tool for directly communicating with Delphin ExpertKey DAQs

//Identified request types.
//NOTE: The unit ignores header and data CRCs.
//&DelphinHeader{2,0x01,0,0,0,0,0} //Request streaming data
//&DelphinHeader{2,0x44,0,0,0,0,0} //Request unit information
//&DelphinHeader{2,0x40,0,0,0,0,0} //Request calib info
//&DelphinHeader{2,0x48,0,0,0,0,0} //Request calib info (Base64 Encoded?)

//&DelphinHeader{2,0x50,0,0,0,0,0} //No idea what this returns. Maybe chan info.

//&DelphinHeader{2,0x20,0x0c,0,0,0,0} //Not sure. Some kind of sync? Looks like there is a timer / counter
// Data Examples:                         Time
// -> 00 00 00 00 80 ad 81 1e 53 46 80 0e (22ms)
// <- 5e 61 45 64 80 ad 81 1e 53 46 80 0e (25ms)
// -> 00 00 00 00 00 f4 8b 2a 53 46 80 0e (228ms)
// <- 5e 64 6a 41 00 f4 8b 2a 53 46 80 0e (231ms)
// -> 00 00 00 00 40 d1 d6 49 53 46 80 0e (753ms)
// <- 5e 6c 6d 2b 40 d1 d6 49 53 46 80 0e (794ms)
// -> 00 00 00 00 c0 a1 39 82 53 46 80 0e (1699ms)
// <- 5e 7a da 73 c0 a1 39 82 53 46 80 0e (1701ms)
// -> 00 00 00 00 40 80 c3 c1 53 46 80 0e (2765ms)
// <- 5e 8b 1f 15 40 80 c3 c1 53 46 80 0e (2794ms)
// -> 00 00 00 00 80 0e 17 fa 53 46 80 0e (3710ms)
// <- 5e 99 8a b0 80 0e 17 fa 53 46 80 0e (3712ms)
// -> 00 00 00 00 40 0b bc 38 54 46 80 0e (4761ms)
// <- 5e a9 93 bf 40 0b bc 38 54 46 80 0e (4794ms)

//&DelphinHeader{2,0x2a,0x08,0,0,0,0} //First Packet. Data = 03 01 00 13 00 02 00 00
//Empty Packet returend

//&DelphinHeader{2,0x07,0x08,0,0,0,0} //Unit info. Firmware version ++

package main

import (
	"bytes"
	"container/ring"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"
)

const (
	RAW_MAX = 2084935581 //2**32/1.03/2 (Signed)
	ENG_MAX = 10000
	ENG_MIN = -10000

	BUFFER_SIZE = 10000 //100 Sec at 100Hz
)

type DelphinHeader struct {
	Ver int16
	Com int16

	Len   int32
	Param int32
	Seq   int32

	DataCheck   int32
	HeaderCheck int32
}

type DelphinRawData struct {
	Timestamp uint32
	ChanValue int32 //Channel (6bit) + Value (26bit @ 1hz, 22 bit @ 50Hz)
}

//Raw data value and raw metadata
type DelphinChannelData struct {
	PacketTime time.Time
	Timestamp  uint32
	Channel    uint8
	RawValue   int32 //Raw 
	Last       bool
}

//Useful information
type ChannelData struct {
	Channel   uint8
	Timestamp time.Time
	Value     float64
}

// Keep updated rolling valuie buffer
func valueBuffer(cin chan ChannelData, buffers []*ring.Ring) {
	for {
		v := <-cin
		if buffers[v.Channel] == nil {
			buffers[v.Channel] = ring.New(BUFFER_SIZE)
		}
		buffers[v.Channel] = buffers[v.Channel].Next()
		buffers[v.Channel].Value = v
	}
}

//Correct Timestamp and Engineering Value
func valueCalc(cin chan DelphinChannelData, cout chan ChannelData, adj []AdjustmentTable) {
	ts0 := uint32(0)
	td := uint64(0)
	sync := true
	var at0 time.Time
	m := 0
	for {
		i := <-cin

		o := ChannelData{}

		//Calculate and Adjust Engineering value
		o.Value = adjustValue(float64(float64(i.RawValue)/RAW_MAX)*ENG_MAX, adj[i.Channel])

		//Convert timestamp to Absolute timestamp
		//Note: This timestamp can be sligltly in the future. (100ms) 
		//The timstamp relative to other mesurements is more important	
		//There is about 70ms drift over an hour on my unit. 
		//Sync every 100k mesurments, this causes some jitter due to network latency (+-1ms)
		//Somekind of incremental adjustment would be better 

		if sync {
			sync = false
			at0 = i.PacketTime
			td = 0
			ts0 = i.Timestamp
		}

		if ts0 > i.Timestamp {
			td += (1 << 32) - uint64(ts0)
			td += uint64(i.Timestamp)
			ts0 = i.Timestamp
		} else {
			td += uint64(i.Timestamp - ts0)
			ts0 = i.Timestamp
		}
		o.Timestamp = at0.Add(time.Duration(td * 1000))
		o.Channel = i.Channel
		cout <- o

		if m > 100000 {
			if i.Last {
				sync = true
				m = 0
			}
		}
		m++
	}
}

// ADC Correction
func adjustValue(v float64, adj AdjustmentTable) float64 {
	if adj.Orders == 4 {
		return adj.Order[0] + adj.Order[1]*v + adj.Order[2]*math.Pow10(-9)*math.Pow(v, 2) + adj.Order[3]*math.Pow10(-13)*math.Pow(v, 3)
	} else if adj.Orders == 3 {
		return adj.Order[0] + adj.Order[1]*v + adj.Order[2]*math.Pow10(-9)*math.Pow(v, 2)
	} else if adj.Orders == 2 {
		return adj.Order[0] + adj.Order[1]*v
	} else if adj.Orders == 1 {
		return adj.Order[0] + v
	}
	return v
}

func readPacket(conn net.Conn) (DelphinHeader, []byte, error) {
	var header DelphinHeader
	err := binary.Read(conn, binary.BigEndian, &header)
	if err != nil {
		return header, nil, err
	}
	if header.Len > 0 {
		buf := make([]byte, header.Len)
		for nn := int32(0); nn < header.Len; {
			kk, err := conn.Read(buf[nn:])
			if err != nil {
				return header, nil, err
			}
			nn += int32(kk)
		}
		return header, buf, nil
	}
	return header, nil, nil
}

func main() {

	//Queues
	calc_chan := make(chan DelphinChannelData, 100) //Value calculations
	buffer_chan := make(chan ChannelData, 100)      //Value buffer

	conn, err := net.Dial("tcp", "192.168.251.252:1034")
	if err != nil {
		fmt.Printf("%s", err)
		return
	}
	fmt.Printf("Connected...\n")

	//Send initial request for Init, Calib Data and Streaming
	request := &DelphinHeader{2, 0x2a, 0x08, 0, 0, 0, 0}
	binary.Write(conn, binary.BigEndian, request)
	conn.Write([]byte{0x03, 0x01, 0x00, 0x13, 0x00, 0x02, 0x00, 0x00}) // Init
	request = &DelphinHeader{2, 0x48, 0x00, 0, 2, 0, 0}
	binary.Write(conn, binary.BigEndian, request)
	request = &DelphinHeader{2, 0x01, 0, 0, 3, 0, 0}
	binary.Write(conn, binary.BigEndian, request)

	//Send "Ping" Packets
	go func() {
		t := time.NewTicker(time.Second)
		i := int32(4)
		for {
			syncreq := &DelphinHeader{2, 0x20, 0x0c, 0, i, 0, 0}
			tn := time.Now()
			binary.Write(conn, binary.BigEndian, syncreq)
			conn.Write([]byte{0, 0, 0, 0})
			binary.Write(conn, binary.BigEndian, tn.UnixNano())
			i++
			<-t.C
		}
	}()

	value_buffer := make([]*ring.Ring, 31)          //Should be faster and smaller then a map
	adj_table := make([]AdjustmentTable, 31)        //Should be faster and smaller then a map
	go valueCalc(calc_chan, buffer_chan, adj_table) // Send calculated values to buffer
	go valueBuffer(buffer_chan, value_buffer)       // Buffer Storage

	//Print the 25 last values for channel 5.. Just testing.
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		for {
			if value_buffer[5] != nil {
				r := value_buffer[5]
				for i := 0; i < 25; i++ {
					if r.Value != nil {
						fmt.Printf("%8.2f", r.Value.(ChannelData).Value)
						r = r.Prev()
					} else {
						break
					}

				}
				fmt.Printf("\n")
			}
			<-t.C
		}
	}()

	for {
		head, data, err := readPacket(conn)
		ptime := time.Now()
		if err != nil {
			fmt.Printf("%s\n", err)
			return
		}
		switch {
		case head.Com == 128: // Channel Data
			databuf := bytes.NewBuffer(data)
			for databuf.Len() > 0 {

				var timestamp uint32
				var chanvalue int32
				var chvalue DelphinChannelData
				if databuf.Len() == 8 {
					chvalue.Last = true
				}
				binary.Read(databuf, binary.LittleEndian, &timestamp)
				binary.Read(databuf, binary.LittleEndian, &chanvalue)
				chvalue.Timestamp = timestamp
				chvalue.Channel = uint8((chanvalue >> 27) & ((1 << 5) - 1))
				chvalue.RawValue = chanvalue & ((1 << 23) - 1)
				chvalue.RawValue = chvalue.RawValue << 9
				chvalue.PacketTime = ptime
				calc_chan <- chvalue
			}
			databuf.Reset()
		case head.Com == -32696: //Calib Data
			calib, err := NewCalibration(data[4:])
			if err != nil {
				fmt.Printf("%s\n", err)
			}
			calib.AdjustmentTable(10, adj_table)
			fmt.Printf("New adjustment table:\n%3s %16s %16s %16s %16s\n", "Chan", "Order0", "Order1", "Order2", "Order3")
			for v, _ := range adj_table {
				fmt.Printf("%3d ", v)
				for _, w := range adj_table[v].Order {
					fmt.Printf("%16e ", w)
				}
				fmt.Printf("\n")
			}
		}
	}
}
