package mempool

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/elastos/Elastos.ELA/blockchain"
	. "github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/common/config"
	"github.com/elastos/Elastos.ELA/common/log"
	. "github.com/elastos/Elastos.ELA/core/types"
	"github.com/elastos/Elastos.ELA/core/types/outputpayload"
	. "github.com/elastos/Elastos.ELA/core/types/payload"
	. "github.com/elastos/Elastos.ELA/errors"
	"github.com/elastos/Elastos.ELA/events"
	"github.com/elastos/Elastos.ELA/protocol"
)

type TxPool struct {
	sync.RWMutex
	txnCnt  uint64                   // count
	txnList map[Uint256]*Transaction // transaction which have been verifyed will put into this map
	//issueSummary  map[Uint256]Fixed64           // transaction which pass the verify will summary the amout to this map
	inputUTXOList     map[string]*Transaction // transaction which pass the verify will add the UTXO to this map
	producerList      map[string]struct{}
	nodePublicKeyList map[string]struct{}
	sidechainTxList   map[Uint256]*Transaction // sidechain tx pool
	Listeners         map[protocol.TxnPoolListener]interface{}
}

func (pool *TxPool) Init() {
	pool.Lock()
	defer pool.Unlock()
	pool.txnCnt = 0
	pool.inputUTXOList = make(map[string]*Transaction)
	//pool.issueSummary = make(map[Uint256]Fixed64)
	pool.txnList = make(map[Uint256]*Transaction)
	pool.producerList = make(map[string]struct{})
	pool.nodePublicKeyList = make(map[string]struct{})
	pool.sidechainTxList = make(map[Uint256]*Transaction)
	pool.Listeners = make(map[protocol.TxnPoolListener]interface{})
}

//append transaction to txnpool when check ok.
//1.check  2.check with ledger(db) 3.check with pool
func (pool *TxPool) AppendToTxnPool(txn *Transaction) ErrCode {

	if txn.IsCoinBaseTx() {
		log.Warn("coinbase cannot be added into transaction pool", txn.Hash().String())
		return ErrIneffectiveCoinbase
	}

	//verify transaction with Concurrency
	if errCode := blockchain.CheckTransactionSanity(blockchain.DefaultLedger.Blockchain.BlockHeight+1, txn); errCode != Success {
		log.Warn("[TxPool CheckTransactionSanity] failed", txn.Hash().String())
		return errCode
	}
	if errCode := blockchain.CheckTransactionContext(blockchain.DefaultLedger.Blockchain.BlockHeight+1, txn); errCode != Success {
		log.Warn("[TxPool CheckTransactionContext] failed", txn.Hash().String())
		return errCode
	}
	//verify transaction by pool with lock
	if errCode := pool.verifyTransactionWithTxnPool(txn); errCode != Success {
		log.Warn("[TxPool verifyTransactionWithTxnPool] failed", txn.Hash())
		return errCode
	}

	txn.Fee = blockchain.GetTxFee(txn, blockchain.DefaultLedger.Blockchain.AssetID)
	buf := new(bytes.Buffer)
	txn.Serialize(buf)
	txn.FeePerKB = txn.Fee * 1000 / Fixed64(len(buf.Bytes()))
	//add the transaction to process scope
	if ok := pool.addToTxList(txn); !ok {
		// reject duplicated transaction
		log.Debugf("Transaction duplicate %s", txn.Hash().String())
		return ErrTransactionDuplicate
	}

	if pool.Listeners != nil && txn.IsIllegalBlockTx() {
		for k := range pool.Listeners {
			k.OnIllegalBlockTxnReceived(txn)
		}
	}

	return Success
}

//get the transaction in txnpool
func (pool *TxPool) GetTransactionPool(hasMaxCount bool) map[Uint256]*Transaction {
	pool.RLock()
	count := config.Parameters.MaxTxsInBlock
	if count <= 0 {
		hasMaxCount = false
	}
	if len(pool.txnList) < count || !hasMaxCount {
		count = len(pool.txnList)
	}
	var num int
	txnMap := make(map[Uint256]*Transaction)
	for txnID, tx := range pool.txnList {
		txnMap[txnID] = tx
		num++
		if num >= count {
			break
		}
	}
	pool.RUnlock()
	return txnMap
}

