package p2p

import (
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	p2pcommons "github.com/bloxapp/ssv/network/commons"
	"github.com/bloxapp/ssv/network/discovery"
	"github.com/bloxapp/ssv/network/peers"
	"github.com/bloxapp/ssv/network/records"
	"github.com/bloxapp/ssv/network/streams"
	"github.com/bloxapp/ssv/utils/commons"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	libp2pdiscbackoff "github.com/libp2p/go-libp2p/p2p/discovery/backoff"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/pkg/errors"
	connections "github.com/stakestar/startracker/connections"
	"go.uber.org/zap"
)

const (
	// defaultReqTimeout is the default timeout used for stream requests
	defaultReqTimeout = 10 * time.Second
	// backoffLow is when we start the backoff exponent interval
	backoffLow = 10 * time.Second
	// backoffLow is when we stop the backoff exponent interval
	backoffHigh = 30 * time.Minute
	// backoffExponentBase is the base of the backoff exponent
	backoffExponentBase = 2.0
	// backoffConnectorCacheSize is the cache size of the backoff connector
	backoffConnectorCacheSize = 1024
	// connectTimeout is the timeout used for connections
	connectTimeout = time.Minute
	// connectorQueueSize is the buffer size of the channel used by the connector
	connectorQueueSize = 256
)

// Setup is used to setup the network
func (n *p2pNetwork) Setup() error {
	if atomic.SwapInt32(&n.state, stateInitializing) == stateReady {
		return errors.New("could not setup network: in ready state")
	}
	// set a seed for rand values
	rand.Seed(time.Now().UnixNano())

	n.logger.Info("configuring p2p network service")

	n.initCfg()

	err := n.SetupHost()
	if err != nil {
		return err
	}
	n.logger = n.logger.With(zap.String("selfPeer", n.host.ID().String()))
	n.logger.Debug("p2p host was configured")

	err = n.SetupServices(n.logger)
	if err != nil {
		return err
	}
	n.logger.Info("p2p services were configured")

	return nil
}

func (n *p2pNetwork) initCfg() {
	if n.cfg.RequestTimeout == 0 {
		n.cfg.RequestTimeout = defaultReqTimeout
	}
	if len(n.cfg.UserAgent) == 0 {
		n.cfg.UserAgent = userAgent(n.cfg.UserAgent)
	}
	if n.cfg.MaxPeers <= 0 {
		n.cfg.MaxPeers = minPeersBuffer
	}
	if n.cfg.TopicMaxPeers <= 0 {
		n.cfg.TopicMaxPeers = minPeersBuffer / 2
	}
}

// SetupHost configures a libp2p host and backoff connector utility
func (n *p2pNetwork) SetupHost() error {
	opts, err := n.cfg.Libp2pOptions(n.fork)
	if err != nil {
		return errors.Wrap(err, "could not create libp2p options")
	}

	limitsCfg := rcmgr.DefaultLimits.AutoScale()
	// TODO: enable and extract resource manager params as config
	rmgr, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limitsCfg))
	if err != nil {
		return errors.Wrap(err, "could not create resource manager")
	}
	opts = append(opts, libp2p.ResourceManager(rmgr))
	host, err := libp2p.New(opts...)
	if err != nil {
		return errors.Wrap(err, "could not create p2p host")
	}
	n.host = host
	n.libConnManager = host.ConnManager()

	backoffFactory := libp2pdiscbackoff.NewExponentialDecorrelatedJitter(backoffLow, backoffHigh, backoffExponentBase, rand.NewSource(0))
	backoffConnector, err := libp2pdiscbackoff.NewBackoffConnector(host, backoffConnectorCacheSize, connectTimeout, backoffFactory)
	if err != nil {
		return errors.Wrap(err, "could not create backoff connector")
	}
	n.backoffConnector = backoffConnector

	return nil
}

