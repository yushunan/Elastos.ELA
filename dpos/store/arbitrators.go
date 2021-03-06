package store

import (
	"errors"
	"sort"
	"sync"

	"github.com/elastos/Elastos.ELA/blockchain"
	"github.com/elastos/Elastos.ELA/blockchain/interfaces"
	"github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/common/log"
	"github.com/elastos/Elastos.ELA/core/contract"
	"github.com/elastos/Elastos.ELA/core/types"
)

type ArbitratorsConfig struct {
	ArbitratorsCount uint32
	CandidatesCount  uint32
	MajorityCount    uint32
	Store            interfaces.IDposStore
}

type Arbitrators struct {
	store interfaces.IDposStore

	config           ArbitratorsConfig
	DutyChangedCount uint32

	currentArbitrators [][]byte
	currentCandidates  [][]byte

	currentArbitratorsProgramHashes []*common.Uint168
	currentCandidatesProgramHashes  []*common.Uint168

	nextArbitrators [][]byte
	nextCandidates  [][]byte

	listener interfaces.ArbitratorsListener
	lock     sync.Mutex
}

func InitArbitrators(arConfig ArbitratorsConfig) {
	if arConfig.MajorityCount > arConfig.ArbitratorsCount {
		log.Error("Majority count should less or equal than arbitrators count.")
		return
	}
	arbiters := &Arbitrators{
		config: arConfig,
	}
	arbiters.store = arConfig.Store
	blockchain.DefaultLedger.Arbitrators = arbiters
	blockchain.DefaultLedger.Blockchain.NewBlocksListeners = []interfaces.NewBlocksListener{blockchain.DefaultLedger.Arbitrators}
}

func (a *Arbitrators) StartUp() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	block, err := blockchain.DefaultLedger.GetBlockWithHeight(blockchain.DefaultLedger.Blockchain.BlockHeight)
	if err != nil {
		return err
	}
	if blockchain.DefaultLedger.HeightVersions.GetDefaultBlockVersion(block.Height) == 0 {
		if a.currentArbitrators, err = blockchain.DefaultLedger.HeightVersions.GetProducersDesc(block); err != nil {
			return err
		}
	} else {
		if err := a.store.GetArbitrators(a); err != nil {
			return err
		}
	}

	if err := a.updateArbitratorsProgramHashes(); err != nil {
		return err
	}

	return nil
}

func (a *Arbitrators) ForceChange() error {
	block, err := blockchain.DefaultLedger.GetBlockWithHeight(blockchain.DefaultLedger.Blockchain.BlockHeight)
	if err != nil {
		return err
	}

	if err = a.updateNextArbitrators(block); err != nil {
		return err
	}

	if err = a.changeCurrentArbitrators(); err != nil {
		return err
	}

	if a.listener != nil {
		a.listener.OnNewElection(a.nextArbitrators)
	}

	return nil
}

func (a *Arbitrators) OnBlockReceived(b *types.Block, confirmed bool) {
	if confirmed {
		a.lock.Lock()
		a.onChainHeightIncreased(b)
		a.lock.Unlock()
	}
}

func (a *Arbitrators) OnConfirmReceived(p *types.DPosProposalVoteSlot) {
	block, err := blockchain.DefaultLedger.GetBlockWithHash(p.Hash)
	if err != nil {
		log.Error("Error occurred when changing arbitrators, details: ", err)
		return
	}

	a.lock.Lock()
	defer a.lock.Unlock()

	a.onChainHeightIncreased(block)
}

func (a *Arbitrators) GetArbitrators() [][]byte {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.currentArbitrators
}

func (a *Arbitrators) GetCandidates() [][]byte {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.currentCandidates
}

func (a *Arbitrators) GetNextArbitrators() [][]byte {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.nextArbitrators
}

func (a *Arbitrators) GetNextCandidates() [][]byte {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.nextCandidates
}

func (a *Arbitrators) GetArbitratorsProgramHashes() []*common.Uint168 {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.currentArbitratorsProgramHashes
}

func (a *Arbitrators) GetCandidatesProgramHashes() []*common.Uint168 {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.currentCandidatesProgramHashes
}

func (a *Arbitrators) GetOnDutyArbitrator() []byte {
	return a.GetNextOnDutyArbitrator(uint32(0))
}

