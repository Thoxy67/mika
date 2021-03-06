package metrics

import (
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
)

var promHelp = map[string]string{
	"num_gc":            "number of garbage collections",
	"alloc_heap":        "alloc_heap is bytes of allocated heap objects",
	"alloc_total":       "alloc_total is cumulative bytes allocated for heap objects.",
	"mem_sys":           "Sys is the total bytes of memory obtained from the OS.",
	"mallocs":           "mallocs is the cumulative count of heap objects allocated.",
	"frees":             "frees is the cumulative count of heap objects freed.",
	"heap_sys":          "heap_sys is bytes of heap memory obtained from the OS.",
	"heap_idle":         "heap_idle is bytes in idle (unused) spans.",
	"heap_in_use":       "heap_in_use is bytes in in-use spans.",
	"heap_released":     "heap_released is bytes of physical memory returned to the OS.",
	"heap_objects":      "heap_objects is the number of allocated heap objects.",
	"stack_in_use":      "stack_in_use is bytes in stack spans.",
	"stack_sys":         "stack_sys is bytes of stack memory obtained from the OS.",
	"m_span_in_use":     "m_span_in_use is bytes of allocated mspan structures.",
	"m_span_sys":        "m_span_sys is bytes of memory obtained from the OS for mspan structures.",
	"m_cache_in_use":    "m_cache_in_use is bytes of allocated mcache structures.",
	"m_cache_sys":       "m_cache_sys is bytes of memory obtained from the OS for mcache structures.",
	"buck_hash_sys":     "buck_hash_sys is bytes of memory in profiling bucket hash tables.",
	"gc_sys":            "gc_sys is bytes of memory in garbage collection metadata.",
	"other_sys":         "other_sys is bytes of memory in miscellaneous off-heap runtime allocations.",
	"gc_next":           "gc_next is the target heap size of the next GC cycle.",
	"gc_last":           "gc_last is the time the last garbage collection finished, as nanoseconds since 1970 (the UNIX epoch).",
	"gc_pause_total_ns": "gc_pause_total_ns is the cumulative nanoseconds in GC stop-the-world pauses since the program started.",
	"gc_pause_ns":       "gc_pause_ns is a circular buffer of recent GC stop-the-world pause times in nanoseconds.",
	"gc_pause_end": "gc_pause_end is a circular buffer of recent GC pause end times, " +
		"as nanoseconds since 1970 (the UNIX epoch).",
	"gc_num": "gc_num is the number of completed GC cycles.",
	"gc_num_forced": "gc_num_forced is the number of GC cycles that were " +
		"forced by the application calling the GC function.",
	"gc_cpu_fraction": "gc_cpu_fraction is the fraction of this program's available " +
		"CPU time used by the GC since the program started.",
	"t_cache_torrents":              "t_cache_torrents is the total count of cached torrents",
	"t_cache_users":                 "t_cache_users is the total count of cached users",
	"t_cache_peers":                 "t_cache_peers is the total count of cached peers",
	"t_ann_total":                   "t_ann_total is the total count of announces",
	"t_ann_status_ok":               "t_ann_status_ok is the total count of successful announces",
	"t_ann_status_unauthorized":     "t_ann_status_unauthorized is the total count of unauthorized users requests",
	"t_ann_status_invalid_infohash": "t_ann_status_invalid_infohash is the total count of invalid info hash requests",
	"t_ann_status_malformed":        "t_ann_status_malformed is the total count of malformed queries",
	"t_ann_time_ns":                 "t_ann_time_ns is the average time it takes to fulfill a successful announce in nanoseconds",
}

var (
	TorrentsTotalCached int64
	PeersTotalCached    int64
	UsersTotalCached    int64

	AnnounceTotal                 int64
	AnnounceStatusOK              int64
	AnnounceStatusUnauthorized    int64
	AnnounceStatusInvalidInfoHash int64
	AnnounceStatusMalformed       int64
	execLock                      *sync.Mutex
	AnnounceExecTimesNs           []int64
)

func AddAnnounceTime(t int64) {
	execLock.Lock()
	AnnounceExecTimesNs = append(AnnounceExecTimesNs, t)
	execLock.Unlock()
}

func avgExecTime() int64 {
	execLock.Lock()
	var t int64
	var avg int64
	for _, v := range AnnounceExecTimesNs {
		t += v
	}
	s := int64(len(AnnounceExecTimesNs))
	if s > 0 {
		avg = t / s
		AnnounceExecTimesNs = nil
	}
	execLock.Unlock()
	return avg
}

