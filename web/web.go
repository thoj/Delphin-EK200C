// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tool for directly communicating with Delphin ExpertKey DAQs
	
//Identified request types.
//NOTE: The unit ignores header and data CRCs.
//request := &DelphinHeader{2,0x01,0,0,0,0,0} //Request streaming data
//request := &DelphinHeader{2,0x44,0,0,0,0,0} //Request unit information
//request := &DelphinHeader{2,0x48,0,0,0,0,0} //Request calib info
//request := &DelphinHeader{2,0x50,0,0,0,0,0} //No idea what this returns. Maybe chan info.
//request := &DelphinHeader{2,0x20,0,0,0,0,0} //+ Data. Not completly sure. 
				              //Probably a ping since it returns the same data you send.

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"unsafe"
)

type DelphinHeader struct {
	Ver int16
	Com int16

	Len int32
	Pad int32
	Seq int32

	DataCheck   int32
	HeaderCheck int32
}

type DelphinRawData struct {
	Timestamp int32
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
	//	cd := make(chan DelphinChannelData, 100)
	//	go DelphinStreamer(cd)
	//	for {
	//	}
	// 00000000  00 02 00 01 00 00 00 00  00 00 00 00 00 00 00 07  |................|
	// 00000010  00 00 00 00 f8 07 0a fb                           |........|

	fmt.Printf("%d\n", unsafe.Sizeof(DelphinHeader{}))
//	request := []byte{0x00, 0x02, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x07, 0x0a, 0xfb}

//	buf := bytes.NewBuffer(request)

	var header DelphinHeader
	//err := binary.Read(buf, binary.BigEndian, &header)
	//if err != nil {
	//	fmt.Printf("%s\n", err)
	//}
	//fmt.Printf("%#v\n", header)
	conn, err := net.Dial("tcp", "192.168.251.252:1034")
	if err != nil {
		fmt.Printf("%s", err)
		return
	}
	fmt.Printf("Connected...\n")
	
	
	

	binary.Write(conn, binary.BigEndian, request)
	//_, err = conn.Write(request)
	for { 
		binary.Read(conn, binary.BigEndian, &header)
		if err != nil {
			fmt.Printf("%s", err)
			return
		}
		fmt.Printf("%#v\n", header)

		if header.Len > 0 {
			buf := make([]byte, header.Len)
			for nn := int32(0);  nn < header.Len; {
				kk, err := conn.Read(buf[nn:])
				if err != nil {
					fmt.Printf("%s", err)
					return 
				}
				nn += int32(kk)
			}
		}
	}

}
