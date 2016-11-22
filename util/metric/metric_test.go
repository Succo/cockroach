// Copyright 2015 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
//
// Author: Tobias Schottdorf (tobias.schottdorf@gmail.com)

package metric

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/util/hlc"
	_ "github.com/cockroachdb/cockroach/util/log" // for flags
)

func testMarshal(t *testing.T, m json.Marshaler, exp string) {
	if b, err := m.MarshalJSON(); err != nil || !bytes.Equal(b, []byte(exp)) {
		t.Fatalf("unexpected: err=%v\nbytes=%s\nwanted=%s\nfor:\n%+v", err, b, exp, m)
	}
}

var emptyMetadata = Metadata{Name: ""}

func TestGauge(t *testing.T) {
	g := NewGauge(emptyMetadata)
	g.Update(10)
	if v := g.Value(); v != 10 {
		t.Fatalf("unexpected value: %d", v)
	}
	testMarshal(t, g, "10")
}

func TestGaugeFloat64(t *testing.T) {
	g := NewGaugeFloat64(emptyMetadata)
	g.Update(10.4)
	if v := g.Value(); v != 10.4 {
		t.Fatalf("unexpected value: %f", v)
	}
	testMarshal(t, g, "10.4")
}

func TestCounter(t *testing.T) {
	c := NewCounter(emptyMetadata)
	c.Inc(100)
	c.Dec(10)
	if v := c.Count(); v != 90 {
		t.Fatalf("unexpected value: %d", v)
	}

	testMarshal(t, c, "90")
}

func TestHistogramRotate(t *testing.T) {
	manual := hlc.NewManualClock(0)
	clock := hlc.NewClock(manual.UnixNano)
	h := NewHistogram(emptyMetadata, histWrapNum*time.Second, 1000+10*histWrapNum, 3, clock)
	var cur int64
	for i := 0; i < 3*histWrapNum; i++ {
		v := int64(10 * i)
		h.RecordValue(v)
		cur += 1000000000
		manual.Set(cur)
		clock = hlc.NewClock(manual.UnixNano)
		h.testingSetClock(clock)
		cur := h.Current()

		// When i == histWrapNum-1, we expect the entry from i==0 to move out
		// of the window (since we rotated for the histWrapNum'th time).
		expMin := int64((1 + i - (histWrapNum - 1)) * 10)
		if expMin < 0 {
			expMin = 0
		}

		if min := cur.Min(); min != expMin {
			t.Fatalf("%d: unexpected minimum %d, expected %d", i, min, expMin)
		}

		if max, expMax := cur.Max(), v; max != expMax {
			t.Fatalf("%d: unexpected maximum %d, expected %d", i, max, expMax)
		}
	}
}

func TestHistogramJSON(t *testing.T) {
	manual := hlc.NewManualClock(0)
	clock := hlc.NewClock(manual.UnixNano)
	h := NewHistogram(emptyMetadata, 0, 1, 3, clock)
	testMarshal(t, h, `[{"Quantile":100,"Count":0,"ValueAt":0}]`)
	h.RecordValue(1)
	testMarshal(t, h, `[{"Quantile":0,"Count":1,"ValueAt":1},{"Quantile":100,"Count":1,"ValueAt":1}]`)
}

func TestRateRotate(t *testing.T) {
	manual := hlc.NewManualClock(0)
	clock := hlc.NewClock(manual.UnixNano)
	const interval = 10 * time.Second
	r := NewRate(emptyMetadata, interval, clock)

	// Skip the warmup phase of the wrapped EWMA for this test.
	for i := 0; i < 100; i++ {
		r.wrapped.Add(0)
	}

	// Put something nontrivial in.
	r.Add(100)

	for cur := int64(0); cur < 10; cur++ {
		prevVal := r.Value()
		manual.Set(cur * 10 * 1000000000)
		clock = hlc.NewClock(manual.UnixNano)
		r.testingSetClock(clock)
		curVal := r.Value()
		expChange := (time.Duration(cur*500) % time.Second) != 0
		hasChange := prevVal != curVal
		if expChange != hasChange {
			t.Fatalf("%v: expChange %t, hasChange %t (from %v to %v)",
				cur, expChange, hasChange, prevVal, curVal)
		}
	}

	v := r.Value()
	if v > .1 {
		t.Fatalf("final value implausible: %v", v)
	}
	expBytes, _ := json.Marshal(v)
	testMarshal(t, r, string(expBytes))
}
