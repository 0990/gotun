package tun

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sync/atomic"
)

// prometheus 指标
var (
	connBytesUplinkTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "conn_bytes_uplink_total",
			Help: "Total bytes uplink from connections",
		},
		[]string{"uuid", "io"},
	)

	connBytesDownlinkTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "conn_bytes_downlink_total",
			Help: "Total bytes downlink to connections",
		},
		[]string{"uuid", "io"},
	)

	//暂未分类的使用这个
	connBytesCommonTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "conn_bytes_common_total",
			Help: "Total bytes common to connections",
		},
		[]string{"uuid", "io"},
	)
)

func NewUplinkCounter(uuid string, io string) *UplinkCounter {
	return &UplinkCounter{
		uuid: uuid,
		io:   io,
	}
}

type UplinkCounter struct {
	uuid  string
	io    string
	value int64
}

func (p *UplinkCounter) Value() int64 {
	return atomic.LoadInt64(&p.value)
}

// Add adds a value to the current counter value, and returns the previous value.
func (p *UplinkCounter) Add(v int64) int64 {
	connBytesUplinkTotal.WithLabelValues(p.uuid, p.io).Add(float64(v))
	return atomic.AddInt64(&p.value, v)
}

func NewDownlinkCounter(uuid string, io string) *DownlinkCounter {
	return &DownlinkCounter{
		uuid: uuid,
		io:   io,
	}
}

type DownlinkCounter struct {
	uuid  string
	io    string
	value int64
}

func (p *DownlinkCounter) Value() int64 {
	return atomic.LoadInt64(&p.value)
}

// Add adds a value to the current counter value, and returns the previous value.
func (p *DownlinkCounter) Add(v int64) int64 {
	connBytesDownlinkTotal.WithLabelValues(p.uuid, p.io).Add(float64(v))
	return atomic.AddInt64(&p.value, v)
}

func NewCommonCounter(uuid string, io string) *CommonCounter {
	return &CommonCounter{
		uuid: uuid,
		io:   io,
	}
}

type CommonCounter struct {
	uuid  string
	io    string
	value int64
}

func (p *CommonCounter) Value() int64 {
	return atomic.LoadInt64(&p.value)
}

// Add adds a value to the current counter value, and returns the previous value.
func (p *CommonCounter) Add(v int64) int64 {
	connBytesCommonTotal.WithLabelValues(p.uuid, p.io).Add(float64(v))
	return atomic.AddInt64(&p.value, v)
}
