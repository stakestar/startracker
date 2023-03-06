package connections

import (
	"context"
	"strings"
	"time"

	"github.com/bloxapp/ssv/network/peers"
	"github.com/bloxapp/ssv/network/records"
	"github.com/bloxapp/ssv/network/streams"
	forksprotocol "github.com/bloxapp/ssv/protocol/forks"
	libp2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/stakestar/startracker/db"
	"github.com/stakestar/startracker/geodata"
	"go.uber.org/zap"
)

const (
	// userAgentKey is the key used by libp2p to save user agent
	userAgentKey = "AgentVersion"
)

// errHandshakeInProcess is thrown when and handshake process for that peer is already running
var errHandshakeInProcess = errors.New("handshake already in process")

// errUnknownUserAgent is thrown when a peer has an unknown user agent
var errUnknownUserAgent = errors.New("user agent is unknown")

// HandshakeFilter can be used to filter nodes once we handshaked with them
type HandshakeFilter func(info *records.NodeInfo) (bool, error)

// Handshaker is the interface for handshaking with peers.
// it uses node info protocol to exchange information with other nodes and decide whether we want to connect.
//
// NOTE: due to compatibility with v0,
// we accept nodes with user agent as a fallback when the new protocol is not supported.
type Handshaker interface {
	Handshake(conn libp2pnetwork.Conn) error
	Handler() libp2pnetwork.StreamHandler
}

type handshaker struct {
	ctx context.Context

	logger *zap.Logger

	streams     streams.StreamController
	nodeInfoIdx peers.NodeInfoIndex
	states      peers.NodeStates
	connIdx     peers.ConnectionIndex
	ids         identify.IDService
	net         libp2pnetwork.Network
	db          *db.BoltDB
	geodata     *geodata.GeoIP2DB
}

// HandshakerCfg is the configuration for creating an handshaker instance
type HandshakerCfg struct {
	Logger      *zap.Logger
	Network     libp2pnetwork.Network
	Streams     streams.StreamController
	NodeInfoIdx peers.NodeInfoIndex
	States      peers.NodeStates
	ConnIdx     peers.ConnectionIndex
	IDService   identify.IDService
}

// NewHandshaker creates a new instance of handshaker
func NewHandshaker(ctx context.Context, cfg *HandshakerCfg, db *db.BoltDB, geoData *geodata.GeoIP2DB) Handshaker {
	h := &handshaker{
		ctx:         ctx,
		logger:      cfg.Logger.With(zap.String("where", "Handshaker")),
		streams:     cfg.Streams,
		nodeInfoIdx: cfg.NodeInfoIdx,
		connIdx:     cfg.ConnIdx,
		ids:         cfg.IDService,
		states:      cfg.States,
		net:         cfg.Network,
		db:          db,
		geodata:     geoData,
	}
	return h
}

// Handler returns the handshake handler
func (h *handshaker) Handler() libp2pnetwork.StreamHandler {
	return func(stream libp2pnetwork.Stream) {
		// start by marking the peer as pending
		pid := stream.Conn().RemotePeer()
		maddr := stream.Conn().RemoteMultiaddr()

		pidStr := pid.String()

		req, res, done, err := h.streams.HandleStream(stream)
		defer done()
		if err != nil {
			return
		}

		logger := h.logger.With(zap.String("otherPeer", pidStr))

		var ni records.NodeInfo
		err = ni.Consume(req)
		if err != nil {
			logger.Warn("could not consume node info request", zap.Error(err))
			return
		}
		// process the node info in a new goroutine so we won't block the stream
		go func() {
			h.processIncomingNodeInfo(maddr, ni)
		}()

		self, err := h.nodeInfoIdx.SelfSealed()
		if err != nil {
			logger.Warn("could not seal self node info", zap.Error(err))
			return
		}
		if err := res(self); err != nil {
			logger.Warn("could not send self node info", zap.Error(err))
			return
		}
	}
}

func (h *handshaker) processIncomingNodeInfo(maddr ma.Multiaddr, ni records.NodeInfo) {
	ip, err := h.getIPAddressFromMultiaddr(maddr)
	if err != nil {
		h.logger.Warn("could not get ip address from multiaddr", zap.Error(err))
	}

	ipGeoData, err := h.geodata.GetGeoDataFromIPAddress(ip)
	if err != nil {
		h.logger.Warn("could not get geo data from ip address", zap.Error(err))
	}

	geoData := &db.GeoData{
		City:           ipGeoData.City.Names["en"],
		CountryName:    ipGeoData.Country.Names["en"],
		CountryCode:    ipGeoData.Country.IsoCode,
		Latitude:       ipGeoData.Location.Latitude,
		Longitude:      ipGeoData.Location.Longitude,
		AccuracyRadius: ipGeoData.Location.AccuracyRadius,
	}

	nodeData := &db.NodeData{
		IPAddress:   ip,
		GeoData:     *geoData,
		NodeVersion: ni.Metadata.NodeVersion,
		OperatorID:  ni.Metadata.OperatorID,
	}
	h.logger.Info("Node Data", zap.Any("data", nodeData))
	h.db.StoreNodeData(nodeData)
}

