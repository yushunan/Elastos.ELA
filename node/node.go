package node

import (
	"errors"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	chain "github.com/elastos/Elastos.ELA/blockchain"
	"github.com/elastos/Elastos.ELA/bloom"
	. "github.com/elastos/Elastos.ELA/common"
	. "github.com/elastos/Elastos.ELA/common/config"
	"github.com/elastos/Elastos.ELA/common/log"
	. "github.com/elastos/Elastos.ELA/core/types"
	"github.com/elastos/Elastos.ELA/mempool"
	"github.com/elastos/Elastos.ELA/p2p"
	"github.com/elastos/Elastos.ELA/p2p/msg"
	"github.com/elastos/Elastos.ELA/protocol"
)

const (
	// dialTimeout is the time limit to finish dialing to an address.
	dialTimeout = 10 * time.Second

	// stateMonitorInterval is the interval to monitor connection and syncing state.
	stateMonitorInterval = 10 * time.Second

	// pingInterval is the interval of time to wait in between sending ping
	// messages.
	pingInterval = 30 * time.Second

	// syncBlockTimeout is the time limit to trigger restart sync block.
	syncBlockTimeout = 30 * time.Second
)

var LocalNode *node

type Semaphore chan struct{}

func MakeSemaphore(n int) Semaphore {
	return make(chan struct{}, n)
}

func (s Semaphore) acquire() { s <- struct{}{} }
func (s Semaphore) release() { <-s }

type node struct {
	//sync.RWMutex	//The Lock not be used as expected to use function channel instead of lock
	state             int32         // node state
	lastActive        time.Time     // The lastActive of node
	id                uint64        // The nodes's id
	version           uint32        // The network protocol the node used
	services          uint64        // The services the node supplied
	relay             bool          // The relay capability of the node (merge into capability flag)
	height            uint64        // The node latest block height
	external          bool          // Indicate if this is an external node
	txnCnt            uint64        // The transactions be transmit by this node
	rxTxnCnt          uint64        // The transaction received by this node
	link                            // The link status and information
	neighbours                      // The neighbor node connect with currently node except itself
	mempool.TxPool                  // Unconfirmed transaction pool
	mempool.BlockPool               // Unconfirmed block pool
	idCache                         // The buffer to store the id of the items which already be processed
	filter            *bloom.Filter // The bloom filter of a spv node
	naFilter          p2p.NAFilter
	/*
	 * |--|--|--|--|--|--|isSyncFailed|isSyncHeaders|
	 */
	syncFlag           uint8
	flagLock           sync.RWMutex
	requestedBlockLock sync.RWMutex
	ConnectingNodes
	KnownAddressList
	DefaultMaxPeers    uint
	RequestedBlockList map[Uint256]time.Time
	syncTimer          *syncTimer
	SyncBlkReqSem      Semaphore
	StartHash          Uint256
	StopHash           Uint256
}

type ConnectingNodes struct {
	sync.RWMutex
	List map[string]struct{}
}

func (cn *ConnectingNodes) init() {
	cn.List = make(map[string]struct{})
}

func (cn *ConnectingNodes) add(addr string) bool {
	cn.Lock()
	defer cn.Unlock()
	_, ok := cn.List[addr]
	if !ok {
		cn.List[addr] = struct{}{}
	}
	return !ok
}

func (cn *ConnectingNodes) del(addr string) {
	cn.Lock()
	defer cn.Unlock()
	delete(cn.List, addr)
}

func NewNode(conn net.Conn, inbound bool) *node {
	log.Debugf("new connection %s <-> %s with %s",
		conn.LocalAddr(), conn.RemoteAddr(), conn.RemoteAddr().Network())

	addr := conn.RemoteAddr().String()
	ip, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Error("node init err:", err)
	}
	n := node{
		link: link{
			magic:     Parameters.Magic,
			addr:      addr,
			ip:        net.ParseIP(ip),
			conn:      conn,
			inbound:   inbound,
			sendQueue: make(chan p2p.Message, 1),
			quit:      make(chan struct{}),
		},
		filter: bloom.LoadFilter(nil),
	}

	n.handler = NewHandlerBase(&n)
	n.start(inbound)

	return &n
}

