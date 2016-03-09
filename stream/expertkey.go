// Copyright 2015 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Protocol Library for Delphin ExpertKey DAQs

//Identified request types.
//NOTE: The unit ignores header and data CRCs.
//&EKHeader{2,0x01,0,0,0,0,0} //Request streaming data
//&EKHeader{2,0x44,0,0,0,0,0} //Request unit information
//&EKHeader{2,0x40,0,0,0,0,0} //Request calib info
//&EKHeader{2,0x48,0,0,0,0,0} //Request calib info (Base64 Encoded?)

//&EKHeader{2,0x50,0,0,0,0,0} //No idea what this returns. Maybe chan info.

//&EKHeader{2,0x20,0x0c,0,0,0,0} //Not sure. Some kind of sync? Looks like there is a timer / counter
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

//&EKHeader{2,0x2a,0x08,0,0,0,0} //First Packet. Data = 03 01 00 13 00 02 00 00
//Empty Packet returend

//&EKHeader{2,0x07,0x08,0,0,0,0} //Unit info. Firmware version ++

package main

import (
	"bytes"
	"container/ring"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"time"
)

const (
//	RAW_MAX = 2036000000 //2**32/1.03/2 (Signed)
	//RAW_MAX = 2036000000 //26 bit max?
	RAW_MAX = 30911 //26 bit max?
	//RAW_MAX = 67108864
	ENG_MAX = 10000
	ENG_MIN = -10000

	BUFFER_SIZE     = 3000 // Buffer for filtered values 300 sec @ Reduction factor = 10
	RAW_BUFFER_SIZE = 2500 //Keep raw values around for 25 seconds @ 100Hz
)

type EKReceiver struct {
	ValueBufferRaw  []*ring.Ring      //Holds raw buffer
	ValueBuffer     []*ring.Ring      //Holds filtered buffer
	AdjustmentTable []AdjustmentTable //Holds channel adjustment data

	FIRTaps    int           //For filter
	SampleTime time.Duration //Sample the filtered buffer this often

	addr        string                  //IP+Port
	calc_chan   chan EKChannelData //Value calculations
	buffer_chan chan EKChannelData //Value buffer

	at0 time.Time
	ts0 uint32

	connected  bool
	sequencenr int32
	conn       net.Conn
}

type EKHeader struct {
	Ver int16
	Com int16

	Len   int32
	Param int32
	Seq   int32

	DataCheck   int32
	HeaderCheck int32
}

type EKRawData struct {
	Timestamp uint32
	ChanValue int32 //Channel (6bit) + Value (26bit @ 1hz, 22 bit @ 50Hz)
}

//Raw data value and raw metadata
type EKChannelData struct {
	PacketTime   time.Time
	Timestamp    uint32
	Channel      uint8
	RawValue     float32 //Raw
	Value        float64
	Abstimestamp time.Time
	Last         bool //Last value in packet
	PacketData1  uint32
	PacketData2  uint32
}

//Useful information
type ChannelData struct {
	Timestamp time.Time
	Value     float64
}

// Process values coming from ADC
func valueBuffer(d *EKReceiver) {
	last_sample := make([]time.Time, 31)

	for i := 0; i < 31; i++ {
		d.ValueBufferRaw[i] = ring.New(RAW_BUFFER_SIZE)
		d.ValueBuffer[i] = ring.New(BUFFER_SIZE)
	}
	for {
		v := <-d.buffer_chan

		//Move head forwards
		d.ValueBufferRaw[v.Channel] = d.ValueBufferRaw[v.Channel].Next()
		d.ValueBufferRaw[v.Channel].Value = ChannelData{v.Abstimestamp, v.Value} //Set value

		// FIR Calculation
		p0 := d.ValueBufferRaw[v.Channel]
		i := 0
		out := float64(0)
		for ; i < d.FIRTaps && p0.Value != nil; i++ {
			out += p0.Value.(ChannelData).Value
			p0 = p0.Prev()
		}
		out = out / float64(i)
		if v.Abstimestamp.Sub(last_sample[v.Channel]) >= d.SampleTime {
			d.ValueBuffer[v.Channel] = d.ValueBuffer[v.Channel].Next()
			d.ValueBuffer[v.Channel].Value = ChannelData{v.Abstimestamp, out}
			last_sample[v.Channel] = v.Abstimestamp
		}
	}
}

//Correct Timestamp and Engineering Value
func valueCalc(d *EKReceiver) {
	td := uint64(0)
	sync := true
	m := 0
	for {
		i := <-d.calc_chan

		//Calculate and Adjust Engineering value
		i.Value = adjustValue(float64(float64(i.RawValue)/RAW_MAX)*ENG_MAX, d.AdjustmentTable[i.Channel])

		//Convert timestamp to Absolute timestamp
		//Note: This timestamp can be sligltly in the future. (100ms)
		//The timstamp relative to other mesurements is more important
		//There is about 70ms drift over an hour on my unit.
		//Sync every 100k mesurments, this causes some jitter due to network latency (+-1ms)
		//Somekind of incremental adjustment would be better

		if sync {
			sync = false
			d.at0 = i.PacketTime
			td = 0
			d.ts0 = i.Timestamp
		}

		if d.ts0 > i.Timestamp {
			td += (1 << 32) - uint64(d.ts0)
			td += uint64(i.Timestamp)
			d.ts0 = i.Timestamp
		} else {
			td += uint64(i.Timestamp - d.ts0)
			d.ts0 = i.Timestamp
		}
		i.Abstimestamp = d.at0.Add(time.Duration(td * 1000))
		d.buffer_chan <- i

		if m > 100000 {
			if i.Last {
				sync = true
				m = 0
			}
		}
		m++
	}
}