//clean the trasaction Pool with committed block.
func (pool *TxPool) CleanSubmittedTransactions(block *Block) error {
	pool.cleanTransactions(block.Transactions)
	pool.cleanSidechainTx(block.Transactions)
	pool.cleanSideChainPowTx()
	pool.cleanCanceledProducer(block.Transactions)

	return nil
}

func (pool *TxPool) cleanTransactions(blockTxs []*Transaction) error {
	txCountInPool := pool.GetTransactionCount()
	deleteCount := 0
	for _, blockTx := range blockTxs {
		if blockTx.TxType == CoinBase {
			continue
		}
		inputUtxos, err := blockchain.DefaultLedger.Store.GetTxReference(blockTx)
		if err != nil {
			log.Info(fmt.Sprintf("Transaction =%x not Exist in Pool when delete.", blockTx.Hash()), err)
			continue
		}
		for input := range inputUtxos {
			// we search transactions in transaction pool which have the same utxos with those transactions
			// in block. That is, if a transaction in the new-coming block uses the same utxo which a transaction
			// in transaction pool uses, then the latter one should be deleted, because one of its utxos has been used
			// by a confirmed transaction packed in the new-coming block.
			if tx := pool.getInputUTXOList(input); tx != nil {
				if tx.Hash() == blockTx.Hash() {
					// it is evidently that two transactions with the same transaction id has exactly the same utxos with each
					// other. This is a special case of what we've said above.
					log.Debugf("duplicated transactions detected when adding a new block. "+
						" Delete transaction in the transaction pool. Transaction id: %x", tx.Hash())
				} else {
					log.Debugf("double spent UTXO inputs detected in transaction pool when adding a new block. "+
						"Delete transaction in the transaction pool. "+
						"block transaction hash: %x, transaction hash: %x, the same input: %s, index: %d",
						blockTx.Hash(), tx.Hash(), input.Previous.TxID, input.Previous.Index)
				}
				//1.remove from txnList
				pool.delFromTxList(tx.Hash())
				//2.remove from UTXO list map
				for _, input := range tx.Inputs {
					pool.delInputUTXOList(input)
				}

				//delete sidechain tx list
				if tx.TxType == WithdrawFromSideChain {
					payload, ok := tx.Payload.(*PayloadWithdrawFromSideChain)
					if !ok {
						log.Error("type cast failed when clean sidechain tx:", tx.Hash())
					}
					for _, hash := range payload.SideChainTransactionHashes {
						pool.delSidechainTx(hash)
					}
				}

				// delete producer
				if tx.TxType == RegisterProducer {
					rpPayload, ok := tx.Payload.(*PayloadRegisterProducer)
					if !ok {
						log.Error("register producer payload cast failed, tx:", tx.Hash())
					}
					pool.delProducer(BytesToHexString(rpPayload.OwnerPublicKey))
					pool.delProducerNode(BytesToHexString(rpPayload.NodePublicKey))
				}
				if tx.TxType == UpdateProducer {
					upPayload, ok := tx.Payload.(*PayloadUpdateProducer)
					if !ok {
						log.Error("update producer payload cast failed, tx:", tx.Hash())
					}
					pool.delProducer(BytesToHexString(upPayload.OwnerPublicKey))
					pool.delProducerNode(BytesToHexString(upPayload.NodePublicKey))
				}
				if tx.TxType == CancelProducer {
					cpPayload, ok := tx.Payload.(*PayloadCancelProducer)
					if !ok {
						log.Error("cancel producer payload cast failed, tx:", tx.Hash())
					}
					pool.delProducer(BytesToHexString(cpPayload.OwnerPublicKey))
				}

				deleteCount++
			}
		}
	}
	log.Debug(fmt.Sprintf("[cleanTransactionList],transaction %d in block, %d in transaction pool before, %d deleted,"+
		" Remains %d in TxPool",
		len(blockTxs), txCountInPool, deleteCount, pool.GetTransactionCount()))
	return nil
}