func InitLocalNode() protocol.Noder {
	LocalNode = &node{
		id:                 rand.New(rand.NewSource(time.Now().Unix())).Uint64(),
		version:            protocol.ProtocolVersion,
		services:           protocol.FlagNode | protocol.OpenService,
		relay:              true,
		SyncBlkReqSem:      MakeSemaphore(protocol.MaxSyncHdrReq),
		RequestedBlockList: make(map[Uint256]time.Time),
		syncTimer:          newSyncTimer(stopSyncing),
		height:             uint64(chain.DefaultLedger.Blockchain.GetBestHeight()),
		link: link{
			magic: Parameters.Magic,
			port:  Parameters.NodePort,
		},
	}

	if !Parameters.OpenService {
		LocalNode.services &^= protocol.OpenService
	}

	LocalNode.neighbours.init()
	LocalNode.ConnectingNodes.init()
	LocalNode.KnownAddressList.init()
	LocalNode.TxPool.Init()
	LocalNode.BlockPool.Init()
	LocalNode.idCache.init()
	LocalNode.handshakeQueue.init()
	LocalNode.initConnection()

	go func() {
		LocalNode.ConnectNodes()
		LocalNode.waitForNeighbourConnections()

		ticker := time.NewTicker(stateMonitorInterval)
		for {
			go LocalNode.ConnectNodes()
			go LocalNode.SyncBlocks()
			<-ticker.C
		}
	}()

	go LocalNode.nodeHeartBeat()
	go monitorNodeState()
	return LocalNode
}

func (node *node) nodeHeartBeat() {
	ticker := time.NewTicker(pingInterval)
	for {
		log.Info("node heart beat")
		for _, peer := range node.GetNeighborNodes() {
			if time.Now().Sub(peer.GetLastActive()) > time.Minute {
				log.Warn("does not update last active time for 1 minutes.")
				peer.Disconnect()
			}
		}
		<-ticker.C
	}
}

func DisconnectNode(node protocol.Noder) {
	if n, ok := LocalNode.DelNeighborNode(node); ok {
		n.Disconnect()
	}
}

func (node *node) AddToConnectingList(addr string) bool {
	return node.ConnectingNodes.add(addr)
}

func (node *node) RemoveFromConnectingList(addr string) {
	node.ConnectingNodes.del(addr)
}

func (node *node) UpdateInfo(t time.Time, version uint32, services uint64,
	port uint16, nonce uint64, relay bool, height uint64) {

	node.lastActive = t
	node.id = nonce
	node.version = version
	node.services = services
	node.port = port
	node.relay = relay
	node.height = uint64(height)
}

func (node *node) State() protocol.State {
	return protocol.State(atomic.LoadInt32(&node.state))
}

func (node *node) SetState(state protocol.State) {
	atomic.StoreInt32(&node.state, int32(state))
}

func (node *node) ID() uint64 {
	return node.id
}

func (node *node) GetConn() net.Conn {
	return node.conn
}

func (node *node) Port() uint16 {
	return node.port
}

func (node *node) IsExternal() bool {
	return node.external
}

func (node *node) HttpInfoPort() int {
	return int(node.httpInfoPort)
}

func (node *node) SetHttpInfoPort(nodeInfoPort uint16) {
	node.httpInfoPort = nodeInfoPort
}

func (node *node) IsRelay() bool {
	return node.relay
}

func (node *node) Version() uint32 {
	return node.version
}

func (node *node) Services() uint64 {
	return node.services
}

func (node *node) IncRxTxnCnt() {
	node.rxTxnCnt++
}

func (node *node) GetTxnCnt() uint64 {
	return node.txnCnt
}

func (node *node) GetRxTxnCnt() uint64 {
	return node.rxTxnCnt
}

func (node *node) Height() uint64 {
	return node.height
}

func (node *node) SetHeight(height uint64) {
	node.height = height
}

func (node *node) SetLastActive(now time.Time) {
	node.lastActive = now
}

func (node *node) GetLastActive() time.Time {
	return node.lastActive
}

func (node *node) Addr() string {
	return node.addr
}

func (node *node) IP() net.IP {
	return node.ip
}

func (node *node) SetNAFilter(filter p2p.NAFilter) {
	node.naFilter = filter
}

func (node *node) NAFilter() p2p.NAFilter {
	return node.naFilter
}

func (node *node) WaitForSyncFinish(interrupt <-chan struct{}) {
	if len(Parameters.SeedList) == 0 {
		return
	}

out:
	for {
		select {
		case <-time.After(time.Second * 5):
			addresses, heights := node.GetInternalNeighborAddressAndHeights()
			// Can not connect to neighbors.
			if len(heights) == 0 {
				break out
			}
			log.Debug("others height is (internal only) ", heights)
			log.Debug("others address is (internal only) ", addresses)
			// Sync finished.
			if node.IsCurrent() {
				LocalNode.SetSyncHeaders(false)
				break out
			}

		case <-interrupt:
			break out
		}
	}
}

func (node *node) waitForNeighbourConnections() {
	if len(Parameters.SeedList) <= 0 {
		return
	}
	ticker := time.NewTicker(time.Millisecond * 100)
	timer := time.NewTimer(time.Second * 10)
	for {
		select {
		case <-ticker.C:
			if node.GetNeighbourCount() > 0 {
				log.Info("successfully connect to neighbours, neighbour count:", node.GetNeighbourCount())
				return
			}
		case <-timer.C:
			log.Warn("cannot connect to any neighbours, waiting for neighbour connections time out")
			return
		}
	}
}

