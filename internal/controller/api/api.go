package api

import "time"

type PeerInfo struct {
	ID        int    `json:"id"`
	Address   string `json:"address"`
	PublicKey []byte `json:"public_key"`
}

type Config struct {
	Peers          map[int]PeerInfo `json:"peers"`
	ShouldInitiate bool             `json:"should_initiate"`
}

type MetricsData struct {
	NodeID            int       `json:"node_id"`
	FirstMessageTime  time.Time `json:"first_message_time"`
	DeliveryTime      time.Time `json:"delivery_time"`
	TotalMessagesRecv int64     `json:"total_messages_recv"`
	TotalBytesRecv    int64     `json:"total_bytes_recv"`
	TotalMessagesSent int64     `json:"total_messages_sent"`
	TotalBytesSent    int64     `json:"total_bytes_sent"`
}