func (pool *TxPool) cleanCanceledProducer(txs []*Transaction) error {
	for _, txn := range txs {
		if txn.TxType == CancelProducer {
			cpPayload, ok := txn.Payload.(*PayloadCancelProducer)
			if !ok {
				return errors.New("invalid cancel producer payload")
			}
			if err := pool.cleanVoteAndUpdateProducer(cpPayload.OwnerPublicKey); err != nil {
				log.Error(err)
			}
		}
	}

	return nil
}

func (pool *TxPool) cleanVoteAndUpdateProducer(ownerPublicKey []byte) error {
	for _, txn := range pool.txnList {
		if txn.TxType == TransferAsset {
			for _, output := range txn.Outputs {
				if output.OutputType == VoteOutput {
					opPayload, ok := output.OutputPayload.(*outputpayload.VoteOutput)
					if !ok {
						return errors.New("invalid vote output payload")
					}
					for _, content := range opPayload.Contents {
						if content.VoteType == outputpayload.Delegate {
							for _, pubKey := range content.Candidates {
								if bytes.Equal(ownerPublicKey, pubKey) {
									pool.removeTransaction(txn)
								}
							}
						}
					}
				}
			}
		} else if txn.TxType == UpdateProducer {
			upPayload, ok := txn.Payload.(*PayloadUpdateProducer)
			if !ok {
				return errors.New("invalid update producer payload")
			}
			if bytes.Equal(upPayload.OwnerPublicKey, ownerPublicKey) {
				pool.removeTransaction(txn)
				pool.delProducer(BytesToHexString(upPayload.OwnerPublicKey))
				pool.delProducerNode(BytesToHexString(upPayload.NodePublicKey))
			}
		}
	}

	return nil
}

//get the transaction by hash
func (pool *TxPool) GetTransaction(hash Uint256) *Transaction {
	pool.RLock()
	defer pool.RUnlock()
	return pool.txnList[hash]
}

//verify transaction with txnpool
func (pool *TxPool) verifyTransactionWithTxnPool(txn *Transaction) ErrCode {
	if txn.IsSideChainPowTx() {
		// check and replace the duplicate sidechainpow tx
		pool.replaceDuplicateSideChainPowTx(txn)
	} else if txn.IsWithdrawFromSideChainTx() {
		// check if the withdraw transaction includes duplicate sidechain tx in pool
		if err := pool.verifyDuplicateSidechainTx(txn); err != nil {
			log.Warn(err)
			return ErrSidechainTxDuplicate
		}
	} else if txn.IsRegisterProducerTx() {
		payload, ok := txn.Payload.(*PayloadRegisterProducer)
		if !ok {
			log.Error("register producer payload cast failed, tx:", txn.Hash())
		}
		if err := pool.verifyDuplicateProducer(BytesToHexString(payload.OwnerPublicKey)); err != nil {
			log.Warn(err)
			return ErrProducerProcessing
		}
		if err := pool.verifyDuplicateProducerNode(BytesToHexString(payload.NodePublicKey)); err != nil {
			log.Warn(err)
			return ErrProducerNodeProcessing
		}
	} else if txn.IsUpdateProducerTx() {
		payload, ok := txn.Payload.(*PayloadUpdateProducer)
		if !ok {
			log.Error("update producer payload cast failed, tx:", txn.Hash())
		}
		if err := pool.verifyDuplicateProducer(BytesToHexString(payload.OwnerPublicKey)); err != nil {
			log.Warn(err)
			return ErrProducerProcessing
		}
		if err := pool.verifyDuplicateProducerNode(BytesToHexString(payload.NodePublicKey)); err != nil {
			log.Warn(err)
			return ErrProducerNodeProcessing
		}
	} else if txn.IsCancelProducerTx() {
		payload, ok := txn.Payload.(*PayloadCancelProducer)
		if !ok {
			log.Error("cancel producer payload cast failed, tx:", txn.Hash())
		}
		if err := pool.verifyDuplicateProducer(BytesToHexString(payload.OwnerPublicKey)); err != nil {
			log.Warn(err)
			return ErrProducerProcessing
		}
	}

	// check if the transaction includes double spent UTXO inputs
	if err := pool.verifyDoubleSpend(txn); err != nil {
		log.Warn(err)
		return ErrDoubleSpend
	}

	return Success
}

