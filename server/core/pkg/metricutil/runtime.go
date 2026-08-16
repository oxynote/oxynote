package metricutil

import (
	"context"
	"runtime"
	"runtime/pprof"
	"time"
)

// _nanosecondsPerSecond is the number of nanoseconds in a second.
const _nanosecondsPerSecond = 1e9

// runtimeMetrics is a collection of runtime metrics.
type runtimeMetrics struct {
	memory runtimeMemoryMetrics

	// NumGoroutine is the number of goroutines that currently exist.
	NumGoroutine Gauge

	// NumThread is the number of OS threads created.
	NumThread Gauge
}

// runtimeMemoryMetrics is a collection of memory metrics.
type runtimeMemoryMetrics struct {
	// Frees is the cumulative count of heap objects freed.
	Frees Gauge

	// HeapAlloc is bytes of allocated heap objects.
	//
	// "Allocated" heap objects include all reachable objects, as
	// well as unreachable objects that the garbage collector has
	// not yet freed. Specifically, HeapAlloc increases as heap
	// objects are allocated and decreases as the heap is swept
	// and unreachable objects are freed. Sweeping occurs
	// incrementally between GC cycles, so these two processes
	// occur simultaneously, and as a result HeapAlloc tends to
	// change smoothly (in contrast with the sawtooth that is
	// typical of stop-the-world garbage collectors).
	HeapAlloc Gauge

	// HeapIdle is bytes in idle (unused) spans.
	//
	// Idle spans have no objects in them. These spans could be
	// (and may already have been) returned to the OS, or they can
	// be reused for heap allocations, or they can be reused as
	// stack memory.
	//
	// HeapIdle minus HeapReleased estimates the amount of memory
	// that could be returned to the OS, but is being retained by
	// the runtime so it can grow the heap without requesting more
	// memory from the OS. If this difference is significantly
	// larger than the heap size, it indicates there was a recent
	// transient spike in live heap size.
	HeapIdle Gauge

	// HeapInuse is bytes in in-use spans.
	//
	// In-use spans have at least one object in them. These spans
	// can only be used for other objects of roughly the same
	// size.
	//
	// HeapInuse minus HeapAlloc estimates the amount of memory
	// that has been dedicated to particular size classes, but is
	// not currently being used. This is an upper bound on
	// fragmentation, but in general this memory can be reused
	// efficiently.
	HeapInuse Gauge

	// HeapObjects is the number of allocated heap objects.
	//
	// Like HeapAlloc, this increases as objects are allocated and
	// decreases as the heap is swept and unreachable objects are
	// freed.
	HeapObjects Gauge

	// LastGC is the time the last garbage collection finished, as
	// nanoseconds since 1970 (the UNIX epoch).
	LastGC Gauge

	// Lookups is the number of pointer lookups performed by the
	// runtime.
	//
	// This is primarily useful for debugging runtime internals.
	Lookups Gauge

	// Mallocs is the cumulative count of heap objects allocated.
	// The number of live objects is Mallocs - Frees.
	Mallocs Gauge

	// NextGC is the target heap size of the next GC cycle.
	//
	// The garbage collector's goal is to keep HeapAlloc ≤ NextGC.
	// At the end of each GC cycle, the target for the next cycle
	// is computed based on the amount of reachable data and the
	// value of GOGC.
	NextGC Gauge

	// NumGC is the number of completed GC cycles.
	NumGC Gauge

	// Pause is a circular buffer of recent GC stop-the-world
	// pause times in seconds.
	//
	// The most recent pause is at Pause[(NumGC+255)%256]. In
	// general, Pause[N%256] records the time paused in the most
	// recent N%256th GC cycle. There may be multiple pauses per
	// GC cycle; this is the sum of all pauses during a cycle.
	Pause Observer

	// PauseTotal is the cumulative seconds in GC
	// stop-the-world pauses since the program started.
	//
	// During a stop-the-world pause, all goroutines are paused
	// and only the garbage collector can run.
	PauseTotal Gauge

	// StackInuse is bytes in stack spans.
	//
	// In-use stack spans have at least one stack in them. These
	// spans can only be used for other stacks of the same size.
	//
	// There is no StackIdle because unused stack spans are
	// returned to the heap (and hence counted toward HeapIdle).
	StackInuse Gauge

	// TotalAlloc is cumulative bytes allocated for heap objects.
	// Increases as heap objects are allocated, but
	// unlike HeapAlloc, it does not decrease when
	// objects are freed.
	TotalAlloc Gauge
}

