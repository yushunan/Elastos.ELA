package blockchain

import (
	. "github.com/elastos/Elastos.ELA/common"
	. "github.com/elastos/Elastos.ELA/core/types"
	. "github.com/elastos/Elastos.ELA/core/types/payload"
	"github.com/elastos/Elastos.ELA/protocol"
)

// IChainStoreDpos provides func for dpos
type IChainStoreDpos interface {
	GetRegisteredProducers() []*PayloadRegisterProducer
	GetActiveRegisteredProducers() []*PayloadRegisterProducer
	GetRegisteredProducersSorted() ([]*PayloadRegisterProducer, error)
	GetProducerVote(publicKey []byte) Fixed64
	GetProducerStatus(publicKey string) ProducerState

	GetIllegalProducers() map[string]struct{}
	GetCancelProducerHeight(publicKey []byte) (uint32, error)
}

// IChainStore provides func with store package.
type IChainStore interface {
	IChainStoreDpos
	protocol.TxnPoolListener

	InitWithGenesisBlock(genesisblock *Block) (uint32, error)
	InitProducerVotes() error

	SaveBlock(b *Block) error
	GetBlock(hash Uint256) (*Block, error)
	GetBlockHash(height uint32) (Uint256, error)
	IsDoubleSpend(tx *Transaction) bool

	SaveConfirm(confirm *DPosProposalVoteSlot) error
	GetConfirm(hash Uint256) (*DPosProposalVoteSlot, error)

	GetHeader(hash Uint256) (*Header, error)

	RollbackBlock(hash Uint256) error

	GetTransaction(txID Uint256) (*Transaction, uint32, error)
	GetTxReference(tx *Transaction) (map[*Input]*Output, error)

	PersistAsset(assetid Uint256, asset Asset) error
	GetAsset(hash Uint256) (*Asset, error)

	PersistSidechainTx(sidechainTxHash Uint256)
	GetSidechainTx(sidechainTxHash Uint256) (byte, error)

	GetCurrentBlockHash() Uint256
	GetHeight() uint32

	GetUnspent(txID Uint256, index uint16) (*Output, error)
	ContainsUnspent(txID Uint256, index uint16) (bool, error)
	GetUnspentFromProgramHash(programHash Uint168, assetid Uint256) ([]*UTXO, error)
	GetUnspentsFromProgramHash(programHash Uint168) (map[Uint256][]*UTXO, error)
	GetAssets() map[Uint256]*Asset

	IsTxHashDuplicate(txhash Uint256) bool
	IsSidechainTxHashDuplicate(sidechainTxHash Uint256) bool
	IsBlockInStore(hash Uint256) bool

	Close()
}
