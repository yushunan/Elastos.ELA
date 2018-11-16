package msg

import (
	"io"
)

type ResponseConsensusMessage struct {
	Consensus ConsensusStatus
}

func (msg *ResponseConsensusMessage) CMD() string {
	return ResponseConsensus
}

func (msg *ResponseConsensusMessage) MaxLength() uint32 {
	//todo add max length
	return 0
}

func (msg *ResponseConsensusMessage) Serialize(w io.Writer) error {
	return msg.Consensus.Serialize(w)
}

func (msg *ResponseConsensusMessage) Deserialize(r io.Reader) error {
	return msg.Consensus.Deserialize(r)
}