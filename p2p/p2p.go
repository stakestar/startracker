package p2p

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	"github.com/bloxapp/ssv/network"
	"github.com/bloxapp/ssv/network/discovery"
	"github.com/bloxapp/ssv/network/forks"
	forksfactory "github.com/bloxapp/ssv/network/forks/factory"
	"github.com/bloxapp/ssv/network/peers"
	"github.com/bloxapp/ssv/network/streams"
	"github.com/bloxapp/ssv/network/topics"
	"github.com/bloxapp/ssv/utils/async"
	"github.com/bloxapp/ssv/utils/tasks"
	connmgrcore "github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pdiscbackoff "github.com/libp2p/go-libp2p/p2p/discovery/backoff"
	connections "github.com/stakestar/startracker/connections"
	"github.com/stakestar/startracker/db"
	"github.com/stakestar/startracker/geodata"
	"go.uber.org/zap"
)

// network states
const (
	stateInitializing int32 = 0
	stateClosing      int32 = 1
	stateClosed       int32 = 2
	stateReady        int32 = 10
)

const (
	peerIndexGCInterval = 15 * time.Minute
)

// p2pNetwork implements network.P2PNetwork
type p2pNetwork struct {
	parentCtx context.Context
	ctx       context.Context
	cancel    context.CancelFunc

	logger *zap.Logger
	fork   forks.Fork
	cfg    *Config

	host        host.Host
	streamCtrl  streams.StreamController
	idx         peers.Index
	disc        discovery.Service
	topicsCtrl  topics.Controller
	msgRouter   network.MessageRouter
	connHandler connections.ConnHandler

	state int32

	backoffConnector *libp2pdiscbackoff.BackoffConnector
	libConnManager   connmgrcore.ConnManager

	db      *db.BoltDB
	geoData *geodata.GeoIP2DB
}

type P2PNetwork interface {
	io.Closer
	// Setup initialize the network layer and starts the libp2p host
	Setup() error
	// Start starts the network
	Start() error
	// UpdateSubnets will update the registered subnets according to active validators
	UpdateSubnets()
}

// New creates a new p2p network
func New(cfg *Config) P2PNetwork {
	ctx, cancel := context.WithCancel(cfg.Ctx)

	logger := cfg.Logger.With(zap.String("who", "p2pNetwork"))

	return &p2pNetwork{
		parentCtx: cfg.Ctx,
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
		fork:      forksfactory.NewFork(cfg.ForkVersion),
		cfg:       cfg,
		msgRouter: cfg.Router,
		state:     stateClosed,
		db:        cfg.DB,
		geoData:   cfg.GeoData,
	}
}

// Host implements HostProvider
func (n *p2pNetwork) Host() host.Host {
	return n.host
}

// Close implements io.Closer
func (n *p2pNetwork) Close() error {
	atomic.SwapInt32(&n.state, stateClosing)
	defer atomic.StoreInt32(&n.state, stateClosed)
	n.cancel()
	if err := n.libConnManager.Close(); err != nil {
		n.logger.Warn("could not close discovery", zap.Error(err))
	}
	if err := n.disc.Close(); err != nil {
		n.logger.Warn("could not close discovery", zap.Error(err))
	}
	if err := n.idx.Close(); err != nil {
		n.logger.Warn("could not close index", zap.Error(err))
	}
	if err := n.topicsCtrl.Close(); err != nil {
		n.logger.Warn("could not close topics controller", zap.Error(err))
	}
	return n.host.Close()
}

// Start starts the discovery service, garbage collector (peer index), and reporting.
func (n *p2pNetwork) Start() error {
	if atomic.SwapInt32(&n.state, stateReady) == stateReady {
		// return errors.New("could not setup network: in ready state")
		return nil
	}

	n.logger.Info("starting p2p network service")

	go n.startDiscovery()

	async.Interval(n.ctx, peerIndexGCInterval, n.idx.GC)

	return nil
}

// startDiscovery starts the required services
// it will try to bootstrap discovery service, and inject a connect function.
// the connect function checks if we can connect to the given peer and if so passing it to the backoff connector.
func (n *p2pNetwork) startDiscovery() {
	discoveredPeers := make(chan peer.AddrInfo, connectorQueueSize)
	go func() {
		ctx, cancel := context.WithCancel(n.ctx)
		defer cancel()
		n.backoffConnector.Connect(ctx, discoveredPeers)
	}()
	err := tasks.Retry(func() error {
		return n.disc.Bootstrap(func(e discovery.PeerEvent) {
			if !n.idx.CanConnect(e.AddrInfo.ID) {
				return
			}
			select {
			case discoveredPeers <- e.AddrInfo:
			default:
				n.logger.Warn("connector queue is full, skipping new peer", zap.String("peerID", e.AddrInfo.ID.String()))
			}
		})
	}, 3)
	if err != nil {
		n.logger.Panic("could not setup discovery", zap.Error(err))
	}
}

// UpdateSubnets will update the registered subnets according to active validators
// NOTE: it won't subscribe to the subnets (use subscribeToSubnets for that)
func (n *p2pNetwork) UpdateSubnets() {
}

// getMaxPeers returns max peers of the given topic.
func (n *p2pNetwork) getMaxPeers(topic string) int {
	if len(topic) == 0 {
		return n.cfg.MaxPeers
	}
	return n.cfg.TopicMaxPeers
}
