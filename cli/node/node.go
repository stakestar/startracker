package node

import (
	"fmt"
	"log"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	forksprotocol "github.com/bloxapp/ssv/protocol/forks"
	"github.com/bloxapp/ssv/utils"
	"github.com/bloxapp/ssv/utils/format"
	"github.com/stakestar/startracker/api"
	"github.com/stakestar/startracker/cli/args"
	"github.com/stakestar/startracker/db"
	"github.com/stakestar/startracker/eth"
	"github.com/stakestar/startracker/geodata"
	"github.com/stakestar/startracker/keys"
	"github.com/stakestar/startracker/logger"
	"github.com/stakestar/startracker/p2p"
)

type config struct {
	P2pNetworkConfig p2p.Config `yaml:"p2p"`
	DbPath           string     `yaml:"dbPath" env:"DB_PATH" env-description:"Path to database file" env-default:"data/nodes.db"`
	GeoDataDbPath    string     `yaml:"geoDataDbPath" env:"GEO_DATA_DB_PATH" env-description:"Path to geo data database file" env-default:"GeoLite2-City.mmdb"`
	EventsConfig     eth.Config `yaml:"eventsConfig"`
}

var cfg config

var globalArgs args.GlobalArgs

// StartNodeCmd is the command to start SSV tracker
var StartNodeCmd = &cobra.Command{
	Use:   "start-node",
	Short: "Starts an instance of SSV node",
	Run: func(cmd *cobra.Command, args []string) {
		if globalArgs.ConfigPath != "" {
			if err := cleanenv.ReadConfig(globalArgs.ConfigPath, &cfg); err != nil {
				log.Fatal("Error reading config file", err)
			}
		} else {
			log.Fatal("Config path is required")
		}

		logger, err := logger.Create(globalArgs.LogLevel)
		if err != nil {
			fmt.Println("Error initializing logger")
		}
		defer logger.Sync()

		boltDb, err := db.NewBoltDB(cfg.DbPath)
		if err != nil {
			logger.Fatal("Error connecting to database", zap.Error(err))
			return
		}
		defer boltDb.Close()

		geoDb, err := geodata.NewGeoIP2DB(cfg.GeoDataDbPath)
		if err != nil {
			logger.Fatal("Error connecting to geo database", zap.Error(err))
			return
		}
		defer geoDb.Close()

		events, err := eth.NewEthEvents(cmd.Context(), &cfg.EventsConfig, boltDb, logger)
		if err != nil {
			logger.Fatal("Error connecting to eth events", zap.Error(err))
			return
		}
		err = events.Start()
		if err != nil {
			logger.Fatal("Error starting eth events", zap.Error(err))
			return
		}
		err = events.FetchAddedOperatorEvents()
		if err != nil {
			logger.Fatal("Error fetching added operator events", zap.Error(err))
			return
		}
		err = events.ListenAddedOperatorEvents()
		if err != nil {
			logger.Fatal("Error fetching added operator events", zap.Error(err))
			return
		}

		forkVersion := forksprotocol.GenesisForkVersion

		cfg.P2pNetworkConfig.Ctx = cmd.Context()

		_, operatorPublicKey, err := keys.GenerateKeys()
		if err != nil {
			logger.Fatal("failed to generate operator keys", zap.Error(err))
		}

		p2pNetwork := setupP2P(forkVersion, operatorPublicKey, boltDb, geoDb, logger)

		if err := p2pNetwork.Setup(); err != nil {
			logger.Fatal("failed to setup network", zap.Error(err))
		}
		if err := p2pNetwork.Start(); err != nil {
			logger.Fatal("failed to start network", zap.Error(err))
		}

		api := api.New(logger, boltDb)
		api.Start()

	},
}

func init() {
	args.ProcessArgs(&globalArgs, StartNodeCmd)
}

func setupP2P(forkVersion forksprotocol.ForkVersion, operatorPubKey string, db *db.BoltDB, geodata *geodata.GeoIP2DB, logger *zap.Logger) p2p.P2PNetwork {
	netPrivKey, err := utils.ECDSAPrivateKey(logger, "")
	if err != nil {
		logger.Fatal("failed to setup network private key", zap.Error(err))
	}

	fmt.Println(operatorPubKey)

	cfg.P2pNetworkConfig.Subnets = "0xffffffffffffffffffffffffffffffff"
	cfg.P2pNetworkConfig.NetworkPrivateKey = netPrivKey
	cfg.P2pNetworkConfig.Logger = logger
	cfg.P2pNetworkConfig.ForkVersion = forkVersion
	cfg.P2pNetworkConfig.OperatorID = format.OperatorID([]byte(operatorPubKey))
	cfg.P2pNetworkConfig.DB = db
	cfg.P2pNetworkConfig.GeoData = geodata
	cfg.P2pNetworkConfig.MaxPeers = 500

	return p2p.New(&cfg.P2pNetworkConfig)
}