// preHandshake makes sure that we didn't reach peers limit and have exchanged framework information (libp2p)
// with the peer on the other side of the connection.
// it should enable us to know the supported protocols of peers we connect to
func (h *handshaker) preHandshake(conn libp2pnetwork.Conn) error {
	ctx, cancel := context.WithTimeout(h.ctx, time.Second*15)
	defer cancel()
	select {
	case <-ctx.Done():
		return errors.New("identity protocol (libp2p) timeout")
	case <-h.ids.IdentifyWait(conn):
	}
	return nil
}

// Handshake initiates handshake with the given conn
func (h *handshaker) Handshake(conn libp2pnetwork.Conn) error {
	pid := conn.RemotePeer()
	maddr := conn.RemoteMultiaddr()
	// check if the peer is known before we continue
	ni, err := h.getNodeInfo(pid)
	if err != nil || ni != nil {
		return err
	}
	if err := h.preHandshake(conn); err != nil {
		return errors.Wrap(err, "could not perform pre-handshake")
	}
	ni, err = h.nodeInfoFromStream(conn)
	if err != nil {
		// fallbacks to user agent
		ni, err = h.nodeInfoFromUserAgent(conn)
		if err != nil {
			return err
		}
	}
	if ni == nil {
		return errors.New("empty node info")
	}
	h.processIncomingNodeInfo(maddr, *ni)

	return nil
}

func (h *handshaker) getNodeInfo(pid peer.ID) (*records.NodeInfo, error) {
	ni, err := h.nodeInfoIdx.GetNodeInfo(pid)
	if err != nil && err != peers.ErrNotFound {
		return nil, errors.Wrap(err, "could not read node info")
	}
	if ni != nil {
		switch h.states.State(pid) {
		case peers.StateIndexing:
			return nil, errHandshakeInProcess
		case peers.StatePruned:
			return nil, errors.Errorf("pruned peer [%s]", pid.String())
		case peers.StateReady:
			return ni, nil
		default: // unknown > continue the flow
		}
	}
	return nil, nil
}

func (h *handshaker) nodeInfoFromStream(conn libp2pnetwork.Conn) (*records.NodeInfo, error) {
	res, err := h.net.Peerstore().FirstSupportedProtocol(conn.RemotePeer(), peers.NodeInfoProtocol)
	if err != nil {
		return nil, errors.Wrapf(err, "could not check supported protocols of peer %s",
			conn.RemotePeer().String())
	}
	data, err := h.nodeInfoIdx.SelfSealed()
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, errors.Errorf("peer [%s] doesn't supports handshake protocol", conn.RemotePeer().String())
	}
	resBytes, err := h.streams.Request(conn.RemotePeer(), peers.NodeInfoProtocol, data)
	if err != nil {
		return nil, err
	}
	var ni records.NodeInfo
	err = ni.Consume(resBytes)
	if err != nil {
		return nil, err
	}
	return &ni, nil
}

func (h *handshaker) nodeInfoFromUserAgent(conn libp2pnetwork.Conn) (*records.NodeInfo, error) {
	pid := conn.RemotePeer()
	uaRaw, err := h.net.Peerstore().Get(pid, userAgentKey)
	if err != nil {
		if err == peerstore.ErrNotFound {
			// if user agent wasn't found, retry libp2p identify after 100ms
			time.Sleep(time.Millisecond * 100)
			if err := h.preHandshake(conn); err != nil {
				return nil, err
			}
			uaRaw, err = h.net.Peerstore().Get(pid, userAgentKey)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	ua, ok := uaRaw.(string)
	if !ok {
		return nil, errors.New("could not cast ua to string")
	}
	parts := strings.Split(ua, ":")
	if len(parts) < 2 { // too old or unknown
		h.logger.Debug("user agent is unknown", zap.String("ua", ua))
		return nil, errUnknownUserAgent
	}
	// TODO: don't assume network is the same
	ni := records.NewNodeInfo(forksprotocol.GenesisForkVersion, h.nodeInfoIdx.Self().NetworkID)
	ni.Metadata = &records.NodeMetadata{
		NodeVersion: parts[1],
	}
	// extract operator id if exist
	if len(parts) > 3 {
		ni.Metadata.OperatorID = parts[3]
	}
	return ni, nil
}

func (h *handshaker) getIPAddressFromMultiaddr(maddr ma.Multiaddr) (string, error) {
	ip, _ := ma.SplitFirst(maddr)

	return ip.Value(), nil
}
