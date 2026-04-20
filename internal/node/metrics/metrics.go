package metrics

import (
	"fmt"
	"sync/atomic"
	"time"
)

type Metrics struct {
	firstMessageTime  atomic.Int64
	deliveryTime      atomic.Int64
	totalMessagesRecv atomic.Int64
	totalBytesRecv    atomic.Int64
	totalMessagesSent atomic.Int64
	totalBytesSent    atomic.Int64
}

func New() *Metrics {
	return &Metrics{}
}

func (m *Metrics) RecordInitTime() {
	now := time.Now().UnixNano()
	m.firstMessageTime.CompareAndSwap(0, now)
}

func (m *Metrics) RecordDelivery() {
	now := time.Now().UnixNano()
	m.deliveryTime.CompareAndSwap(0, now)
}

func (m *Metrics) RecordMessageSent(payloadSize int) {
	m.totalMessagesSent.Add(1)
	m.totalBytesSent.Add(int64(payloadSize))
}

func (m *Metrics) RecordMessageRecieved(payloadSize int) {
	m.totalMessagesRecv.Add(1)
	m.totalBytesRecv.Add(int64(payloadSize))

	if m.firstMessageTime.Load() == 0 {
		now := time.Now().UnixNano()
		m.firstMessageTime.CompareAndSwap(0, now)
	}
}

func (m *Metrics) PrintMetrics() {
	firstNano := m.firstMessageTime.Load()
	delNano := m.deliveryTime.Load()

	var firstTime, delTime time.Time

	if firstNano != 0 {
		firstTime = time.Unix(0, firstNano)
		fmt.Println("First Message Time:", firstTime.String())
	}

	if delNano != 0 {
		delTime = time.Unix(0, delNano)
		fmt.Println("Delivery Time:", delTime.String())
	}

	if firstNano != 0 && delNano != 0 {
		fmt.Println("Time to Deliver:", delTime.Sub(firstTime).String())
	}

	fmt.Println("Total Messages Received:", m.totalMessagesRecv.Load())
	fmt.Println("Total Bytes Received:", m.totalBytesRecv.Load())
	fmt.Println("Total Messages Sent:", m.totalMessagesSent.Load())
	fmt.Println("Total Bytes Sent:", m.totalBytesSent.Load())
}

type MetricsResults struct {
	FirstMessageTime  time.Time
	DeliveryTime      time.Time
	TotalMessagesRecv int64
	TotalBytesRecv    int64
	TotalMessagesSent int64
	TotalBytesSent    int64
}

func (m *Metrics) GetMetrics() MetricsResults {
	firstNano := m.firstMessageTime.Load()
	delNano := m.deliveryTime.Load()

	res := MetricsResults{
		TotalMessagesRecv: m.totalMessagesRecv.Load(),
		TotalBytesRecv:    m.totalBytesRecv.Load(),
		TotalMessagesSent: m.totalMessagesSent.Load(),
		TotalBytesSent:    m.totalBytesSent.Load(),
	}

	if firstNano != 0 {
		res.FirstMessageTime = time.Unix(0, firstNano)
	}

	if delNano != 0 {
		res.DeliveryTime = time.Unix(0, delNano)
	}

	return res
}
