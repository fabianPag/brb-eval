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
	ShouldInitiate      bool                `json:"should_initiate"`
	DesignatedSender    int                 `json:"designated_sender"`
	T                   int                 `json:"t"`
	D                   int                 `json:"d"`
	PublicCryptoParams  PublicCryptoParams  `json:"public_crypto_params"`
	PrivateCryptoParams PrivateCryptoParams `json:"private_crypto_params"`
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