func (a *Arbitrators) GetNextOnDutyArbitrator(offset uint32) []byte {
	return blockchain.DefaultLedger.HeightVersions.GetNextOnDutyArbitrator(
		blockchain.DefaultLedger.Blockchain.BlockHeight, a.DutyChangedCount, offset)
}

func (a *Arbitrators) HasArbitersMajorityCount(num uint32) bool {
	return num > a.config.MajorityCount
}

func (a *Arbitrators) HasArbitersMinorityCount(num uint32) bool {
	return num >= a.config.ArbitratorsCount-a.config.MajorityCount
}

func (a *Arbitrators) RegisterListener(listener interfaces.ArbitratorsListener) {
	a.listener = listener
}

func (a *Arbitrators) UnregisterListener(listener interfaces.ArbitratorsListener) {
	a.listener = nil
}

func (a *Arbitrators) onChainHeightIncreased(block *types.Block) {
	if a.isNewElection() {
		if err := a.changeCurrentArbitrators(); err != nil {
			log.Error("Change current arbitrators error: ", err)
			return
		}

		if err := a.updateNextArbitrators(block); err != nil {
			log.Error("Update arbitrators error: ", err)
			return
		}

		if a.listener != nil {
			a.listener.OnNewElection(a.nextArbitrators)
		}
	} else {
		a.DutyChangedCount++
		a.store.SaveDposDutyChangedCount(a.DutyChangedCount)
	}
}

func (a *Arbitrators) isNewElection() bool {
	return a.DutyChangedCount == a.config.ArbitratorsCount-1
}

func (a *Arbitrators) changeCurrentArbitrators() error {
	a.currentArbitrators = a.nextArbitrators
	a.currentCandidates = a.nextCandidates

	a.store.SaveCurrentArbitrators(a)

	if err := a.sortArbitrators(); err != nil {
		return err
	}

	if err := a.updateArbitratorsProgramHashes(); err != nil {
		return err
	}

	a.DutyChangedCount = 0
	a.store.SaveDposDutyChangedCount(a.DutyChangedCount)

	return nil
}

func (a *Arbitrators) updateNextArbitrators(block *types.Block) error {
	producers, err := blockchain.DefaultLedger.HeightVersions.GetProducersDesc(block)
	if err != nil {
		return err
	}

	if uint32(len(producers)) < a.config.ArbitratorsCount {
		return errors.New("Producers count less than arbitrators count.")
	}

	a.nextArbitrators = producers[:a.config.ArbitratorsCount]

	if uint32(len(producers)) < a.config.ArbitratorsCount+a.config.CandidatesCount {
		a.nextCandidates = producers[a.config.ArbitratorsCount:]
	} else {
		a.nextCandidates = producers[a.config.ArbitratorsCount : a.config.ArbitratorsCount+a.config.CandidatesCount]
	}

	a.store.SaveNextArbitrators(a)
	return nil
}

func (a *Arbitrators) sortArbitrators() error {

	strArbitrators := make([]string, len(a.currentArbitrators))
	for i := 0; i < len(strArbitrators); i++ {
		strArbitrators[i] = common.BytesToHexString(a.currentArbitrators[i])
	}
	sort.Strings(strArbitrators)

	a.currentArbitrators = make([][]byte, len(strArbitrators))
	for i := 0; i < len(strArbitrators); i++ {
		value, err := common.HexStringToBytes(strArbitrators[i])
		if err != nil {
			return err
		}
		a.currentArbitrators[i] = value
	}

	return nil
}

func (a *Arbitrators) updateArbitratorsProgramHashes() error {
	a.currentArbitratorsProgramHashes = make([]*common.Uint168, len(a.currentArbitrators))
	for index, v := range a.currentArbitrators {
		hash, err := contract.PublicKeyToStandardProgramHash(v)
		if err != nil {
			return err
		}
		a.currentArbitratorsProgramHashes[index] = hash
	}

	a.currentCandidatesProgramHashes = make([]*common.Uint168, len(a.currentCandidates))
	for index, v := range a.currentCandidates {
		hash, err := contract.PublicKeyToStandardProgramHash(v)
		if err != nil {
			return err
		}
		a.currentCandidatesProgramHashes[index] = hash
	}

	return nil
}