// newRuntimeMetrics creates a new runtime metrics.
func newRuntimeMetrics(f *factory) runtimeMetrics {
	return runtimeMetrics{
		memory: runtimeMemoryMetrics{
			Frees: f.NewGauge(Options{
				Name: "go_memstats_frees",
				Help: "Cumulative count of heap objects freed.",
			}),
			HeapAlloc: f.NewGauge(Options{
				Name: "go_memstats_heap_alloc_bytes",
				Help: "Bytes of allocated heap objects.",
			}),
			HeapIdle: f.NewGauge(Options{
				Name: "go_memstats_heap_idle_bytes",
				Help: "Bytes in idle (unused) spans.",
			}),
			HeapInuse: f.NewGauge(Options{
				Name: "go_memstats_heap_inuse_bytes",
				Help: "Bytes in in-use spans.",
			}),
			HeapObjects: f.NewGauge(Options{
				Name: "go_memstats_heap_objects",
				Help: "Number of allocated heap objects.",
			}),
			LastGC: f.NewGauge(Options{
				Name: "go_memstats_last_gc_time_seconds",
				Help: "Time the last garbage collection finished, as nanoseconds since 1970 (the UNIX epoch).",
			}),
			Lookups: f.NewGauge(Options{
				Name: "go_memstats_lookups",
				Help: "Cumulative count of pointer lookups performed by the runtime.",
			}),
			Mallocs: f.NewGauge(Options{
				Name: "go_memstats_mallocs",
				Help: "Cumulative count of heap objects allocated.",
			}),
			NextGC: f.NewGauge(Options{
				Name: "go_memstats_next_gc_heap_alloc_bytes",
				Help: "Target heap size of the next GC cycle.",
			}),
			NumGC: f.NewGauge(Options{
				Name: "go_memstats_num_gc",
				Help: "Number of completed GC cycles.",
			}),
			Pause: f.NewHistogram(Options{
				Name: "go_memstats_gc_pause_seconds",
				Help: "Circular buffer of recent GC stop-the-world pause times in seconds.",
			}),
			PauseTotal: f.NewGauge(Options{
				Name: "go_memstats_gc_pause_total_seconds",
				Help: "Cumulative seconds in GC stop-the-world pauses since the program started.",
			}),
			StackInuse: f.NewGauge(Options{
				Name: "go_memstats_stack_inuse_bytes",
				Help: "Bytes in stack spans.",
			}),
			TotalAlloc: f.NewGauge(Options{
				Name: "go_memstats_total_alloc_bytes",
				Help: "Cumulative bytes allocated for heap objects.",
			}),
		},
		NumGoroutine: f.NewGauge(Options{
			Name: "go_goroutines",
			Help: "Number of goroutines that currently exist.",
		}),
		NumThread: f.NewGauge(Options{
			Name: "go_threads",
			Help: "Number of OS threads created.",
		}),
	}
}

// CollectRuntimeMetrics collects runtime metrics. It blocks
// until the context is canceled.
// Ported from: https://github.com/rcrowley/go-metrics/blob/master/runtime.go#L73
func (f *factory) CollectRuntimeMetrics(ctx context.Context, dur time.Duration) {
	var (
		metrics             = newRuntimeMetrics(f)
		threadCreateProfile = pprof.Lookup("threadcreate")

		memStats runtime.MemStats
		frees    uint64
		lookups  uint64
		mallocs  uint64
		numGC    uint32
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.Tick(dur):
			runtime.ReadMemStats(&memStats)

			metrics.memory.Frees.Set(float64(memStats.Frees - frees))
			metrics.memory.HeapAlloc.Set(float64(memStats.HeapAlloc))
			metrics.memory.HeapIdle.Set(float64(memStats.HeapIdle))
			metrics.memory.HeapInuse.Set(float64(memStats.HeapInuse))
			metrics.memory.HeapObjects.Set(float64(memStats.HeapObjects))
			metrics.memory.LastGC.Set(float64(memStats.LastGC))
			metrics.memory.Lookups.Set(float64(memStats.Lookups - lookups))
			metrics.memory.Mallocs.Set(float64(memStats.Mallocs - mallocs))
			metrics.memory.NextGC.Set(float64(memStats.NextGC))
			metrics.memory.NumGC.Set(float64(memStats.NumGC - numGC))

			observeGCPauses(metrics.memory.Pause, &memStats, numGC)

			frees = memStats.Frees
			lookups = memStats.Lookups
			mallocs = memStats.Mallocs
			numGC = memStats.NumGC

			metrics.memory.PauseTotal.Set(float64(memStats.PauseTotalNs * _nanosecondsPerSecond))
			metrics.memory.StackInuse.Set(float64(memStats.StackInuse))
			metrics.memory.TotalAlloc.Set(float64(memStats.TotalAlloc))

			metrics.NumGoroutine.Set(float64(runtime.NumGoroutine()))
			metrics.NumThread.Set(float64(threadCreateProfile.Count()))
		}
	}
}

// observeGCPauses observes the garbage collection pause durations
// recorded since the previous collection cycle, unwinding the
// runtime's circular pause buffer.
func observeGCPauses(pause Observer, memStats *runtime.MemStats, prevNumGC uint32) {
	i := prevNumGC % uint32(len(memStats.PauseNs))
	ii := memStats.NumGC % uint32(len(memStats.PauseNs))

	if memStats.NumGC-prevNumGC >= uint32(len(memStats.PauseNs)) {
		for i = range uint32(len(memStats.PauseNs)) {
			pause.Observe(float64(memStats.PauseNs[i] * _nanosecondsPerSecond))
		}

		return
	}

	if i > ii {
		for ; i < uint32(len(memStats.PauseNs)); i++ {
			pause.Observe(float64(memStats.PauseNs[i] * _nanosecondsPerSecond))
		}

		i = 0
	}

	for ; i < ii; i++ {
		pause.Observe(float64(memStats.PauseNs[i] * _nanosecondsPerSecond))
	}
}
