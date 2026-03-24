package metrics

import (
	"fmt"
	"sync"
	"time"
)

type Metrics struct {
	mu                sync.Mutex
	firstMessageTime  time.Time
	deliveryTime      time.Time
	totalMessagesRecv int64
	totalBytesRecv    int64
	totalMessagesSent int64
	totalBytesSent    int64
}

func New() *Metrics {
	return &Metrics{
		firstMessageTime:  time.Time{},
		deliveryTime:      time.Time{},
		totalMessagesRecv: 0,
		totalBytesRecv:    0,
		totalMessagesSent: 0,
		totalBytesSent:    0,
	}
}

func (m *Metrics) RecordInitTime() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.firstMessageTime.IsZero() {
		m.firstMessageTime = time.Now()
	}
}

func (m *Metrics) RecordDelivery() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deliveryTime.IsZero() {
		m.deliveryTime = time.Now()
	}
}

func (m *Metrics) RecordMessageSent(payloadSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalMessagesSent++
	m.totalBytesSent += int64(payloadSize)
}

func (m *Metrics) RecordMessageRecieved(payloadSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalMessagesRecv++
	m.totalBytesRecv += int64(payloadSize)

	if m.firstMessageTime.IsZero() {
		m.firstMessageTime = time.Now()
	}
}

func (m *Metrics) PrintMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Println("First Message Time:", m.firstMessageTime.String())
	fmt.Println("Delivery Time:", m.deliveryTime.String())
	fmt.Println("Time to Deliver:", m.deliveryTime.Sub(m.firstMessageTime).String())
	fmt.Println("Total Messages Received:", m.totalMessagesRecv)
	fmt.Println("Total Bytes Received:", m.totalBytesRecv)
	fmt.Println("Total Messages Sent:", m.totalMessagesSent)
	fmt.Println("Total Bytes Sent:", m.totalBytesSent)
}
