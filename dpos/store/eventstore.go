package store

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/elastos/Elastos.ELA/blockchain/interfaces"
	"github.com/elastos/Elastos.ELA/core/types"
	"github.com/elastos/Elastos.ELA/dpos/log"
)

var ConsensusEventTable = &interfaces.DBTable{
	Name:       "ConsensusEvent",
	PrimaryKey: 4,
	Indexes:    []uint64{3},
	Fields: []string{
		"StartTime",
		"EndTime",
		"Height",
		"RawData",
	},
}

var ProposalEventTable = &interfaces.DBTable{
	Name:       "ProposalEvent",
	PrimaryKey: 7,
	Indexes:    []uint64{1, 2, 6},
	Fields: []string{
		"Proposal",
		"BlockHash",
		"ReceivedTime",
		"EndTime",
		"Result",
		"ProposalHash",
		"RawData",
	},
}

var VoteEventTable = &interfaces.DBTable{
	Name:       "VoteEvent",
	PrimaryKey: 0,
	Indexes:    nil,
	Fields: []string{
		"ProposalID",
		"Signer",
		"ReceivedTime",
		"Result",
		"RawData",
	},
}

var ViewEventTable = &interfaces.DBTable{
	Name:       "ViewEvent",
	PrimaryKey: 0,
	Indexes:    nil,
	Fields: []string{
		"ConsensusID",
		"OnDutyArbitrator",
		"StartTime",
		"Offset",
	},
}

const (
	MaxEvnetTaskNumber = 10000
)

type addConsensusEventTask struct {
	event *log.ConsensusEvent
	reply chan bool
}

type updateConsensusEventTask struct {
	event *log.ConsensusEvent
	reply chan bool
}

type addProposalEventTask struct {
	event *log.ProposalEvent
	reply chan bool
}

type updateProposalEventTask struct {
	event *log.ProposalEvent
	reply chan bool
}

type addVoteEventTask struct {
	event *log.VoteEvent
	reply chan bool
}

type addViewEventTask struct {
	event *log.ViewEvent
	reply chan bool
}

func (s *DposStore) eventLoop() {
	s.wg.Add(1)

out:
	for {
		select {
		case t := <-s.taskCh:
			now := time.Now()
			switch task := t.(type) {
			case *addConsensusEventTask:
				s.handleAddConsensusEvent(task.event)
				task.reply <- true
				tcall := float64(time.Now().Sub(now)) / float64(time.Second)
				log.Debugf("handle add consensus event task exetime: %g", tcall)
			case *updateConsensusEventTask:
				s.handleUpdateConsensusEvent(task.event)
				task.reply <- true
				tcall := float64(time.Now().Sub(now)) / float64(time.Second)
				log.Debugf("handle update consensus event task exetime: %g", tcall)
			case *addProposalEventTask:
				s.handleAddProposalEvent(task.event)
				task.reply <- true
				tcall := float64(time.Now().Sub(now)) / float64(time.Second)
				log.Debugf("handle add proposal event task exetime: %g", tcall)
			case *updateProposalEventTask:
				s.handleUpdateProposalEvent(task.event)
				task.reply <- true
				tcall := float64(time.Now().Sub(now)) / float64(time.Second)
				log.Debugf("handle update proposal event task exetime: %g", tcall)
			case *addVoteEventTask:
				s.handleVoteProposalEvent(task.event)
				task.reply <- true
				tcall := float64(time.Now().Sub(now)) / float64(time.Second)
				log.Debugf("handle add vote event task exetime: %g", tcall)
			case *addViewEventTask:
				s.handleViewProposalEvent(task.event)
				task.reply <- true
				tcall := float64(time.Now().Sub(now)) / float64(time.Second)
				log.Debugf("handle add view event task exetime: %g", tcall)
			}

		case <-s.quit:
			break out
		}
	}

	s.wg.Done()
}

