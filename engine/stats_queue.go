/*
Rating system designed to be used in VoIP Carriers World
Copyright (C) 2013 ITsysCOM

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package engine

import (
	"strings"
	"sync"
	"time"

	"github.com/cgrates/cgrates/utils"
)

type StatsQueue struct {
	cdrs    []*QCdr
	conf    *CdrStats
	metrics map[string]Metric
	mux     sync.Mutex
}

// Simplified cdr structure containing only the necessary info
type QCdr struct {
	SetupTime  time.Time
	AnswerTime time.Time
	Usage      time.Duration
	Cost       float64
}

func NewStatsQueue(conf *CdrStats) *StatsQueue {
	if conf == nil {
		return &StatsQueue{metrics: make(map[string]Metric)}
	}
	sq := &StatsQueue{}
	sq.UpdateConf(conf)
	return sq
}

func (sq *StatsQueue) UpdateConf(conf *CdrStats) {
	sq.conf = conf
	sq.metrics = make(map[string]Metric, len(conf.Metrics))
	for _, m := range conf.Metrics {
		metric := CreateMetric(m)
		if metric != nil {
			sq.metrics[m] = metric
		}
	}
}

func (sq *StatsQueue) AppendCDR(cdr *utils.StoredCdr) {
	sq.mux.Lock()
	defer sq.mux.Unlock()
	if sq.conf.AcceptCdr(cdr) {
		qcdr := sq.simplifyCdr(cdr)
		sq.cdrs = append(sq.cdrs, qcdr)
		sq.addToMetrics(qcdr)
		sq.purgeObsoleteCdrs()
		// check for trigger
		stats := sq.getStats()
		sq.conf.Triggers.Sort()
		for _, at := range sq.conf.Triggers {
			if at.MinQueuedItems > 0 && len(sq.cdrs) < at.MinQueuedItems {
				continue
			}
			if strings.HasPrefix(at.ThresholdType, "*min_") {
				if value, ok := stats[at.ThresholdType[len("*min_"):]]; ok {
					if value <= at.ThresholdValue {
						at.Execute(nil, sq)
					}
				}
			}
			if strings.HasPrefix(at.ThresholdType, "*max_") {
				if value, ok := stats[at.ThresholdType[len("*max_"):]]; ok {
					if value >= at.ThresholdValue {
						at.Execute(nil, sq)
					}
				}
			}
		}
	}
}

func (sq *StatsQueue) addToMetrics(cdr *QCdr) {
	for _, metric := range sq.metrics {
		metric.AddCdr(cdr)
	}
}

func (sq *StatsQueue) removeFromMetrics(cdr *QCdr) {
	for _, metric := range sq.metrics {
		metric.RemoveCdr(cdr)
	}
}

func (sq *StatsQueue) simplifyCdr(cdr *utils.StoredCdr) *QCdr {
	return &QCdr{
		SetupTime:  cdr.SetupTime,
		AnswerTime: cdr.AnswerTime,
		Usage:      cdr.Usage,
		Cost:       cdr.Cost,
	}
}

func (sq *StatsQueue) purgeObsoleteCdrs() {
	if sq.conf.QueueLength > 0 {
		currentLength := len(sq.cdrs)
		if currentLength > sq.conf.QueueLength {
			for _, cdr := range sq.cdrs[:currentLength-sq.conf.QueueLength] {
				sq.removeFromMetrics(cdr)
			}
			sq.cdrs = sq.cdrs[currentLength-sq.conf.QueueLength:]
		}
	}
	if sq.conf.TimeWindow > 0 {
		for i, cdr := range sq.cdrs {
			if time.Now().Sub(cdr.SetupTime) > sq.conf.TimeWindow {
				sq.removeFromMetrics(cdr)
				continue
			} else {
				if i > 0 {
					sq.cdrs = sq.cdrs[i:]
				}
				break
			}
		}
	}
}

func (sq *StatsQueue) GetStats() map[string]float64 {
	sq.mux.Lock()
	defer sq.mux.Unlock()
	sq.purgeObsoleteCdrs()
	return sq.getStats()
}

func (sq *StatsQueue) getStats() map[string]float64 {
	stat := make(map[string]float64, len(sq.metrics))
	for key, metric := range sq.metrics {
		stat[key] = metric.GetValue()
	}
	return stat
}