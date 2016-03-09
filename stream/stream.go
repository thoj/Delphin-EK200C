// Copyright 2015 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tool for streaming data directly from Delphin ExpertKey DAQs

package main

import (
	"flag"
	"fmt"
)

var address = flag.String("address", "192.168.251.50:1034", "ip:port to ExpertKey Device")
var channel = flag.Int("channel", -1, "Only stream channel")

func main() {
	flag.Parse()
	fmt.Printf("Channel = %d\n", *channel)
	del := NewEKReceiver(*address)
	del.Stream(Stream)
}

func Stream(ek EKChannelData) {
	if (*channel > -1 && int(ek.Channel) == *channel) || *channel < 0 {
		fmt.Printf("%3d: %032b %032b %10d %10f %10d\n", ek.Channel, ek.PacketData2, ek.RawValue, ek.Timestamp, ek.Value, ek.RawValue)
	}
}
