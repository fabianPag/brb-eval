package metrics

import (
	"fmt"
	"sync/atomic"
	"time"
)

type Metrics struct {
	firstMessageTime atomic.Int64
	deliveryTime     atomic.Int64
	messagesReceived atomic.Int64
	bytesReceived    atomic.Int64
	messagesSent     atomic.Int64
	bytesSent        atomic.Int64
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
	m.messagesSent.Add(1)
	m.bytesSent.Add(int64(payloadSize))
}

func (m *Metrics) RecordMessageReceived(payloadSize int) {
	m.messagesReceived.Add(1)
	m.bytesReceived.Add(int64(payloadSize))

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

	fmt.Println("Total Messages Received:", m.messagesReceived.Load())
	fmt.Println("Total Bytes Received:", m.bytesReceived.Load())
	fmt.Println("Total Messages Sent:", m.messagesSent.Load())
	fmt.Println("Total Bytes Sent:", m.bytesSent.Load())
}

type MetricsResults struct {
	FirstMessageTime time.Time
	DeliveryTime     time.Time
	MessagesReceived int64
	BytesReceived    int64
	MessagesSent     int64
	BytesSent        int64
}

func (m *Metrics) GetMetrics() MetricsResults {
	firstNano := m.firstMessageTime.Load()
	delNano := m.deliveryTime.Load()

	res := MetricsResults{
		MessagesReceived: m.messagesReceived.Load(),
		BytesReceived:    m.bytesReceived.Load(),
		MessagesSent:     m.messagesSent.Load(),
		BytesSent:        m.bytesSent.Load(),
	}

	if firstNano != 0 {
		res.FirstMessageTime = time.Unix(0, firstNano)
	}

	if delNano != 0 {
		res.DeliveryTime = time.Unix(0, delNano)
	}

	return res
}