//remove from associated map
func (pool *TxPool) removeTransaction(txn *Transaction) {
	//1.remove from txnList
	pool.delFromTxList(txn.Hash())
	//2.remove from UTXO list map
	result, err := blockchain.DefaultLedger.Store.GetTxReference(txn)
	if err != nil {
		log.Info(fmt.Sprintf("Transaction =%x not Exist in Pool when delete.", txn.Hash()))
		return
	}
	for UTXOTxInput := range result {
		pool.delInputUTXOList(UTXOTxInput)
	}
}

//check and add to utxo list pool
func (pool *TxPool) verifyDoubleSpend(txn *Transaction) error {
	reference, err := blockchain.DefaultLedger.Store.GetTxReference(txn)
	if err != nil {
		return err
	}
	inputs := []*Input{}
	for k := range reference {
		if txn := pool.getInputUTXOList(k); txn != nil {
			return errors.New(fmt.Sprintf("double spent UTXO inputs detected, "+
				"transaction hash: %x, input: %s, index: %d",
				txn.Hash(), k.Previous.TxID, k.Previous.Index))
		}
		inputs = append(inputs, k)
	}
	for _, v := range inputs {
		pool.addInputUTXOList(txn, v)
	}

	return nil
}

func (pool *TxPool) IsDuplicateSidechainTx(sidechainTxHash Uint256) bool {
	_, ok := pool.sidechainTxList[sidechainTxHash]
	if ok {
		return true
	}

	return false
}

//check and add to sidechain tx pool
func (pool *TxPool) verifyDuplicateSidechainTx(txn *Transaction) error {
	withPayload, ok := txn.Payload.(*PayloadWithdrawFromSideChain)
	if !ok {
		return errors.New("convert the payload of withdraw tx failed")
	}

	for _, hash := range withPayload.SideChainTransactionHashes {
		_, ok := pool.sidechainTxList[hash]
		if ok {
			return errors.New("duplicate sidechain tx detected")
		}
	}
	pool.addSidechainTx(txn)

	return nil
}

func (pool *TxPool) verifyDuplicateProducer(ownerPublicKey string) error {
	_, ok := pool.producerList[ownerPublicKey]
	if ok {
		return errors.New("this producer in being processed")
	}
	pool.addProducer(ownerPublicKey)

	return nil
}

func (pool *TxPool) addProducer(publicKey string) {
	pool.Lock()
	defer pool.Unlock()
	pool.producerList[publicKey] = struct{}{}
}

func (pool *TxPool) delProducer(publicKey string) bool {
	pool.Lock()
	defer pool.Unlock()
	_, ok := pool.producerList[publicKey]
	if !ok {
		return false
	}
	delete(pool.producerList, publicKey)
	return true
}

func (pool *TxPool) verifyDuplicateProducerNode(nodePublicKey string) error {
	_, ok := pool.nodePublicKeyList[nodePublicKey]
	if ok {
		return errors.New("this producer node in being processed")
	}
	pool.addProducerNode(nodePublicKey)

	return nil
}

func (pool *TxPool) addProducerNode(nodePublicKey string) {
	pool.Lock()
	defer pool.Unlock()
	pool.nodePublicKeyList[nodePublicKey] = struct{}{}
}

func (pool *TxPool) delProducerNode(nodePublicKey string) bool {
	pool.Lock()
	defer pool.Unlock()
	_, ok := pool.nodePublicKeyList[nodePublicKey]
	if !ok {
		return false
	}
	delete(pool.nodePublicKeyList, nodePublicKey)
	return true
}