func (s *DposStore) handleAddConsensusEvent(cons *log.ConsensusEvent) {
	rowID, err := s.addConsensusEvent(cons)
	if err != nil {
		log.Error("add consensus event failed:", err.Error())
	}
	log.Info("add consensus event succeed row id:", rowID)
}

func (s *DposStore) handleUpdateConsensusEvent(cons *log.ConsensusEvent) {
	_, err := s.updateConsensusEvent(cons)
	if err != nil {
		log.Error("update consensus event failed:", err.Error())
	}
}

func (s *DposStore) handleAddProposalEvent(prop *log.ProposalEvent) {
	rowID, err := s.addProposalEvent(prop)
	if err != nil {
		log.Error("add proposal event failed:", err.Error())
	}
	log.Info("add proposal event succeed at row id:", rowID)
}

func (s *DposStore) handleUpdateProposalEvent(prop *log.ProposalEvent) {
	_, err := s.updateProposalEvent(prop)
	if err != nil {
		log.Error("update proposal event failed:", err.Error())
	}
}

func (s *DposStore) handleVoteProposalEvent(vote *log.VoteEvent) {
	rowID, err := s.addVoteEvent(vote)
	if err != nil {
		log.Error("add vote event failed:", err.Error())
	}
	log.Info("add vote event succeed at row id:", rowID)
}

func (s *DposStore) handleViewProposalEvent(view *log.ViewEvent) {
	rowID, err := s.addViewEvent(view)
	if err != nil {
		log.Error("add view event failed:", err.Error())
	}
	log.Info("add view event succeed at row id:", rowID)
}

func (s *DposStore) StartRecordEvent() error {
	err := s.createConsensusEventTable()
	if err != nil {
		log.Debug("create ConsensusEvent table Connect failed:", err.Error())
	}
	err = s.createProposalEventTable()
	if err != nil {
		log.Debug("create ProposalEvent table failed:", err.Error())
	}
	err = s.createVoteEventTable()
	if err != nil {
		log.Debug("create VoteEvent table failed:", err.Error())
	}
	err = s.createViewEventTable()
	if err != nil {
		log.Debug("create ViewEvent table failed:", err.Error())
	}

	go s.eventLoop()

	return nil
}

func (s *DposStore) createConsensusEventTable() error {
	result := s.Create(ConsensusEventTable)
	return result
}

func (s *DposStore) AddConsensusEvent(event interface{}) error {
	e, ok := event.(*log.ConsensusEvent)
	if !ok {
		return errors.New("[AddProposalEvent] invalid proposal event")
	}

	reply := make(chan bool)
	s.taskCh <- &addConsensusEventTask{event: e, reply: reply}
	<-reply

	return nil
}

func (s *DposStore) addConsensusEvent(cons *log.ConsensusEvent) (uint64, error) {
	return s.Insert(ConsensusEventTable, []*interfaces.Field{
		{"StartTime", cons.StartTime.UnixNano()},
		{"Height", cons.Height},
		{"RawData", cons.RawData},
	})
}

func (s *DposStore) UpdateConsensusEvent(event interface{}) error {
	e, ok := event.(*log.ConsensusEvent)
	if !ok {
		return errors.New("[AddProposalEvent] invalid proposal event")
	}

	reply := make(chan bool)
	s.taskCh <- &updateConsensusEventTask{event: e, reply: reply}
	<-reply

	return nil
}

func (s *DposStore) updateConsensusEvent(cons *log.ConsensusEvent) ([]uint64, error) {
	return s.Update(ConsensusEventTable, []*interfaces.Field{
		{"Height", cons.Height}}, []*interfaces.Field{
		{"EndTime", cons.EndTime.UnixNano()}})
}

func (s *DposStore) createProposalEventTable() error {
	return s.Create(ProposalEventTable)
}

func (s *DposStore) AddProposalEvent(event interface{}) error {
	e, ok := event.(*log.ProposalEvent)
	if !ok {
		return errors.New("[AddProposalEvent] invalid proposal event")
	}

	reply := make(chan bool)
	s.taskCh <- &addProposalEventTask{event: e, reply: reply}
	<-reply

	return nil
}

