package msg

import (
	"bytes"

	"github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/p2p"
)

const (
	CmdVersion = "version"
	CmdVerAck  = "verack"
	CmdPing    = "ping"
	CmdPong    = "pong"

	CmdInv      = "inventory"
	CmdGetBlock = "getblock"

	CmdReceivedProposal  = "proposal"
	CmdAcceptVote        = "acc_vote"
	CmdRejectVote        = "rej_vote"
	CmdGetBlocks         = "get_blc"
	CmdResponseBlocks    = "res_blc"
	CmdRequestConsensus  = "req_con"
	CmdResponseConsensus = "res_con"
	CmdRequestProposal   = "req_pro"
	CmdIllegalProposals  = "ill_pro"
	CmdIllegalVotes      = "ill_vote"
)

func GetMessageHash(msg p2p.Message) common.Uint256 {
	buf := new(bytes.Buffer)
	msg.Serialize(buf)
	msgHash := common.Sha256D(buf.Bytes())
	return msgHash
}