// check and replace the duplicate sidechainpow tx
func (pool *TxPool) replaceDuplicateSideChainPowTx(txn *Transaction) {
	var replaceList []*Transaction

	pool.RLock()
	for _, v := range pool.txnList {
		if v.TxType == SideChainPow {
			oldPayload := v.Payload.Data(SideChainPowPayloadVersion)
			oldGenesisHashData := oldPayload[32:64]

			newPayload := txn.Payload.Data(SideChainPowPayloadVersion)
			newGenesisHashData := newPayload[32:64]

			if bytes.Equal(oldGenesisHashData, newGenesisHashData) {
				replaceList = append(replaceList, v)
			}
		}
	}
	pool.RUnlock()

	for _, txn := range replaceList {
		txid := txn.Hash()
		log.Info("replace sidechainpow transaction, txid=", txid.String())
		pool.removeTransaction(txn)
	}
}

// clean the sidechain tx pool
func (pool *TxPool) cleanSidechainTx(txs []*Transaction) {
	for _, txn := range txs {
		if txn.IsWithdrawFromSideChainTx() {
			withPayload := txn.Payload.(*PayloadWithdrawFromSideChain)
			for _, hash := range withPayload.SideChainTransactionHashes {
				poolTx := pool.sidechainTxList[hash]
				if poolTx != nil {
					// delete tx
					pool.delFromTxList(poolTx.Hash())
					//delete utxo map
					for _, input := range poolTx.Inputs {
						pool.delInputUTXOList(input)
					}
					//delete sidechain tx map
					payload, ok := poolTx.Payload.(*PayloadWithdrawFromSideChain)
					if !ok {
						log.Error("type cast failed when clean sidechain tx:", poolTx.Hash())
					}
					for _, hash := range payload.SideChainTransactionHashes {
						pool.delSidechainTx(hash)
					}
				}
			}
		}
	}
}

// clean the sidechainpow tx pool
func (pool *TxPool) cleanSideChainPowTx() {
	arbitrator := blockchain.DefaultLedger.Arbitrators.GetOnDutyArbitrator()

	pool.Lock()
	defer pool.Unlock()
	for hash, txn := range pool.txnList {
		if txn.IsSideChainPowTx() {
			if err := blockchain.CheckSideChainPowConsensus(txn, arbitrator); err != nil {
				// delete tx
				delete(pool.txnList, hash)
				//delete utxo map
				for _, input := range txn.Inputs {
					delete(pool.inputUTXOList, input.ReferKey())
				}
			}
		}
	}
}

func (pool *TxPool) addToTxList(txn *Transaction) bool {
	pool.Lock()
	defer pool.Unlock()
	txnHash := txn.Hash()
	if _, ok := pool.txnList[txnHash]; ok {
		return false
	}
	pool.txnList[txnHash] = txn
	blockchain.DefaultLedger.Blockchain.BCEvents.Notify(events.EventNewTransactionPutInPool, txn)
	return true
}

func (pool *TxPool) delFromTxList(txID Uint256) bool {
	pool.Lock()
	defer pool.Unlock()
	if _, ok := pool.txnList[txID]; !ok {
		return false
	}
	delete(pool.txnList, txID)
	return true
}

func (pool *TxPool) copyTxList() map[Uint256]*Transaction {
	pool.RLock()
	defer pool.RUnlock()
	txnMap := make(map[Uint256]*Transaction, len(pool.txnList))
	for txnID, txn := range pool.txnList {
		txnMap[txnID] = txn
	}
	return txnMap
}

func (pool *TxPool) GetTransactionCount() int {
	pool.RLock()
	defer pool.RUnlock()
	return len(pool.txnList)
}

func (pool *TxPool) getInputUTXOList(input *Input) *Transaction {
	pool.RLock()
	defer pool.RUnlock()
	return pool.inputUTXOList[input.ReferKey()]
}