func (node *node) LoadFilter(filter *msg.FilterLoad) {
	node.filter.Reload(filter)
}

func (node *node) BloomFilter() *bloom.Filter {
	return node.filter
}

func (node *node) Relay(from protocol.Noder, message interface{}) error {
	log.Debug()
	if from != nil && LocalNode.IsSyncHeaders() {
		return nil
	}

	for _, nbr := range node.GetNeighborNodes() {
		if from == nil || nbr.ID() != from.ID() {

			switch message := message.(type) {
			case *Transaction:
				log.Debug("Relay transaction message")
				if nbr.BloomFilter().IsLoaded() && nbr.BloomFilter().MatchTxAndUpdate(message) {
					inv := msg.NewInventory()
					txID := message.Hash()
					inv.AddInvVect(msg.NewInvVect(msg.InvTypeTx, &txID))
					go nbr.SendMessage(inv)
					continue
				}

				if nbr.IsRelay() {
					nbr.SendMessage(msg.NewTx(message))
					node.txnCnt++
				}
			case *DposBlock:
				log.Debug("Relay block message")
				if nbr.BloomFilter().IsLoaded() && message.BlockFlag {
					inv := msg.NewInventory()
					blockHash := message.Block.Hash()
					inv.AddInvVect(msg.NewInvVect(msg.InvTypeBlock, &blockHash))
					go nbr.SendMessage(inv)
					continue
				}

				if nbr.IsRelay() {
					nbr.SendMessage(msg.NewBlock(message))
				}
			default:
				log.Warn("unknown relay message type")
				return errors.New("unknown relay message type")
			}
		}
	}

	return nil
}

func (node *node) IsSyncHeaders() bool {
	node.flagLock.RLock()
	defer node.flagLock.RUnlock()
	if (node.syncFlag & 0x01) == 0x01 {
		return true
	} else {
		return false
	}
}

func (node *node) SetSyncHeaders(b bool) {
	node.flagLock.Lock()
	defer node.flagLock.Unlock()
	if b == true {
		node.syncFlag = node.syncFlag | 0x01
	} else {
		node.syncFlag = node.syncFlag & 0xFE
	}
}

// IsCurrent returns if node believes it was synced to current height.
func (node *node) IsCurrent() bool {
	addresses, heights := node.GetInternalNeighborAddressAndHeights()
	log.Info("internal nbr height-->", heights, chain.DefaultLedger.Blockchain.BlockHeight)
	log.Info("internal nbr address ", addresses)
	return CompareHeight(uint64(chain.DefaultLedger.Blockchain.BlockHeight), heights) > 0
}

func CompareHeight(localHeight uint64, heights []uint64) int {
	for _, height := range heights {
		if localHeight < height {
			return -1
		}
	}
	return 1
}

func (node *node) GetRequestBlockList() map[Uint256]time.Time {
	return node.RequestedBlockList
}

func (node *node) IsRequestedBlock(hash Uint256) bool {
	node.requestedBlockLock.Lock()
	defer node.requestedBlockLock.Unlock()
	_, ok := node.RequestedBlockList[hash]
	return ok
}

func (node *node) AddRequestedBlock(hash Uint256) {
	node.requestedBlockLock.Lock()
	defer node.requestedBlockLock.Unlock()
	node.RequestedBlockList[hash] = time.Now()
}

func (node *node) ResetRequestedBlock() {
	node.requestedBlockLock.Lock()
	defer node.requestedBlockLock.Unlock()

	node.RequestedBlockList = make(map[Uint256]time.Time)
}

func (node *node) DeleteRequestedBlock(hash Uint256) {
	node.requestedBlockLock.Lock()
	defer node.requestedBlockLock.Unlock()
	_, ok := node.RequestedBlockList[hash]
	if ok == false {
		return
	}
	delete(node.RequestedBlockList, hash)
}

func (node *node) AcqSyncBlkReqSem() {
	node.SyncBlkReqSem.acquire()
}

func (node *node) RelSyncBlkReqSem() {
	node.SyncBlkReqSem.release()
}

func (node *node) SetStartHash(hash Uint256) {
	node.StartHash = hash
}

func (node *node) GetStartHash() Uint256 {
	return node.StartHash
}

func (node *node) SetStopHash(hash Uint256) {
	node.StopHash = hash
}

func (node *node) GetStopHash() Uint256 {
	return node.StopHash
}

func (node *node) RegisterTxPoolListener(listener protocol.TxnPoolListener) {
	node.TxPool.Listeners[listener] = nil
}

func (node *node) UnregisterTxPoolListener(listener protocol.TxnPoolListener) {
	delete(node.TxPool.Listeners, listener)
}
