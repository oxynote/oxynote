package metricutil

import (
	"context"
	"testing"
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

func Test_CollectRuntimeMetrics(t *testing.T) {
	var f factory

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(time.Second)
		cancel()
	}()

	assert.NotPanics(t, func() {
		f.CollectRuntimeMetrics(ctx, time.Millisecond*50)
	})
}