func (pool *TxPool) addInputUTXOList(tx *Transaction, input *Input) bool {
	pool.Lock()
	defer pool.Unlock()
	id := input.ReferKey()
	_, ok := pool.inputUTXOList[id]
	if ok {
		return false
	}
	pool.inputUTXOList[id] = tx

	return true
}

func (pool *TxPool) delInputUTXOList(input *Input) bool {
	pool.Lock()
	defer pool.Unlock()
	id := input.ReferKey()
	_, ok := pool.inputUTXOList[id]
	if !ok {
		return false
	}
	delete(pool.inputUTXOList, id)
	return true
}

func (pool *TxPool) addSidechainTx(txn *Transaction) {
	pool.Lock()
	defer pool.Unlock()
	witPayload := txn.Payload.(*PayloadWithdrawFromSideChain)
	for _, hash := range witPayload.SideChainTransactionHashes {
		pool.sidechainTxList[hash] = txn
	}
}

func (pool *TxPool) delSidechainTx(hash Uint256) bool {
	pool.Lock()
	defer pool.Unlock()
	_, ok := pool.sidechainTxList[hash]
	if !ok {
		return false
	}
	delete(pool.sidechainTxList, hash)
	return true
}

func (pool *TxPool) MaybeAcceptTransaction(txn *Transaction) error {
	txHash := txn.Hash()

	// Don't accept the transaction if it already exists in the pool.  This
	// applies to orphan transactions as well.  This check is intended to
	// be a quick check to weed out duplicates.
	if txn := pool.GetTransaction(txHash); txn != nil {
		return fmt.Errorf("already have transaction")
	}

	// A standalone transaction must not be a coinbase
	if txn.IsCoinBaseTx() {
		return fmt.Errorf("transaction is an individual coinbase")
	}

	if errCode := pool.AppendToTxnPool(txn); errCode != Success {
		return fmt.Errorf("VerifyTxs failed when AppendToTxnPool")
	}

	return nil
}

func (pool *TxPool) RemoveTransaction(txn *Transaction) {
	txHash := txn.Hash()
	for i := range txn.Outputs {
		input := Input{
			Previous: OutPoint{
				TxID:  txHash,
				Index: uint16(i),
			},
		}

		txn := pool.getInputUTXOList(&input)
		if txn != nil {
			pool.removeTransaction(txn)
		}
	}
}

func (pool *TxPool) isTransactionCleaned(tx *Transaction) error {
	if tx := pool.txnList[tx.Hash()]; tx != nil {
		return errors.New("has transaction in transaction pool" + tx.Hash().String())
	}
	for _, input := range tx.Inputs {
		if poolInput := pool.inputUTXOList[input.ReferKey()]; poolInput != nil {
			return errors.New("has utxo inputs in input list pool" + input.String())
		}
	}
	if tx.TxType == WithdrawFromSideChain {
		payload := tx.Payload.(*PayloadWithdrawFromSideChain)
		for _, hash := range payload.SideChainTransactionHashes {
			if sidechainPoolTx := pool.sidechainTxList[hash]; sidechainPoolTx != nil {
				return errors.New("has sidechain hash in sidechain list pool" + hash.String())
			}
		}
	}
	return nil
}

func (pool *TxPool) isTransactionExisted(tx *Transaction) error {
	if tx := pool.txnList[tx.Hash()]; tx == nil {
		return errors.New("does not have transaction in transaction pool" + tx.Hash().String())
	}
	for _, input := range tx.Inputs {
		if poolInput := pool.inputUTXOList[input.ReferKey()]; poolInput == nil {
			return errors.New("does not have utxo inputs in input list pool" + input.String())
		}
	}
	if tx.TxType == WithdrawFromSideChain {
		payload := tx.Payload.(*PayloadWithdrawFromSideChain)
		for _, hash := range payload.SideChainTransactionHashes {
			if sidechainPoolTx := pool.sidechainTxList[hash]; sidechainPoolTx == nil {
				return errors.New("does not have sidechain hash in sidechain list pool" + hash.String())
			}
		}
	}
	return nil
}