func (s *DposStore) addProposalEvent(event *log.ProposalEvent) (uint64, error) {
	return s.Insert(ProposalEventTable, []*interfaces.Field{
		{"Proposal", event.Proposal},
		{"BlockHash", event.BlockHash.Bytes()},
		{"ReceivedTime", event.ReceivedTime.UnixNano()},
		{"Result", event.Result},
		{"ProposalHash", event.ProposalHash},
		{"RawData", event.RawData},
	})
}
func (s *DposStore) UpdateProposalEvent(event interface{}) error {
	e, ok := event.(*log.ProposalEvent)
	if !ok {
		return errors.New("[UpdateProposalEvent] invalid proposal event")
	}

	reply := make(chan bool)
	s.taskCh <- &updateProposalEventTask{event: e, reply: reply}
	<-reply

	return nil
}

func (s *DposStore) updateProposalEvent(event *log.ProposalEvent) ([]uint64, error) {
	return s.Update(ProposalEventTable, []*interfaces.Field{
		{"Proposal", event.Proposal},
		{"BlockHash", event.BlockHash.Bytes()},
	}, []*interfaces.Field{
		{"EndTime", event.EndTime.UnixNano()},
		{"Result", event.Result},
	})
}

func (s *DposStore) createVoteEventTable() error {
	result := s.Create(VoteEventTable)
	return result
}

func (s *DposStore) AddVoteEvent(event interface{}) error {
	e, ok := event.(*log.VoteEvent)
	if !ok {
		return errors.New("[AddVoteEvent] invalid proposal event")
	}

	reply := make(chan bool)
	s.taskCh <- &addVoteEventTask{event: e, reply: reply}
	<-reply

	return nil
}

func (s *DposStore) addVoteEvent(event *log.VoteEvent) (uint64, error) {
	vote := &types.DPosProposalVote{}
	err := vote.Deserialize(bytes.NewReader(event.RawData))
	if err != nil {
		return 0, err
	}
	var proposalId uint64
	rowIDs, err := s.SelectID(ProposalEventTable, []*interfaces.Field{
		{"ProposalHash", vote.ProposalHash},
	})
	if err != nil || len(rowIDs) != 1 {
		proposalId = math.MaxInt64
	} else {
		proposalId = rowIDs[0]
	}

	fmt.Println("[AddVoteEvent] proposalId = ", proposalId)
	return s.Insert(VoteEventTable, []*interfaces.Field{
		{"ProposalID", proposalId},
		{"Signer", event.Signer},
		{"ReceivedTime", event.ReceivedTime.UnixNano()},
		{"Result", event.Result},
		{"RawData", event.RawData},
	})
}

func (s *DposStore) createViewEventTable() error {
	result := s.Create(ViewEventTable)
	return result
}

func (s *DposStore) AddViewEvent(event interface{}) error {
	e, ok := event.(*log.ViewEvent)
	if !ok {
		return errors.New("[AddViewEvent] invalid proposal event")
	}

	reply := make(chan bool)
	s.taskCh <- &addViewEventTask{event: e, reply: reply}
	<-reply
	return nil
}

func (s *DposStore) addViewEvent(event *log.ViewEvent) (uint64, error) {
	var consensusId uint64
	rowIDs, err := s.SelectID(ConsensusEventTable, []*interfaces.Field{
		{"Height", event.Height},
	})
	if err != nil || len(rowIDs) != 1 {
		consensusId = math.MaxInt64
	} else {
		consensusId = rowIDs[0]
	}

	return s.Insert(ViewEventTable, []*interfaces.Field{
		{"ConsensusID", consensusId},
		{"OnDutyArbitrator", event.OnDutyArbitrator},
		{"StartTime", event.StartTime.UnixNano()},
		{"Offset", event.Offset},
	})
}