// Calculate Absolute Timestamp

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

func readPacket(conn net.Conn) (EKHeader, []byte, error) {
	var header EKHeader
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

//Send initial request for Init, Calib Data and Streaming
func (d *EKReceiver) postConnect() {
	request := &EKHeader{2, 0x2a, 0x08, 0, 0, 0, 0}
	binary.Write(d.conn, binary.BigEndian, request)
	d.conn.Write([]byte{0x03, 0x01, 0x00, 0x13, 0x00, 0x02, 0x00, 0x00}) // Init
	request = &EKHeader{2, 0x48, 0x00, 0, 2, 0, 0}
	binary.Write(d.conn, binary.BigEndian, request)
	request = &EKHeader{2, 0x01, 0, 0, 3, 0, 0}
	binary.Write(d.conn, binary.BigEndian, request)
	d.connected = true
	d.sequencenr = int32(4)
}

func (d *EKReceiver) connectEK() error {
	log.Printf("Connecting to %s...\n", d.addr)
	var err error
	d.conn, err = net.Dial("tcp", d.addr)
	if err != nil {
		log.Printf("Connection Error: %s", err)
		return err
	}
	log.Printf("Connected...\n")
	d.postConnect()
	return nil
}

//Receiver loop
func (d *EKReceiver) receiverLoop(fn func(EKChannelData)) {
	for {
		if d.conn == nil {
			return
		}
		head, data, err := readPacket(d.conn)
		ptime := time.Now()
		if err != nil {
			log.Printf("%s\n", err)
			return
		}
//		fmt.Printf("Packet: %#v\n", head)
		switch {
		case head.Com == 128: // Channel Data
			databuf := bytes.NewBuffer(data)
			//fmt.Printf("%0x\n", databuf)
			for databuf.Len() > 0 {
				var timestamp uint32
				var chanvalue uint32
				var chvalue EKChannelData
				if databuf.Len() == 8 {
					chvalue.Last = true
				}
				binary.Read(databuf, binary.LittleEndian, &timestamp)
				binary.Read(databuf, binary.LittleEndian, &chanvalue)
				chvalue.PacketData1 = timestamp;
				chvalue.PacketData2 = chanvalue;
				chvalue.Timestamp = timestamp
				chvalue.Channel = uint8(chanvalue >> 27)
				//chvalue.RawValue = int32(int16((chanvalue << 6 >> 16))) // 1hz 9 = 50hz 12 = 100hz
				chvalue.RawValue = float32(chanvalue << 6)

				chvalue.Value = adjustValue(float64(float64(chvalue.RawValue)/RAW_MAX)*ENG_MAX, d.AdjustmentTable[chvalue.Channel])
				//chvalue.Value = float64(float64(chvalue.RawValue)/RAW_MAX)*ENG_MAX
				chvalue.PacketTime = ptime
				fn(chvalue)
			}
			databuf.Reset()
		case head.Com == -32696: //Calib Data
			calib, err := NewCalibration(data[4:])
			if err != nil {
				log.Printf("%s\n", err)
			}
			calib.AdjustmentTable(10, d.AdjustmentTable) //Convert to adjustment table
			log.Printf("New adjustment table:\n%3s %16s %16s %16s %16s\n", "Chan", "Order0", "Order1", "Order2", "Order3")
			for v, _ := range d.AdjustmentTable {
				fmt.Printf("%3d ", v)
				for _, w := range d.AdjustmentTable[v].Order {
					fmt.Printf("%16e ", w)
				}
				fmt.Printf("\n")
			}
		default:
//		fmt.Printf("Not Handled: %#v\n", head)
		}
	}
}

//Start streaming data from EK device.
//Never returns. Will handle d.connection errors by red.connecting.
func (d *EKReceiver) Stream(fn func(EKChannelData)) {
	for {
		err := d.connectEK()
		if err == nil {
			d.receiverLoop(fn)
			d.conn.Close()
		}
		log.Printf("Socket Read Error... Reconnecting\n")
		time.Sleep(10000 * time.Millisecond)
	}
}

func (d *EKReceiver) foo() {
	time.Sleep(10 * time.Second)
	request := &EKHeader{2, 0x01, 0, 0, d.sequencenr, 0, 0}
	binary.Write(d.conn, binary.BigEndian, request)
	d.sequencenr = d.sequencenr + 1
}

//Init set up everything needed for receiving data.
func NewEKReceiver(addr string) *EKReceiver {
	d := new(EKReceiver)
	d.addr = addr
	d.FIRTaps = 400
	d.SampleTime = 1000 * time.Millisecond
	d.calc_chan = make(chan EKChannelData, 100)   //Value calculations
	d.buffer_chan = make(chan EKChannelData, 100) //Value buffer
	d.ValueBufferRaw = make([]*ring.Ring, 31)          //Should be faster and smaller then a map
	d.ValueBuffer = make([]*ring.Ring, 31)             //Should be faster and smaller then a map
	d.AdjustmentTable = make([]AdjustmentTable, 31)    //Should be faster and smaller then a map
	go valueCalc(d)                                    // Send calculated values to buffer
	//	go d.foo()
	go valueBuffer(d) // Buffer Storage
	//Send "Ping" Packets
	go func() {
		t := time.NewTicker(10 * time.Second)
		for {
			if d.connected {
				syncreq := &EKHeader{2, 0x20, 0x0c, 0, d.sequencenr, 0, 0}
				tn := time.Now()
				binary.Write(d.conn, binary.BigEndian, syncreq)
				d.conn.Write([]byte{0, 0, 0, 0})
				binary.Write(d.conn, binary.BigEndian, tn.UnixNano())
				d.sequencenr++
			}
			<-t.C
		}
	}()
	return d
}
