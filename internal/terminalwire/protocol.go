// Package terminalwire defines the versioned, private edge/core protocol.
package terminalwire

const Version = 1

type Request struct {
	Version   int    `json:"version"`
	EdgeID    string `json:"edge_id"`
	ID        string `json:"id"`
	Sequence  int64  `json:"sequence"`
	Ack       uint64 `json:"ack"`
	Kind      string `json:"kind"` // open, input, poll, close
	Input     string `json:"input,omitempty"`
	Terminal  string `json:"terminal,omitempty"`
	Dedicated bool   `json:"dedicated,omitempty"`
}

type Output struct {
	ID   uint64 `json:"id"`
	Data []byte `json:"data"`
}

type Response struct {
	Version      int      `json:"version"`
	CoreID       string   `json:"core_id"`
	Sequence     int64    `json:"sequence"`
	Output       []Output `json:"output,omitempty"`
	Presentation int      `json:"presentation"`
	Closed       bool     `json:"closed"`
	Prompt       []byte   `json:"prompt,omitempty"`
	Reset        bool     `json:"reset,omitempty"`
}
