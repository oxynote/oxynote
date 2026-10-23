package metricutil

import (
	"context"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_newRuntimeMetrics(t *testing.T) {
	var f factory

	rm := newRuntimeMetrics(&f)
	assert.NotNil(t, rm)
	assert.NotNil(t, rm.memory)
	assert.NotNil(t, rm.memory.Frees)
	assert.NotNil(t, rm.memory.HeapAlloc)
	assert.NotNil(t, rm.memory.HeapIdle)
	assert.NotNil(t, rm.memory.HeapInuse)
	assert.NotNil(t, rm.memory.HeapObjects)
	assert.NotNil(t, rm.memory.LastGC)
	assert.NotNil(t, rm.memory.Lookups)
	assert.NotNil(t, rm.memory.Mallocs)
	assert.NotNil(t, rm.memory.NextGC)
	assert.NotNil(t, rm.memory.NumGC)
	assert.NotNil(t, rm.memory.Pause)
	assert.NotNil(t, rm.memory.PauseTotal)
	assert.NotNil(t, rm.memory.StackInuse)
	assert.NotNil(t, rm.memory.TotalAlloc)
	assert.NotNil(t, rm.NumGoroutine)
	assert.NotNil(t, rm.NumThread)
}

func Test_factory_CollectRuntimeMetrics(t *testing.T) {
	var f factory

	// the bubble's clock moves only while every goroutine in it is blocked.
	// So the deadline lets exactly one tick run and no real time passes.
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*75)
		defer cancel()

		assert.NotPanics(t, func() {
			f.CollectRuntimeMetrics(ctx, time.Millisecond*50)
		})
	})
}

func Test_observeGCPauses(t *testing.T) {
	// the runtime records pauses in a 256-entry circular buffer, so
	// the case data is expressed through buffer indexes.
	cc := map[string]struct {
		PauseNs   [256]uint64
		NumGC     uint32
		PrevNumGC uint32
		Result    []float64
	}{
		"No new garbage collections": {
			PauseNs:   [256]uint64{3: 1},
			NumGC:     3,
			PrevNumGC: 3,
		},
		"Pauses within a single window": {
			PauseNs:   [256]uint64{2: 1, 3: 2, 4: 3},
			NumGC:     5,
			PrevNumGC: 2,
			Result:    []float64{1e-9, 2e-9, 3e-9},
		},
		"Pauses wrapping the buffer": {
			PauseNs:   [256]uint64{254: 1, 255: 2, 0: 3, 1: 4},
			NumGC:     258,
			PrevNumGC: 254,
			Result:    []float64{1e-9, 2e-9, 3e-9, 4e-9},
		},
		"Pause beyond the uint64 overflow point": {
			PauseNs:   [256]uint64{2: 20 * uint64(time.Second)},
			NumGC:     3,
			PrevNumGC: 2,
			Result:    []float64{20},
		},
		"Full buffer of pauses": {
			PauseNs:   [256]uint64{0: 1},
			NumGC:     300,
			PrevNumGC: 0,
			Result: append(
				[]float64{1e-9},
				make([]float64, 255)...,
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ob := &observerStub{}
			memStats := &runtime.MemStats{
				NumGC:   c.NumGC,
				PauseNs: c.PauseNs,
			}

			observeGCPauses(ob, memStats, c.PrevNumGC)
			assert.Equal(t, c.Result, ob.values)
		})
	}
}

// observerStub records observed values for assertions.
type observerStub struct {
	values []float64
}

// Observe records the observed value.
func (ob *observerStub) Observe(value float64) {
	ob.values = append(ob.values, value)
}
