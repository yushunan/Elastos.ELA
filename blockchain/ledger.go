package blockchain

import (
	"errors"

	"github.com/elastos/Elastos.ELA/blockchain/interfaces"
	. "github.com/elastos/Elastos.ELA/common"
	. "github.com/elastos/Elastos.ELA/core/types"
	. "github.com/elastos/Elastos.ELA/core/types/payload"
)

var FoundationAddress Uint168

var DefaultLedger *Ledger

// Ledger - the struct for ledger
type Ledger struct {
	Blockchain     *Blockchain
	Store          IChainStore
	Arbitrators    interfaces.Arbitrators
	HeightVersions interfaces.HeightVersions
}

//check weather the transaction contains the doubleSpend.
func (l *Ledger) IsDoubleSpend(Tx *Transaction) bool {
	return DefaultLedger.Store.IsDoubleSpend(Tx)
}

//Get the DefaultLedger.
//Note: the later version will support the mutiLedger.So this func mybe expired later.

//Get the Asset from store.
func (l *Ledger) GetAsset(assetID Uint256) (*Asset, error) {
	asset, err := l.Store.GetAsset(assetID)
	if err != nil {
		return nil, errors.New("[Ledger],GetAsset failed with assetID =" + assetID.String())
	}
	return asset, nil
}

//Get Block With Height.
func (l *Ledger) GetBlockWithHeight(height uint32) (*Block, error) {
	temp, err := l.Store.GetBlockHash(height)
	if err != nil {
		return nil, errors.New("[Ledger],GetBlockWithHeight failed with height=" + string(height))
	}
	bk, err := DefaultLedger.Store.GetBlock(temp)
	if err != nil {
		return nil, errors.New("[Ledger],GetBlockWithHeight failed with hash=" + temp.String())
	}
	return bk, nil
}

//Get block with block hash.
func (l *Ledger) GetBlockWithHash(hash Uint256) (*Block, error) {
	bk, err := l.Store.GetBlock(hash)
	if err != nil {
		return nil, errors.New("[Ledger],GetBlockWithHeight failed with hash=" + hash.String())
	}
	return bk, nil
}

//BlockInLedger checks if the block existed in ledger
func (l *Ledger) BlockInLedger(hash Uint256) bool {
	return l.Store.IsBlockInStore(hash)
}

//Get transaction with hash.
func (l *Ledger) GetTransactionWithHash(hash Uint256) (*Transaction, error) {
	tx, _, err := l.Store.GetTransaction(hash)
	if err != nil {
		return nil, errors.New("[Ledger],GetTransactionWithHash failed with hash=" + hash.String())
	}
	return tx, nil
}

//Get local block chain height.
func (l *Ledger) GetLocalBlockChainHeight() uint32 {
	return l.Blockchain.GetBestHeight()
}

//Get blocks and confirms by given height range, if end equals zero will be treat as current highest block height
func (l *Ledger) GetDposBlocks(start, end uint32) ([]*DposBlock, error) {
	//todo complete me
	return nil, nil
}

//Append blocks and confirms directly
func (l *Ledger) AppendDposBlocks(confirms []*DposBlock) error {
	//todo complete me
	return nil
}
