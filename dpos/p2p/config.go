package p2p

import (
	"net"
	"time"

	"github.com/elastos/Elastos.ELA/dpos/p2p/peer"

	"github.com/elastos/Elastos.ELA/p2p"
)

const (
	// defaultConnectTimeout is the default duration we timeout a dial to peer.
	defaultConnectTimeout = time.Second * 30

	// defaultPingInterval is the default interval of time to wait in between
	// sending ping messages.
	defaultPingInterval = time.Second * 10
)

// Config is a descriptor which specifies the server instance configuration.
type Config struct {
	// PID is the public key id of this server.
	PID peer.PID

	// MagicNumber is the peer-to-peer network ID to connect to.
	MagicNumber uint32

	// ProtocolVersion represent the protocol version you are supporting.
	ProtocolVersion uint32

	// Services represent which services you are supporting.
	Services uint64

	// DefaultPort defines the default peer-to-peer port for the network.
	DefaultPort uint16

	// ConnectTimeout is the duration before we timeout a dial to peer.
	ConnectTimeout time.Duration

	// PingInterval is the interval of time to wait in between sending ping
	// messages.
	PingInterval time.Duration

	// SignNonce will be invoked when creating a version message to do the
	// protocol negotiate.  The passed nonce is a 32 bytes length random value,
	// and returns the signature of the nonce value to proof you have the right
	// of the PID(public key) you've provided.
	SignNonce func(nonce []byte) (signature [64]byte)

	// PingNonce will be invoked before send a ping message to the connect peer
	// with the given PID, to get the nonce value within the ping message.
	PingNonce func(pid peer.PID) uint64

	// PongNonce will be invoked before send a pong message to the connect peer
	// with the given PID, to get the nonce value within the pong message.
	PongNonce func(pid peer.PID) uint64

	// MakeEmptyMessage will be invoked to creates a message of the appropriate
	// concrete type based on the command.
	MakeEmptyMessage func(command string) (p2p.Message, error)

	// HandleMessage will be invoked to handle the received message from
	// connected peers.  The peer's public key id will be pass together with
	// the received message.
	HandleMessage func(pid peer.PID, msg p2p.Message)

	// StateNotifier notifies the server peer state changes.
	StateNotifier StateNotifier
}

// normalizeAddress returns addr with the passed default port appended if
// there is not already a port specified.
func normalizeAddress(addr, defaultPort string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}
