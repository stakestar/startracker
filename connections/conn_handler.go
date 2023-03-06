package connections

import (
	"context"
	"time"

	"github.com/bloxapp/ssv/network/peers"
	"github.com/bloxapp/ssv/utils/tasks"
	libp2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// ConnHandler handles new connections (inbound / outbound) using libp2pnetwork.NotifyBundle
type ConnHandler interface {
	Handle() *libp2pnetwork.NotifyBundle
}

// connHandler implements ConnHandler
type connHandler struct {
	ctx    context.Context
	logger *zap.Logger

	handshaker Handshaker
	connIdx    peers.ConnectionIndex
}

// NewConnHandler creates a new connection handler
func NewConnHandler(ctx context.Context, logger *zap.Logger, handshaker Handshaker, connIdx peers.ConnectionIndex) ConnHandler {
	return &connHandler{
		ctx:        ctx,
		logger:     logger.With(zap.String("who", "ConnHandler")),
		handshaker: handshaker,
		connIdx:    connIdx,
	}
}

// Handle configures a network notifications handler that handshakes and tracks all p2p connections
func (ch *connHandler) Handle() *libp2pnetwork.NotifyBundle {

	q := tasks.NewExecutionQueue(time.Millisecond*10, tasks.WithoutErrors())

	go func() {
		c, cancel := context.WithCancel(ch.ctx)
		defer cancel()
		defer q.Stop()
		q.Start()
		<-c.Done()
	}()

	disconnect := func(net libp2pnetwork.Network, conn libp2pnetwork.Conn) {
		id := conn.RemotePeer()
		_ = net.ClosePeer(id)
	}

	onNewConnection := func(net libp2pnetwork.Network, conn libp2pnetwork.Conn) error {
		id := conn.RemotePeer()
		logger := ch.logger.With(zap.String("targetPeer", id.String()))
		ok, err := ch.handshake(conn)
		if err != nil {
			logger.Debug("could not handshake with peer", zap.Error(err))
		}
		if !ok {
			disconnect(net, conn)
			return err
		}
		if ch.connIdx.Limit(conn.Stat().Direction) {
			disconnect(net, conn)
			return errors.New("reached peers limit")
		}
		return nil
	}

	return &libp2pnetwork.NotifyBundle{
		ConnectedF: func(net libp2pnetwork.Network, conn libp2pnetwork.Conn) {
			if conn == nil || conn.RemoteMultiaddr() == nil {
				return
			}
			id := conn.RemotePeer()
			q.QueueDistinct(func() error {
				return onNewConnection(net, conn)
			}, id.String())
		},
		DisconnectedF: func(net libp2pnetwork.Network, conn libp2pnetwork.Conn) {
			if conn == nil || conn.RemoteMultiaddr() == nil {
				return
			}
			// skip if we are still connected to the peer
			if net.Connectedness(conn.RemotePeer()) == libp2pnetwork.Connected {
				return
			}
		},
	}
}

func (ch *connHandler) handshake(conn libp2pnetwork.Conn) (bool, error) {
	err := ch.handshaker.Handshake(conn)
	if err != nil {
		switch err {
		case peers.ErrIndexingInProcess, errHandshakeInProcess, peerstore.ErrNotFound:
			// ignored errors
			return true, nil
		case errUnknownUserAgent:
			// ignored errors but we still close connection
			return false, nil
		default:
		}
		return false, err
	}
	return true, nil
}
