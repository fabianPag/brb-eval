package api

type PeerInfo struct {
	ID        int    `json:"id"`
	Address   string `json:"address"`
	PublicKey []byte `json:"public_key"`
}

type Config struct {
	Peers          map[int]PeerInfo `json:"peers"`
	ShouldInitiate bool             `json:"should_initiate"`
}
