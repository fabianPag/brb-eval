package api

import (
	"time"
)

type PeerInfo struct {
	ID        int    `json:"id"`
	Address   string `json:"address"`
	PublicKey []byte `json:"public_key"`
}

type PublicCryptoParams struct {
	NumDataShards     int            `json:"num_data_shards"`   // k
	NumParityShards   int            `json:"num_parity_shards"` // n - k
	PKBytes           []byte         `json:"pk_bytes"`
	VKBytes           []byte         `json:"vk_bytes"`
	MasterPubKeyBytes []byte         `json:"master_pub_key_bytes"` // Serialized bn254.G2Affine
	NodePubKeysBytes  map[int][]byte `json:"node_pub_keys_bytes"`  // Map of node ID to serialized bn254.G2Affine
}

type PrivateCryptoParams struct {
	NodeIndex         int    `json:"node_index"`
	PrivateShareBytes []byte `json:"private_share_bytes"` // Serialized fr.Element
}

type Config struct {
	Peers               map[int]PeerInfo    `json:"peers"`
	Role                string              `json:"role"` // "honest", "twins_sender", "silent"
	DesignatedSender    int                 `json:"designated_sender"`
	T                   int                 `json:"t"`
	D                   int                 `json:"d"`
	NumMsgDrops         int                 `json:"num_msg_drops"`
	HonestNodes         []int               `json:"honest_nodes"`
	PublicCryptoParams  PublicCryptoParams  `json:"public_crypto_params"`
	PrivateCryptoParams PrivateCryptoParams `json:"private_crypto_params"`
	Algorithm           string              `json:"algorithm"`
	PayloadSize         int                 `json:"payload_size"`
}

type MetricsData struct {
	NodeID           int       `json:"node_id"`
	Role             string    `json:"role"`
	Delivered        bool      `json:"delivered"`
	DeliveredHash    string    `json:"delivered_hash"`
	SentHash         string    `json:"sent_hash"`
	FirstMessageTime time.Time `json:"first_message_time"`
	DeliveryTime     time.Time `json:"delivery_time"`
	MessagesReceived int64     `json:"messages_received"`
	BytesReceived    int64     `json:"bytes_received"`
	MessagesSent     int64     `json:"messages_sent"`
	BytesSent        int64     `json:"bytes_sent"`
}