type RuntimeMetrics struct {
	TorrentsTotalCached           int64 `prom:"t_cache_torrents" prom_type:"counter"`
	UsersTotalCached              int64 `prom:"t_cache_users" prom_type:"counter"`
	PeersTotalCached              int64 `prom:"t_cache_peers" prom_type:"counter"`
	AnnounceTotal                 int64 `prom:"t_ann_total" prom_type:"gauge"`
	AnnounceStatusOK              int64 `prom:"t_ann_status_ok" prom_type:"gauge"`
	AnnounceStatusUnauthorized    int64 `prom:"t_ann_status_unauthorized" prom_type:"gauge"`
	AnnounceStatusInvalidInfoHash int64 `prom:"t_ann_status_invalid_infohash" prom_type:"gauge"`
	AnnounceStatusMalformed       int64 `prom:"t_ann_status_malformed" prom_type:"gauge"`
	AnnounceExecTimesNsAvg        int64 `prom:"t_ann_time_ns" prom_type:"gauge"`

	// GC stats
	NumGC      int64 `prom:"num_gc" prom_type:"gauge"`
	PauseTotal int64 `prom:"pause_total" prom_type:"gauge"`

	// Goro stats
	GoRoutines int `prom:"go_routines" prom_type:"gauge"`

	// Mem stats
	AllocHeap      uint64  `prom:"alloc_heap" prom_type:"gauge"`
	AllocTotal     uint64  `prom:"alloc_total" prom_type:"counter"`
	MemSys         uint64  `prom:"mem_sys" prom_type:"gauge"`
	Mallocs        uint64  `prom:"mallocs" prom_type:"gauge"`
	Frees          uint64  `prom:"frees" prom_type:"counter"`
	HeapSys        uint64  `prom:"heap_sys" prom_type:"gauge"`
	HeapIdle       uint64  `prom:"heap_idle" prom_type:"gauge"`
	HeapInUse      uint64  `prom:"heap_in_use" prom_type:"gauge"`
	HeapReleased   uint64  `prom:"heap_released" prom_type:"gauge"`
	HeapObjects    uint64  `prom:"heap_objects" prom_type:"gauge"`
	StackInUse     uint64  `prom:"stack_in_use" prom_type:"gauge"`
	StackSys       uint64  `prom:"stack_sys" prom_type:"gauge"`
	MSpanInUse     uint64  `prom:"m_span_in_use" prom_type:"gauge"`
	MSpanSys       uint64  `prom:"m_span_sys" prom_type:"gauge"`
	MCacheInUse    uint64  `prom:"m_cache_in_use" prom_type:"gauge"`
	MCacheSys      uint64  `prom:"m_cache_sys" prom_type:"gauge"`
	BuckHashSys    uint64  `prom:"buck_hash_sys" prom_type:"gauge"`
	GCSys          uint64  `prom:"gc_sys" prom_type:"gauge"`
	OtherSys       uint64  `prom:"other_sys" prom_type:"gauge"`
	GCNext         uint64  `prom:"gc_next" prom_type:"gauge"`
	GCLast         uint64  `prom:"gc_last" prom_type:"gauge"`
	GCPauseTotalNS uint64  `prom:"gc_pause_total_ns" prom_type:"gauge"`
	GCPauseNS      uint64  `prom:"gc_pause_ns" prom_type:""`
	GCPauseEnd     uint64  `prom:"gc_pause_end" prom_type:""`
	GCNum          uint32  `prom:"gc_num" prom_type:""`
	GCNumForced    uint32  `prom:"gc_num_forced" prom_type:""`
	GCCPUFraction  float64 `prom:"gc_cpu_fraction" prom_type:"gauge"`
}

func (m RuntimeMetrics) String() string {
	var out strings.Builder
	v := reflect.ValueOf(m)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		tagKey := field.Tag.Get("prom")
		out.WriteString(fmt.Sprintf("# HELP %s %s\n", tagKey, promHelp[tagKey]))
		out.WriteString(fmt.Sprintf("# TYPE %s %s\n", tagKey, field.Tag.Get("prom_type")))
		out.WriteString(fmt.Sprintf("%s %v\n", tagKey, v.Field(i).Interface()))
	}
	return out.String()
}

func Get() RuntimeMetrics {
	var (
		mem runtime.MemStats
		gc  debug.GCStats
	)
	runtime.ReadMemStats(&mem)
	debug.ReadGCStats(&gc)
	var m RuntimeMetrics

	m.TorrentsTotalCached = atomic.LoadInt64(&TorrentsTotalCached)
	m.UsersTotalCached = atomic.LoadInt64(&UsersTotalCached)
	m.PeersTotalCached = atomic.LoadInt64(&PeersTotalCached)
	m.AnnounceTotal = atomic.SwapInt64(&AnnounceTotal, 0)
	m.AnnounceStatusOK = atomic.SwapInt64(&AnnounceStatusOK, 0)
	m.AnnounceStatusUnauthorized = atomic.SwapInt64(&AnnounceStatusUnauthorized, 0)
	m.AnnounceStatusInvalidInfoHash = atomic.SwapInt64(&AnnounceStatusInvalidInfoHash, 0)
	m.AnnounceStatusMalformed = atomic.SwapInt64(&AnnounceStatusMalformed, 0)
	m.AnnounceExecTimesNsAvg = avgExecTime()
	m.NumGC = gc.NumGC
	m.PauseTotal = gc.PauseTotal.Milliseconds()

	m.AllocHeap = mem.HeapAlloc
	m.AllocTotal = mem.TotalAlloc
	m.MemSys = mem.Sys
	m.Mallocs = mem.Mallocs
	m.Frees = mem.Frees
	m.HeapSys = mem.HeapSys
	m.HeapIdle = mem.HeapIdle
	m.HeapInUse = mem.HeapInuse
	m.HeapReleased = mem.HeapReleased
	m.HeapObjects = mem.HeapObjects
	m.StackInUse = mem.StackInuse
	m.StackSys = mem.StackSys
	m.MSpanInUse = mem.MSpanInuse
	m.MSpanSys = mem.MSpanSys
	m.MCacheSys = mem.MCacheSys
	m.BuckHashSys = mem.BuckHashSys
	m.GCSys = mem.GCSys
	m.OtherSys = mem.OtherSys
	m.GCNext = mem.NextGC
	m.GCLast = mem.LastGC
	m.GCPauseTotalNS = mem.PauseTotalNs
	m.GCPauseNS = mem.PauseNs[(mem.NumGC+255)%256]
	m.GCPauseEnd = mem.PauseEnd[(mem.NumGC+255)%256]
	m.GCNum = mem.NumGC
	m.GCNumForced = mem.NumForcedGC
	m.GCCPUFraction = mem.GCCPUFraction

	m.GoRoutines = runtime.NumGoroutine()

	return m
}

func init() {
	execLock = &sync.Mutex{}
}
