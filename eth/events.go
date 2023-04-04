package eth

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/bloxapp/ssv/eth1"
	"github.com/bloxapp/ssv/eth1/abiparser"
	"github.com/bloxapp/ssv/logging/fields"
	"github.com/bloxapp/ssv/utils/format"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/stakestar/startracker/db"
	"go.uber.org/zap"
)

const blocksInBatch uint64 = 10000

var startBlock = big.NewInt(8661727)

type Config struct {
	RPCUrl          string `yaml:"RPCUrl" env:"ETH_RPC_URL" env-description:"Ethereum RPC URL"`
	ContractAddress string `yaml:"ContractAddress" env:"ETH_CONTRACT_ADDRESS" env-description:"Ethereum contract address"`
}

type EthEvents struct {
	config *Config
	client *ethclient.Client
	ctx    context.Context

	logger *zap.Logger

	db *db.BoltDB
}

func NewEthEvents(ctx context.Context, config *Config, db *db.BoltDB, logger *zap.Logger) (*EthEvents, error) {
	return &EthEvents{
		config: config,
		ctx:    ctx,
		logger: logger,
		db:     db,
	}, nil
}

func (e *EthEvents) Start() error {
	if err := e.connect(); err != nil {
		return err
	}

	return nil
}

func (e *EthEvents) connect() error {
	client, err := ethclient.Dial(e.config.RPCUrl)
	if err != nil {
		return err
	}
	e.client = client
	return nil
}

func (e *EthEvents) FetchAddedOperatorEvents() error {
	contractAbi, err := abi.JSON(strings.NewReader(eth1.ContractABI(0)))
	if err != nil {
		e.logger.Error("failed to parse contract abi", zap.Error(err))
		return err
	}

	abiParser := eth1.NewParser(e.logger, 0)

	e.logger.Info("fetching operator added events")

	currentBlock, err := e.client.BlockNumber(e.ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current block")
	}

	latestBlock, err := e.db.GetLastBlock()
	if err != nil {
		e.logger.Error("failed to get latest block", zap.Error(err))
	}

	if latestBlock != nil && latestBlock.Uint64() > startBlock.Uint64() {
		startBlock = latestBlock
	}

	e.logger.Info("fetching events from block", zap.String("from", startBlock.String()))

	var toBlock *big.Int
	var stop = false
	for {
		if currentBlock-startBlock.Uint64() > blocksInBatch {
			toBlock = big.NewInt(int64(startBlock.Uint64() + blocksInBatch))
		} else {
			toBlock = big.NewInt(int64(currentBlock))
			stop = true
		}

		err = e.fetchEvents(startBlock, toBlock, contractAbi, abiParser)
		if err != nil {
			return err
		}

		startBlock = toBlock
		err = e.db.SaveLastBlock(toBlock)
		if err != nil {
			e.logger.Error("failed to save last block", zap.Error(err))
		}
		if stop {
			e.logger.Info("finished fetching events")
			break
		}
	}

	return nil
}

func (e *EthEvents) reconnect(retries int, delay time.Duration) {
	for i := 0; i < retries; i++ {
		if err := e.connect(); err != nil {
			e.logger.Warn("connection failed, retrying...", zap.Int("retry", i+1), zap.Error(err))
			time.Sleep(delay)
			continue
		}

		err := e.ListenAddedOperatorEvents()
		if err != nil {
			e.logger.Error("failed to listen after reconnect", zap.Error(err))
		}

		return
	}

	e.logger.Panic("failed to reconnect to eth node")
}

func (e *EthEvents) ListenAddedOperatorEvents() error {
	contractAbi, err := abi.JSON(strings.NewReader(eth1.ContractABI(0)))
	if err != nil {
		e.logger.Error("failed to parse contract abi", zap.Error(err))
		return err
	}

	abiParser := eth1.NewParser(e.logger, 0)

	e.logger.Info("listening to operator added events")

	contractAddress := common.HexToAddress(e.config.ContractAddress)

	eventSignature := []byte("OperatorAdded(uint64,address,bytes,uint256)")
	hash := crypto.Keccak256Hash(eventSignature)

	query := ethereum.FilterQuery{
		Addresses: []common.Address{
			contractAddress,
		},
		Topics: [][]common.Hash{{
			hash,
		}},
	}

	logs := make(chan types.Log)

	sub, err := e.client.SubscribeFilterLogs(e.ctx, query, logs)
	if err != nil {
		return errors.Wrap(err, "Failed to subscribe to logs")
	}

	go func() {
		if err := e.listenToSubscription(sub, logs, contractAbi, abiParser); err != nil {
			e.reconnect(5, 10*time.Second)
		}
	}()

	return nil
}

func (e *EthEvents) listenToSubscription(sub ethereum.Subscription, logs chan types.Log, contractAbi abi.ABI, abiParser eth1.AbiParser) error {
	for {
		select {
		case err := <-sub.Err():
			e.logger.Warn("failed to read logs from subscription", zap.Error(err))
			return err
		case vLog := <-logs:
			err := e.handeNewEvent(vLog, contractAbi, abiParser)
			if err != nil {
				e.logger.Error("failed to handle new event", zap.Error(err))
			}
		}
	}
}

func (e *EthEvents) fetchEvents(fromBlock *big.Int, toBlock *big.Int, contractAbi abi.ABI, abiParser eth1.AbiParser) error {
	contractAddress := common.HexToAddress(e.config.ContractAddress)

	eventSignature := []byte("OperatorAdded(uint64,address,bytes,uint256)")
	hash := crypto.Keccak256Hash(eventSignature)

	query := ethereum.FilterQuery{
		Addresses: []common.Address{
			contractAddress,
		},
		FromBlock: fromBlock,
		ToBlock:   toBlock,

		Topics: [][]common.Hash{{
			hash,
		}},
	}

	logs, err := e.client.FilterLogs(e.ctx, query)
	if err != nil {
		e.logger.Error("failed to subscribe to logs", zap.Error(err))
		return err
	}

	for _, vLog := range logs {
		err = e.handeNewEvent(vLog, contractAbi, abiParser)
		if err != nil {
			e.logger.Error("failed to handle new event", zap.Error(err))
		}
	}

	return nil
}

func (e *EthEvents) handeNewEvent(log types.Log, contractAbi abi.ABI, abiParser eth1.AbiParser) error {
	if log.Removed {
		return nil
	}
	parsed, err := abiParser.ParseOperatorAddedEvent(log, contractAbi)
	if err != nil {
		e.logger.Warn("could not parse ongoing event, the event is malformed",
			fields.BlockNumber(log.BlockNumber),
			fields.TxHash(log.TxHash),
			zap.Error(err),
		)
		return nil
	}
	e.logger.Info("received operator added event", zap.Any("event", parsed))
	err = e.saveOperator(parsed)
	if err != nil {
		e.logger.Warn("could not save operator", zap.Error(err))
	}

	return nil
}

func (e *EthEvents) saveOperator(event *abiparser.OperatorAddedEvent) error {
	return e.db.SaveOperatorAndUpdateNodeData(&db.Operator{
		OperatorID:         format.OperatorID(event.PublicKey),
		PublicKey:          string(event.PublicKey),
		OperatorIDContract: event.OperatorId,
	})
}
