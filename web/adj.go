// Copyright 2013 Thomas Jager <mail@jager.no> All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Unmarshal XML Adustemnet data

package main

import "encoding/xml"

type ChannelAdjustment struct {
	Channel     int     `xml:"Channel,attr"`
	Range       float64 `xml:"MeasuringRange,attr"`
	Coefficient []float64
}

type Adjustment struct {
	Version  string `xml:"Version,attr"`
	Date     string `xml:"Date,attr"`
	Hardware string `xml:"Hardware,attr"`
	Software string `xml:"Software,attr"`
	Person   string `xml:"Person,attr"`
	User     string `xml:"User,attr"`
	Location string `xml:"Location,attr"`
	Data     []ChannelAdjustment
}

type Calibration struct {
	XMLName     xml.Name `xml:"Default"`
	Calibration string   //Looks like Base64 encoded but is mostly garbage
	Adjustment  Adjustment
}

type AdjustmentTable struct {
	Orders int
	Order  []float64
}

//Return simple adjustment table for range
func (c *Calibration) AdjustmentTable(r float64, adjt []AdjustmentTable) {
	for _, adj := range c.Adjustment.Data {
		if adj.Range == r {
			adjt[adj.Channel] = AdjustmentTable{}
			adjt[adj.Channel].Orders = len(adj.Coefficient)
			adjt[adj.Channel].Order = make([]float64, len(adj.Coefficient))
			for key, coeff := range adj.Coefficient {
				adjt[adj.Channel].Order[key] = coeff
			}
		}
	}
}

//Returns unmarshaled adjustment data
func NewCalibration(b []byte) (*Calibration, error) {
	adj := Calibration{}
	err := xml.Unmarshal(b, &adj)
	if err != nil {
		return nil, err
	}
	return &adj, nil
}
