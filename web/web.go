// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tool for directly communicating with Delphin ExpertKey DAQs

//Identified request types.
//NOTE: The unit ignores header and data CRCs.
//&DelphinHeader{2,0x01,0,0,0,0,0} //Request streaming data
//&DelphinHeader{2,0x44,0,0,0,0,0} //Request unit information
//&DelphinHeader{2,0x48,0,0,0,0,0} //Request calib info
//&DelphinHeader{2,0x40,0,0,0,0,0} //Request calib info (Base64 Encoded?)

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
)

var foo int

type DelphinHeader struct {
	Ver int16
	Com int16

	Len   int32
	Param int32
	Seq   int32

	DataCheck   int32
	HeaderCheck int32
}

type DelphinChannelValue struct {
	MesNumber    uint32 //Mesuremnt number in packet
	PacketTime   time.Time
	Timestamp    uint32
	Abstimestamp time.Time
	Channel      uint8
	RawValue     int32   //Raw Value 
	EngValue     float64 //Engineering value
}
type DelphinRawData struct {
	Timestamp uint32
	ChanValue int32 //Channel (6bit) + Value (26bit @ 1hz, 22 bit @ 50Hz)
}

type DelphinChannelData struct {
	Timestamp int64
	Rawvalue  int64
	Engvalue  float64
}

// Comunication with module
/*
func DelphinStreamer(cd chan DelphinChannelData) {
	request := []byte{0x02, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0xf8, 0xfb, 0x0a}
	for {
		conn, err := net.Dial("tcp", "192.168.251.252:1034")
		if err != nil {
			fmt.Printf("%s", err)
			continue
		}
		fmt.Printf("Connected...\n")
		_, err = conn.Write(request)
		if err != nil {
			fmt.Printf("Write error...%s\n", err)
			conn.Close()
			continue
		}

	}
}
*/

func main() {
	conn, err := net.Dial("tcp", "192.168.251.252:1034")

	//Queues
	value_calc := make(chan DelphinChannelValue, 100)   //Value calculations
	value_buffer := make(chan DelphinChannelValue, 100) //Value buffer

	if err != nil {
		fmt.Printf("%s", err)
		return
	}
	fmt.Printf("Connected...\n")

	request := &DelphinHeader{2, 0x2a, 0x08, 0, 0, 0, 0}
	binary.Write(conn, binary.BigEndian, request)
	conn.Write([]byte{0x03, 0x01, 0x00, 0x13, 0x00, 0x02, 0x00, 0x00}) // Init
	readPacket(conn)
	request = &DelphinHeader{2, 0x01, 0, 0, 0, 0, 0}
	binary.Write(conn, binary.BigEndian, request)
	readPacket(conn)

	go func() {
		t := time.NewTicker(time.Second)
		for {
			syncreq := &DelphinHeader{2, 0x20, 0x0c, 0, 0, 0, 0}
			tn := time.Now()
			binary.Write(conn, binary.BigEndian, syncreq)
			conn.Write([]byte{0, 0, 0, 0})
			binary.Write(conn, binary.BigEndian, tn.UnixNano())
			foo = 0

			<-t.C
		}
	}()
	go valueCalc(value_calc, value_buffer) // Send calculated values to buffer
	go valueBuffer(value_buffer)           // Buffer Storage
	for {
		head, data, err := readPacket(conn)
		ptime := time.Now()
		if err != nil {
			fmt.Printf("%s\n", err)
			return
		}
		if head.Com == 128 { // Channel Data
			databuf := bytes.NewBuffer(data)
			mesnumber := uint32(head.Len/8) - 1
			for databuf.Len() > 0 {

				var timestamp uint32
				var chanvalue int32
				var chvalue DelphinChannelValue

				binary.Read(databuf, binary.LittleEndian, &timestamp)
				binary.Read(databuf, binary.LittleEndian, &chanvalue)
				chvalue.Timestamp = timestamp
				chvalue.Channel = uint8((chanvalue >> 27) & ((1 << 5) - 1))
				chvalue.RawValue = chanvalue & ((1 << 23) - 1)
				chvalue.RawValue = chvalue.RawValue << 9
				chvalue.MesNumber = mesnumber
				mesnumber--
				chvalue.PacketTime = ptime
				value_calc <- chvalue
			}
		}
	}
}

func valueBuffer(cin chan DelphinChannelValue) {
	last := make(map[uint8]float64)
	var v0 DelphinChannelValue
	for {
		v := <-cin
		last[v.Channel] = v.EngValue
		if v.Channel == 5 || v.Channel == 4 {
			fmt.Printf("%d %35s: %5.2f (%7s) %d\n", v.Timestamp, v.Abstimestamp, v.EngValue, v.Abstimestamp.Sub(v0.Abstimestamp), v.MesNumber)
			v0 = v
		}
	}
}

//Correct Timestamp and Engineering Value
func valueCalc(cin chan DelphinChannelValue, cout chan DelphinChannelValue) {
	t0 := uint32(0)
	for {
		v := <-cin

		//Calculate and Adjust Engineering value
		v.EngValue = adjustValue(float64(float64(v.RawValue)/RAW_MAX)*ENG_MAX, v.Channel)

		//Calculate Absolute Timestamp
		td := uint64(333) //Default for 100Hz
		if t0 > 0 {
			td = uint64(v.Timestamp-t0)
		}
		tdd := ((td+24) * uint64(v.MesNumber)) * 1000 //Less Jitter with 25 extra microseconds... no idea why.
		v.Abstimestamp = v.PacketTime.Add(-time.Duration(tdd))
		t0 = v.Timestamp
		foo++
		cout <- v
	}
}

// ADC Correction curve
//TODO: get constatns from adj table!! this is only for channel 5!!
func adjustValue(v float64, c uint8) float64 {
	return 6.2053809 + 1.000379*v + 3.0443865*math.Pow10(-9)*math.Pow(v, 2) - 1.6165593*math.Pow10(-13)*math.Pow(v, 3)
}

func readPacket(conn net.Conn) (DelphinHeader, []byte, error) {
	var header DelphinHeader
	err := binary.Read(conn, binary.BigEndian, &header)
	if err != nil {
		return header, nil, err
	}
	//	fmt.Printf("%#v\n", header)
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