// SetupServices configures the required services
func (n *p2pNetwork) SetupServices(logger *zap.Logger) error {
	if err := n.setupStreamCtrl(logger); err != nil {
		return errors.Wrap(err, "could not setup stream controller")
	}
	if err := n.setupPeerServices(); err != nil {
		return errors.Wrap(err, "could not setup peer services")
	}
	if err := n.setupDiscovery(); err != nil {
		return errors.Wrap(err, "could not setup discovery service")
	}

	return nil
}

func (n *p2pNetwork) setupStreamCtrl(logger *zap.Logger) error {
	n.streamCtrl = streams.NewStreamController(n.ctx, n.host, n.fork, n.cfg.RequestTimeout)
	logger.Debug("stream controller is ready")
	return nil
}

func (n *p2pNetwork) setupPeerServices() error {
	libPrivKey, err := p2pcommons.ConvertToInterfacePrivkey(n.cfg.NetworkPrivateKey)
	if err != nil {
		return err
	}

	self := records.NewNodeInfo(n.cfg.ForkVersion, n.cfg.NetworkID)
	self.Metadata = &records.NodeMetadata{
		OperatorID:  n.cfg.OperatorID,
		NodeVersion: commons.GetNodeVersion(),
	}
	getPrivKey := func() crypto.PrivKey {
		return libPrivKey
	}

	n.idx = peers.NewPeersIndex(n.logger, n.host.Network(), self, n.getMaxPeers, getPrivKey, n.fork.Subnets(), 10*time.Minute)
	n.logger.Debug("peers index is ready", zap.String("forkVersion", string(n.cfg.ForkVersion)))

	var ids identify.IDService
	if bh, ok := n.host.(*basichost.BasicHost); ok {
		ids = bh.IDService()
	} else {
		ids, err = identify.NewIDService(n.host, identify.UserAgent(userAgent(n.cfg.UserAgent)))
		if err != nil {
			return errors.Wrap(err, "could not create ID service")
		}
	}

	handshaker := connections.NewHandshaker(n.ctx, &connections.HandshakerCfg{
		Logger:      n.logger,
		Streams:     n.streamCtrl,
		NodeInfoIdx: n.idx,
		States:      n.idx,
		ConnIdx:     n.idx,
		IDService:   ids,
		Network:     n.host.Network(),
	}, n.db, n.geoData)
	n.host.SetStreamHandler(peers.NodeInfoProtocol, handshaker.Handler(n.logger))
	n.logger.Debug("handshaker is ready")

	n.connHandler = connections.NewConnHandler(n.ctx, n.logger, handshaker, n.idx)
	n.host.Network().Notify(n.connHandler.Handle())
	n.logger.Debug("connection handler is ready")
	return nil
}

func (n *p2pNetwork) setupDiscovery() error {
	ipAddr, err := p2pcommons.IPAddr()
	if err != nil {
		return errors.Wrap(err, "could not get ip addr")
	}
	var discV5Opts *discovery.DiscV5Options
	if n.cfg.Discovery != localDiscvery { // otherwise, we are in local scenario
		discV5Opts = &discovery.DiscV5Options{
			IP:         ipAddr.String(),
			BindIP:     net.IPv4zero.String(),
			Port:       n.cfg.UDPPort,
			TCPPort:    n.cfg.TCPPort,
			NetworkKey: n.cfg.NetworkPrivateKey,
			Bootnodes:  n.cfg.TransformBootnodes(),
			OperatorID: n.cfg.OperatorID,
		}
		n.logger.Info("discovery: using discv5", zap.Strings("bootnodes", discV5Opts.Bootnodes))
	} else {
		n.logger.Info("discovery: using mdns (local)")
	}
	discOpts := discovery.Options{
		Host:        n.host,
		DiscV5Opts:  discV5Opts,
		ConnIndex:   n.idx,
		SubnetsIdx:  n.idx,
		HostAddress: n.cfg.HostAddress,
		HostDNS:     n.cfg.HostDNS,
		ForkVersion: n.cfg.ForkVersion,
	}
	disc, err := discovery.NewService(n.ctx, n.logger, discOpts)
	if err != nil {
		return err
	}
	n.disc = disc

	n.logger.Debug("discovery is ready")

	return nil
}
