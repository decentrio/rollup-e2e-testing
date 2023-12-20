package hub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	tmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	libclient "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/types"
	authTx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	paramsutils "github.com/cosmos/cosmos-sdk/x/params/client/utils"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/decentrio/rollup-e2e-testing/blockdb"
	"github.com/decentrio/rollup-e2e-testing/dockerutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

// HubNode represents a node in the test network that is being created
type HubNode struct {
	VolumeName   string
	Index        int
	Chain        ibc.Chain
	Validator    bool
	NetworkID    string
	DockerClient *dockerclient.Client
	Client       rpcclient.Client
	TestName     string
	Image        ibc.DockerImage

	lock sync.Mutex
	log  *zap.Logger

	containerLifecycle *dockerutil.ContainerLifecycle

	// Ports set during StartContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
}

func NewHubNode(log *zap.Logger, validator bool, chain *CosmosChain, dockerClient *dockerclient.Client, networkID string, testName string, image ibc.DockerImage, index int) *HubNode {
	hn := &HubNode{
		log: log,

		Validator: validator,

		Chain:        chain,
		DockerClient: dockerClient,
		NetworkID:    networkID,
		TestName:     testName,
		Image:        image,
		Index:        index,
	}

	hn.containerLifecycle = dockerutil.NewContainerLifecycle(log, dockerClient, hn.Name())

	return hn
}

// HubNodes is a collection of HubNode
type HubNodes []*HubNode

const (
	valKey      = "validator"
	blockTime   = 2 // seconds
	p2pPort     = "26656/tcp"
	rpcPort     = "26657/tcp"
	grpcPort    = "9090/tcp"
	apiPort     = "1317/tcp"
	privValPort = "1234/tcp"
)

var (
	sentryPorts = nat.PortSet{
		nat.Port(p2pPort):     {},
		nat.Port(rpcPort):     {},
		nat.Port(grpcPort):    {},
		nat.Port(apiPort):     {},
		nat.Port(privValPort): {},
	}
)

// NewClient creates and assigns a new Tendermint RPC client to the HubNode
func (hn *HubNode) NewClient(addr string) error {
	httpClient, err := libclient.DefaultHTTPClient(addr)
	if err != nil {
		return err
	}

	httpClient.Timeout = 10 * time.Second
	rpcClient, err := rpchttp.NewWithClient(addr, "/websocket", httpClient)
	if err != nil {
		return err
	}

	hn.Client = rpcClient
	return nil
}

// CliContext creates a new Cosmos SDK client context
func (hn *HubNode) CliContext() client.Context {
	cfg := hn.Chain.Config()
	return client.Context{
		Client:            hn.Client,
		ChainID:           cfg.ChainID,
		InterfaceRegistry: cfg.EncodingConfig.InterfaceRegistry,
		Input:             os.Stdin,
		Output:            os.Stdout,
		OutputFormat:      "json",
		LegacyAmino:       cfg.EncodingConfig.Amino,
		TxConfig:          cfg.EncodingConfig.TxConfig,
	}
}

// Name of the test node container
func (hn *HubNode) Name() string {
	var nodeType string
	if hn.Validator {
		nodeType = "val"
	} else {
		nodeType = "fn"
	}
	return fmt.Sprintf("%s-%s-%d-%s", hn.Chain.Config().ChainID, nodeType, hn.Index, dockerutil.SanitizeContainerName(hn.TestName))
}

func (hn *HubNode) ContainerID() string {
	return hn.containerLifecycle.ContainerID()
}

// hostname of the test node container
func (hn *HubNode) HostName() string {
	return dockerutil.CondenseHostName(hn.Name())
}

func (hn *HubNode) GenesisFileContent(ctx context.Context) ([]byte, error) {
	gen, err := hn.ReadFile(ctx, "config/genesis.json")
	if err != nil {
		return nil, fmt.Errorf("getting genesis.json content: %w", err)
	}

	return gen, nil
}

func (hn *HubNode) OverwriteGenesisFile(ctx context.Context, content []byte) error {
	err := hn.WriteFile(ctx, content, "config/genesis.json")
	if err != nil {
		return fmt.Errorf("overwriting genesis.json: %w", err)
	}

	return nil
}

func (hn *HubNode) copyGentx(ctx context.Context, destVal *HubNode) error {
	nid, err := hn.NodeID(ctx)
	if err != nil {
		return fmt.Errorf("getting node ID: %w", err)
	}

	relPath := fmt.Sprintf("config/gentx/gentx-%s.json", nid)

	gentx, err := hn.ReadFile(ctx, relPath)
	if err != nil {
		return fmt.Errorf("getting gentx content: %w", err)
	}

	err = destVal.WriteFile(ctx, gentx, relPath)
	if err != nil {
		return fmt.Errorf("overwriting gentx: %w", err)
	}

	return nil
}

type PrivValidatorKey struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type PrivValidatorKeyFile struct {
	Address string           `json:"address"`
	PubKey  PrivValidatorKey `json:"pub_key"`
	PrivKey PrivValidatorKey `json:"priv_key"`
}

// Bind returns the home folder bind point for running the node
func (hn *HubNode) Bind() []string {
	return []string{fmt.Sprintf("%s:%s", hn.VolumeName, hn.HomeDir())}
}

func (hn *HubNode) HomeDir() string {
	return path.Join("/var/cosmos-chain", hn.Chain.Config().Name)
}

// SetTestConfig modifies the config to reasonable values for use within interchaintest.
func (hn *HubNode) SetTestConfig(ctx context.Context) error {
	c := make(testutil.Toml)

	// Set Log Level to info
	c["log_level"] = "info"

	p2p := make(testutil.Toml)

	// Allow p2p strangeness
	p2p["allow_duplicate_ip"] = true
	p2p["addr_book_strict"] = false

	c["p2p"] = p2p

	consensus := make(testutil.Toml)

	blockT := (time.Duration(blockTime) * time.Second).String()
	consensus["timeout_commit"] = blockT
	consensus["timeout_propose"] = blockT

	c["consensus"] = consensus

	rpc := make(testutil.Toml)

	// Enable public RPC
	rpc["laddr"] = "tcp://0.0.0.0:26657"
	rpc["allowed_origins"] = []string{"*"}

	c["rpc"] = rpc

	if err := testutil.ModifyTomlConfigFile(
		ctx,
		hn.logger(),
		hn.DockerClient,
		hn.TestName,
		hn.VolumeName,
		"config/config.toml",
		c,
	); err != nil {
		return err
	}

	a := make(testutil.Toml)
	a["minimum-gas-prices"] = hn.Chain.Config().GasPrices

	grpc := make(testutil.Toml)

	// Enable public GRPC
	grpc["address"] = "0.0.0.0:9090"

	a["grpc"] = grpc

	api := make(testutil.Toml)

	// Enable public REST API
	api["enable"] = true
	api["swagger"] = true
	api["address"] = "tcp://0.0.0.0:1317"

	a["api"] = api

	return testutil.ModifyTomlConfigFile(
		ctx,
		hn.logger(),
		hn.DockerClient,
		hn.TestName,
		hn.VolumeName,
		"config/app.toml",
		a,
	)
}

// SetPeers modifies the config persistent_peers for a node
func (hn *HubNode) SetPeers(ctx context.Context, peers string) error {
	c := make(testutil.Toml)
	p2p := make(testutil.Toml)

	// Set peers
	p2p["persistent_peers"] = peers
	c["p2p"] = p2p

	return testutil.ModifyTomlConfigFile(
		ctx,
		hn.logger(),
		hn.DockerClient,
		hn.TestName,
		hn.VolumeName,
		"config/config.toml",
		c,
	)
}

func (hn *HubNode) Height(ctx context.Context) (uint64, error) {
	res, err := hn.Client.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("tendermint rpc client status: %w", err)
	}
	height := res.SyncInfo.LatestBlockHeight
	return uint64(height), nil
}

// FindTxs implements blockdb.BlockSaver.
func (hn *HubNode) FindTxs(ctx context.Context, height uint64) ([]blockdb.Tx, error) {
	h := int64(height)
	var eg errgroup.Group
	var blockRes *coretypes.ResultBlockResults
	var block *coretypes.ResultBlock
	eg.Go(func() (err error) {
		blockRes, err = hn.Client.BlockResults(ctx, &h)
		return err
	})
	eg.Go(func() (err error) {
		block, err = hn.Client.Block(ctx, &h)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	interfaceRegistry := hn.Chain.Config().EncodingConfig.InterfaceRegistry
	txs := make([]blockdb.Tx, 0, len(block.Block.Txs)+2)
	for i, tx := range block.Block.Txs {
		var newTx blockdb.Tx
		newTx.Data = []byte(fmt.Sprintf(`{"data":"%s"}`, hex.EncodeToString(tx)))

		sdkTx, err := decodeTX(interfaceRegistry, tx)
		if err != nil {
			hn.logger().Info("Failed to decode tx", zap.Uint64("height", height), zap.Error(err))
			continue
		}
		b, err := encodeTxToJSON(interfaceRegistry, sdkTx)
		if err != nil {
			hn.logger().Info("Failed to marshal tx to json", zap.Uint64("height", height), zap.Error(err))
			continue
		}
		newTx.Data = b

		rTx := blockRes.TxsResults[i]

		newTx.Events = make([]blockdb.Event, len(rTx.Events))
		for j, e := range rTx.Events {
			attrs := make([]blockdb.EventAttribute, len(e.Attributes))
			for k, attr := range e.Attributes {
				attrs[k] = blockdb.EventAttribute{
					Key:   string(attr.Key),
					Value: string(attr.Value),
				}
			}
			newTx.Events[j] = blockdb.Event{
				Type:       e.Type,
				Attributes: attrs,
			}
		}
		txs = append(txs, newTx)
	}
	if len(blockRes.FinalizeBlockEvents) > 0 {
		finalizeBlockTx := blockdb.Tx{
			Data: []byte(`{"data":"finalize_block","note":"this is a transaction artificially created for debugging purposes"}`),
		}
		finalizeBlockTx.Events = make([]blockdb.Event, len(blockRes.FinalizeBlockEvents))
		for i, e := range blockRes.FinalizeBlockEvents {
			attrs := make([]blockdb.EventAttribute, len(e.Attributes))
			for j, attr := range e.Attributes {
				attrs[j] = blockdb.EventAttribute{
					Key:   string(attr.Key),
					Value: string(attr.Value),
				}
			}
			finalizeBlockTx.Events[i] = blockdb.Event{
				Type:       e.Type,
				Attributes: attrs,
			}
		}
		txs = append(txs, finalizeBlockTx)
	}
	return txs, nil
}

// TxCommand is a helper to retrieve a full command for broadcasting a tx
// with the chain node binary.
func (hn *HubNode) TxCommand(keyName string, command ...string) []string {
	command = append([]string{"tx"}, command...)
	var gasPriceFound, gasAdjustmentFound, feesFound = false, false, false
	for i := 0; i < len(command); i++ {
		if command[i] == "--gas-prices" {
			gasPriceFound = true
		}
		if command[i] == "--gas-adjustment" {
			gasAdjustmentFound = true
		}
		if command[i] == "--fees" {
			feesFound = true
		}
	}
	if !gasPriceFound && !feesFound {
		command = append(command, "--gas-prices", hn.Chain.Config().GasPrices)
	}
	if !gasAdjustmentFound {
		command = append(command, "--gas-adjustment", fmt.Sprint(hn.Chain.Config().GasAdjustment))
	}
	return hn.NodeCommand(append(command,
		"--from", keyName,
		"--keyring-backend", keyring.BackendTest,
		"--output", "json",
		"-y",
		"--chain-id", hn.Chain.Config().ChainID,
	)...)
}

// ExecTx executes a transaction, waits for 2 blocks if successful, then returns the tx hash.
func (hn *HubNode) ExecTx(ctx context.Context, keyName string, command ...string) (string, error) {
	hn.lock.Lock()
	defer hn.lock.Unlock()

	stdout, _, err := hn.Exec(ctx, hn.TxCommand(keyName, command...), nil)
	if err != nil {
		return "", err
	}
	output := CosmosTx{}
	err = json.Unmarshal([]byte(stdout), &output)
	if err != nil {
		return "", err
	}
	if output.Code != 0 {
		return output.TxHash, fmt.Errorf("transaction failed with code %d: %s", output.Code, output.RawLog)
	}
	if err := testutil.WaitForBlocks(ctx, 2, hn); err != nil {
		return "", err
	}
	return output.TxHash, nil
}

// NodeCommand is a helper to retrieve a full command for a chain node binary.
// when interactions with the RPC endpoint are necessary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to return the full command.
// Will include additional flags for node URL, home directory, and chain ID.
func (hn *HubNode) NodeCommand(command ...string) []string {
	command = hn.BinCommand(command...)
	return append(command,
		"--node", fmt.Sprintf("tcp://%s:26657", hn.HostName()),
	)
}

// BinCommand is a helper to retrieve a full command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to return the full command.
// Will include additional flags for home directory and chain ID.
func (hn *HubNode) BinCommand(command ...string) []string {
	command = append([]string{hn.Chain.Config().Bin}, command...)
	return append(command,
		"--home", hn.HomeDir(),
	)
}

// ExecBin is a helper to execute a command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to execute the command against the node.
// Will include additional flags for home directory and chain ID.
func (hn *HubNode) ExecBin(ctx context.Context, command ...string) ([]byte, []byte, error) {
	return hn.Exec(ctx, hn.BinCommand(command...), nil)
}

// QueryCommand is a helper to retrieve the full query command. For example,
// if chain node binary is gaiad, and desired command is `gaiad query gov params`,
// pass ("gov", "params") for command to return the full command with all necessary
// flags to query the specific node.
func (hn *HubNode) QueryCommand(command ...string) []string {
	command = append([]string{"query"}, command...)
	return hn.NodeCommand(append(command,
		"--output", "json",
	)...)
}

// ExecQuery is a helper to execute a query command. For example,
// if chain node binary is gaiad, and desired command is `gaiad query gov params`,
// pass ("gov", "params") for command to execute the query against the node.
// Returns response in json format.
func (hn *HubNode) ExecQuery(ctx context.Context, command ...string) ([]byte, []byte, error) {
	return hn.Exec(ctx, hn.QueryCommand(command...), nil)
}

// CondenseMoniker fits a moniker into the cosmos character limit for monikers.
// If the moniker already fits, it is returned unmodified.
// Otherwise, the middle is truncated, and a hash is appended to the end
// in case the only unique data was in the middle.
func CondenseMoniker(m string) string {
	if len(m) <= stakingtypes.MaxMonikerLength {
		return m
	}

	// Get the hash suffix, a 32-bit uint formatted in base36.
	// fnv32 was chosen because a 32-bit number ought to be sufficient
	// as a distinguishing suffix, and it will be short enough so that
	// less of the middle will be truncated to fit in the character limit.
	// It's also non-cryptographic, not that this function will ever be a bottleneck in tests.
	h := fnv.New32()
	h.Write([]byte(m))
	suffix := "-" + strconv.FormatUint(uint64(h.Sum32()), 36)

	wantLen := stakingtypes.MaxMonikerLength - len(suffix)

	// Half of the want length, minus 2 to account for half of the ... we add in the middle.
	keepLen := (wantLen / 2) - 2

	return m[:keepLen] + "..." + m[len(m)-keepLen:] + suffix
}

// InitHomeFolder initializes a home folder for the given node
func (hn *HubNode) InitHomeFolder(ctx context.Context) error {
	hn.lock.Lock()
	defer hn.lock.Unlock()

	_, _, err := hn.ExecBin(ctx,
		"init", CondenseMoniker(hn.Name()),
		"--chain-id", hn.Chain.Config().ChainID,
	)
	return err
}

// WriteFile accepts file contents in a byte slice and writes the contents to
// the docker filesystem. relPath describes the location of the file in the
// docker volume relative to the home directory
func (hn *HubNode) WriteFile(ctx context.Context, content []byte, relPath string) error {
	fw := dockerutil.NewFileWriter(hn.logger(), hn.DockerClient, hn.TestName)
	return fw.WriteFile(ctx, hn.VolumeName, relPath, content)
}

// CopyFile adds a file from the host filesystem to the docker filesystem
// relPath describes the location of the file in the docker volume relative to
// the home directory
func (hn *HubNode) CopyFile(ctx context.Context, srcPath, dstPath string) error {
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return hn.WriteFile(ctx, content, dstPath)
}

// ReadFile reads the contents of a single file at the specified path in the docker filesystem.
// relPath describes the location of the file in the docker volume relative to the home directory.
func (hn *HubNode) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	fr := dockerutil.NewFileRetriever(hn.logger(), hn.DockerClient, hn.TestName)
	gen, err := fr.SingleFileContent(ctx, hn.VolumeName, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file at %s: %w", relPath, err)
	}
	return gen, nil
}

// CreateKey creates a key in the keyring backend test for the given node
func (hn *HubNode) CreateKey(ctx context.Context, name string) error {
	hn.lock.Lock()
	defer hn.lock.Unlock()

	_, _, err := hn.ExecBin(ctx,
		"keys", "add", name,
		"--coin-type", hn.Chain.Config().CoinType,
		"--keyring-backend", keyring.BackendTest,
	)
	return err
}

// RecoverKey restores a key from a given mnemonic.
func (hn *HubNode) RecoverKey(ctx context.Context, keyName, mnemonic string) error {
	command := []string{
		"sh",
		"-c",
		fmt.Sprintf(`echo %q | %s keys add %s --recover --keyring-backend %s --coin-type %s --home %s --output json`, mnemonic, hn.Chain.Config().Bin, keyName, keyring.BackendTest, hn.Chain.Config().CoinType, hn.HomeDir()),
	}

	hn.lock.Lock()
	defer hn.lock.Unlock()

	_, _, err := hn.Exec(ctx, command, nil)
	return err
}

func (hn *HubNode) IsAboveSDK47(ctx context.Context) bool {
	// In SDK v47, a new genesis core command was added. This spec has many state breaking features
	// so we use this to switch between new and legacy SDK logic.
	// https://github.com/cosmos/cosmos-sdk/pull/14149
	return hn.HasCommand(ctx, "genesis")
}

// AddGenesisAccount adds a genesis account for each key
func (hn *HubNode) AddGenesisAccount(ctx context.Context, address string, genesisAmount []types.Coin) error {
	amount := ""
	for i, coin := range genesisAmount {
		if i != 0 {
			amount += ","
		}
		amount += fmt.Sprintf("%s%s", coin.Amount.String(), coin.Denom)
	}

	hn.lock.Lock()
	defer hn.lock.Unlock()

	// Adding a genesis account should complete instantly,
	// so use a 1-minute timeout to more quickly detect if Docker has locked up.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	var command []string
	if hn.IsAboveSDK47(ctx) {
		command = append(command, "genesis")
	}

	command = append(command, "add-genesis-account", address, amount)

	if hn.Chain.Config().UsingChainIDFlagCLI {
		command = append(command, "--chain-id", hn.Chain.Config().ChainID)
	}

	_, _, err := hn.ExecBin(ctx, command...)

	return err
}

// Gentx generates the gentx for a given node
func (hn *HubNode) Gentx(ctx context.Context, name string, genesisSelfDelegation types.Coin) error {
	hn.lock.Lock()
	defer hn.lock.Unlock()

	var command []string
	if hn.IsAboveSDK47(ctx) {
		command = append(command, "genesis")
	}

	command = append(command, "gentx", valKey, fmt.Sprintf("%s%s", genesisSelfDelegation.Amount.String(), genesisSelfDelegation.Denom),
		"--keyring-backend", keyring.BackendTest,
		"--chain-id", hn.Chain.Config().ChainID)

	_, _, err := hn.ExecBin(ctx, command...)
	return err
}

// CollectGentxs runs collect gentxs on the node's home folders
func (hn *HubNode) CollectGentxs(ctx context.Context) error {
	command := []string{hn.Chain.Config().Bin}
	if hn.IsAboveSDK47(ctx) {
		command = append(command, "genesis")
	}

	command = append(command, "collect-gentxs", "--home", hn.HomeDir())

	hn.lock.Lock()
	defer hn.lock.Unlock()

	_, _, err := hn.Exec(ctx, command, nil)
	return err
}

type CosmosTx struct {
	TxHash string `json:"txhash"`
	Code   int    `json:"code"`
	RawLog string `json:"raw_log"`
}

func (hn *HubNode) SendIBCTransfer(
	ctx context.Context,
	channelID string,
	keyName string,
	amount ibc.WalletAmount,
	options ibc.TransferOptions,
) (string, error) {
	command := []string{
		"ibc-transfer", "transfer", "transfer", channelID,
		amount.Address, fmt.Sprintf("%s%s", amount.Amount.String(), amount.Denom),
		"--gas", "auto",
	}
	if options.Timeout != nil {
		if options.Timeout.NanoSeconds > 0 {
			command = append(command, "--packet-timeout-timestamp", fmt.Sprint(options.Timeout.NanoSeconds))
		} else if options.Timeout.Height > 0 {
			command = append(command, "--packet-timeout-height", fmt.Sprintf("0-%d", options.Timeout.Height))
		}
	}
	if options.Memo != "" {
		command = append(command, "--memo", options.Memo)
	}
	return hn.ExecTx(ctx, keyName, command...)
}

func (hn *HubNode) SendFunds(ctx context.Context, keyName string, amount ibc.WalletAmount) error {
	_, err := hn.ExecTx(ctx,
		keyName, "bank", "send", keyName,
		amount.Address, fmt.Sprintf("%s%s", amount.Amount.String(), amount.Denom),
	)
	return err
}

type InstantiateContractAttribute struct {
	Value string `json:"value"`
}

type InstantiateContractEvent struct {
	Attributes []InstantiateContractAttribute `json:"attributes"`
}

type InstantiateContractLog struct {
	Events []InstantiateContractEvent `json:"event"`
}

type InstantiateContractResponse struct {
	Logs []InstantiateContractLog `json:"log"`
}

type QueryContractResponse struct {
	Contracts []string `json:"contracts"`
}

type CodeInfo struct {
	CodeID string `json:"code_id"`
}
type CodeInfosResponse struct {
	CodeInfos []CodeInfo `json:"code_infos"`
}

// StoreContract takes a file path to smart contract and stores it on-chain. Returns the contracts code id.
func (hn *HubNode) StoreContract(ctx context.Context, keyName string, fileName string, extraExecTxArgs ...string) (string, error) {
	_, file := filepath.Split(fileName)
	err := hn.CopyFile(ctx, fileName, file)
	if err != nil {
		return "", fmt.Errorf("writing contract file to docker volume: %w", err)
	}

	cmd := []string{"wasm", "store", path.Join(hn.HomeDir(), file), "--gas", "auto"}
	cmd = append(cmd, extraExecTxArgs...)

	if _, err := hn.ExecTx(ctx, keyName, cmd...); err != nil {
		return "", err
	}

	err = testutil.WaitForBlocks(ctx, 5, hn.Chain)
	if err != nil {
		return "", fmt.Errorf("wait for blocks: %w", err)
	}

	stdout, _, err := hn.ExecQuery(ctx, "wasm", "list-code", "--reverse")
	if err != nil {
		return "", err
	}

	res := CodeInfosResponse{}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		return "", err
	}

	return res.CodeInfos[0].CodeID, nil
}

func (hn *HubNode) GetTransaction(clientCtx client.Context, txHash string) (*types.TxResponse, error) {
	// Retry because sometimes the tx is not committed to state yet.
	var txResp *types.TxResponse
	err := retry.Do(func() error {
		var err error
		txResp, err = authTx.QueryTx(clientCtx, txHash)
		return err
	},
		// retry for total of 3 seconds
		retry.Attempts(15),
		retry.Delay(200*time.Millisecond),
		retry.DelayType(retry.FixedDelay),
		retry.LastErrorOnly(true),
	)
	return txResp, err
}

// HasCommand checks if a command in the chain binary is available.
func (hn *HubNode) HasCommand(ctx context.Context, command ...string) bool {
	_, _, err := hn.ExecBin(ctx, command...)
	if err == nil {
		return true
	}

	if strings.Contains(string(err.Error()), "Error: unknown command") {
		return false
	}

	// cmd just needed more arguments, but it is a valid command (ex: appd tx bank send)
	if strings.Contains(string(err.Error()), "Error: accepts") {
		return true
	}

	return false
}

// GetBuildInformation returns the build information and dependencies for the chain binary.
func (hn *HubNode) GetBuildInformation(ctx context.Context) *BinaryBuildInformation {
	stdout, _, err := hn.ExecBin(ctx, "version", "--long", "--output", "json")
	if err != nil {
		return nil
	}

	type tempBuildDeps struct {
		Name             string   `json:"name"`
		ServerName       string   `json:"server_name"`
		Version          string   `json:"version"`
		Commit           string   `json:"commit"`
		BuildTags        string   `json:"build_tags"`
		Go               string   `json:"go"`
		BuildDeps        []string `json:"build_deps"`
		CosmosSdkVersion string   `json:"cosmos_sdk_version"`
	}

	var deps tempBuildDeps
	if err := json.Unmarshal([]byte(stdout), &deps); err != nil {
		return nil
	}

	getRepoAndVersion := func(dep string) (string, string) {
		split := strings.Split(dep, "@")
		return split[0], split[1]
	}

	var buildDeps []BuildDependency
	for _, dep := range deps.BuildDeps {
		var bd BuildDependency

		if strings.Contains(dep, "=>") {
			// Ex: "github.com/aaa/bbb@v1.2.1 => github.com/ccc/bbb@v1.2.0"
			split := strings.Split(dep, " => ")
			main, replacement := split[0], split[1]

			parent, parentVersion := getRepoAndVersion(main)
			r, rV := getRepoAndVersion(replacement)

			bd = BuildDependency{
				Parent:             parent,
				Version:            parentVersion,
				IsReplacement:      true,
				Replacement:        r,
				ReplacementVersion: rV,
			}

		} else {
			// Ex: "github.com/aaa/bbb@v0.0.0-20191008050251-8e49817e8af4"
			parent, version := getRepoAndVersion(dep)

			bd = BuildDependency{
				Parent:             parent,
				Version:            version,
				IsReplacement:      false,
				Replacement:        "",
				ReplacementVersion: "",
			}
		}

		buildDeps = append(buildDeps, bd)
	}

	return &BinaryBuildInformation{
		BuildDeps:        buildDeps,
		Name:             deps.Name,
		ServerName:       deps.ServerName,
		Version:          deps.Version,
		Commit:           deps.Commit,
		BuildTags:        deps.BuildTags,
		Go:               deps.Go,
		CosmosSdkVersion: deps.CosmosSdkVersion,
	}
}

// InstantiateContract takes a code id for a smart contract and initialization message and returns the instantiated contract address.
func (hn *HubNode) InstantiateContract(ctx context.Context, keyName string, codeID string, initMessage string, needsNoAdminFlag bool, extraExecTxArgs ...string) (string, error) {
	command := []string{"wasm", "instantiate", codeID, initMessage, "--label", "wasm-contract"}
	command = append(command, extraExecTxArgs...)
	if needsNoAdminFlag {
		command = append(command, "--no-admin")
	}
	txHash, err := hn.ExecTx(ctx, keyName, command...)
	if err != nil {
		return "", err
	}

	txResp, err := hn.GetTransaction(hn.CliContext(), txHash)
	if err != nil {
		return "", fmt.Errorf("failed to get transaction %s: %w", txHash, err)
	}
	if txResp.Code != 0 {
		return "", fmt.Errorf("error in transaction (code: %d): %s", txResp.Code, txResp.RawLog)
	}

	stdout, _, err := hn.ExecQuery(ctx, "wasm", "list-contract-by-code", codeID)
	if err != nil {
		return "", err
	}

	contactsRes := QueryContractResponse{}
	if err := json.Unmarshal([]byte(stdout), &contactsRes); err != nil {
		return "", err
	}

	contractAddress := contactsRes.Contracts[len(contactsRes.Contracts)-1]
	return contractAddress, nil
}

// ExecuteContract executes a contract transaction with a message using it's address.
func (hn *HubNode) ExecuteContract(ctx context.Context, keyName string, contractAddress string, message string, extraExecTxArgs ...string) (res *types.TxResponse, err error) {
	cmd := []string{"wasm", "execute", contractAddress, message}
	cmd = append(cmd, extraExecTxArgs...)

	txHash, err := hn.ExecTx(ctx, keyName, cmd...)
	if err != nil {
		return &types.TxResponse{}, err
	}

	txResp, err := hn.GetTransaction(hn.CliContext(), txHash)
	if err != nil {
		return &types.TxResponse{}, fmt.Errorf("failed to get transaction %s: %w", txHash, err)
	}

	if txResp.Code != 0 {
		return txResp, fmt.Errorf("error in transaction (code: %d): %s", txResp.Code, txResp.RawLog)
	}

	return txResp, nil
}

// QueryContract performs a smart query, taking in a query struct and returning a error with the response struct populated.
func (hn *HubNode) QueryContract(ctx context.Context, contractAddress string, queryMsg any, response any) error {
	var query []byte
	var err error

	if q, ok := queryMsg.(string); ok {
		var jsonMap map[string]interface{}
		if err := json.Unmarshal([]byte(q), &jsonMap); err != nil {
			return err
		}

		query, err = json.Marshal(jsonMap)
		if err != nil {
			return err
		}
	} else {
		query, err = json.Marshal(queryMsg)
		if err != nil {
			return err
		}
	}

	stdout, _, err := hn.ExecQuery(ctx, "wasm", "contract-state", "smart", contractAddress, string(query))
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(stdout), response)
	return err
}

// StoreClientContract takes a file path to a client smart contract and stores it on-chain. Returns the contracts code id.
func (hn *HubNode) StoreClientContract(ctx context.Context, keyName string, fileName string, extraExecTxArgs ...string) (string, error) {
	content, err := os.ReadFile(fileName)
	if err != nil {
		return "", err
	}
	_, file := filepath.Split(fileName)
	err = hn.WriteFile(ctx, content, file)
	if err != nil {
		return "", fmt.Errorf("writing contract file to docker volume: %w", err)
	}

	cmd := []string{"ibc-wasm", "store-code", path.Join(hn.HomeDir(), file), "--gas", "auto"}
	cmd = append(cmd, extraExecTxArgs...)

	_, err = hn.ExecTx(ctx, keyName, cmd...)
	if err != nil {
		return "", err
	}

	codeHashByte32 := sha256.Sum256(content)
	codeHash := hex.EncodeToString(codeHashByte32[:])

	//return stdout, nil
	return codeHash, nil
}

// QueryClientContractCode performs a query with the contract codeHash as the input and code as the output
func (hn *HubNode) QueryClientContractCode(ctx context.Context, codeHash string, response any) error {
	stdout, _, err := hn.ExecQuery(ctx, "ibc-wasm", "code", codeHash)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(stdout), response)
	return err
}

// GetModuleAddress performs a query to get the address of the specified chain module
func (hn *HubNode) GetModuleAddress(ctx context.Context, moduleName string) (string, error) {
	queryRes, err := hn.GetModuleAccount(ctx, moduleName)
	if err != nil {
		return "", err
	}
	return queryRes.Account.BaseAccount.Address, nil
}

// GetModuleAccount performs a query to get the account details of the specified chain module
func (hn *HubNode) GetModuleAccount(ctx context.Context, moduleName string) (QueryModuleAccountResponse, error) {
	stdout, _, err := hn.ExecQuery(ctx, "auth", "module-account", moduleName)
	if err != nil {
		return QueryModuleAccountResponse{}, err
	}

	queryRes := QueryModuleAccountResponse{}
	err = json.Unmarshal(stdout, &queryRes)
	if err != nil {
		return QueryModuleAccountResponse{}, err
	}
	return queryRes, nil
}

// VoteOnProposal submits a vote for the specified proposal.
func (hn *HubNode) VoteOnProposal(ctx context.Context, keyName string, proposalID string, vote string) error {
	_, err := hn.ExecTx(ctx, keyName,
		"gov", "vote",
		proposalID, vote, "--gas", "auto",
	)
	return err
}

// QueryProposal returns the state and details of a governance proposal.
func (hn *HubNode) QueryProposal(ctx context.Context, proposalID string) (*ProposalResponse, error) {
	stdout, _, err := hn.ExecQuery(ctx, "gov", "proposal", proposalID)
	if err != nil {
		return nil, err
	}
	var proposal ProposalResponse
	err = json.Unmarshal(stdout, &proposal)
	if err != nil {
		return nil, err
	}
	return &proposal, nil
}

// SubmitProposal submits a gov v1 proposal to the chain.
func (hn *HubNode) SubmitProposal(ctx context.Context, keyName string, prop TxProposalv1) (string, error) {
	// Write msg to container
	file := "proposal.json"
	propJson, err := json.MarshalIndent(prop, "", " ")
	if err != nil {
		return "", err
	}
	fw := dockerutil.NewFileWriter(hn.logger(), hn.DockerClient, hn.TestName)
	if err := fw.WriteFile(ctx, hn.VolumeName, file, propJson); err != nil {
		return "", fmt.Errorf("writing contract file to docker volume: %w", err)
	}

	command := []string{
		"gov", "submit-proposal",
		path.Join(hn.HomeDir(), file), "--gas", "auto",
	}

	return hn.ExecTx(ctx, keyName, command...)
}

// UpgradeProposal submits a software-upgrade governance proposal to the chain.
func (hn *HubNode) UpgradeProposal(ctx context.Context, keyName string, prop SoftwareUpgradeProposal) (string, error) {
	command := []string{
		"gov", "submit-proposal",
		"software-upgrade", prop.Name,
		"--upgrade-height", strconv.FormatUint(prop.Height, 10),
		"--title", prop.Title,
		"--description", prop.Description,
		"--deposit", prop.Deposit,
	}

	if prop.Info != "" {
		command = append(command, "--upgrade-info", prop.Info)
	}

	return hn.ExecTx(ctx, keyName, command...)
}

// TextProposal submits a text governance proposal to the chain.
func (hn *HubNode) TextProposal(ctx context.Context, keyName string, prop TextProposal) (string, error) {
	command := []string{
		"gov", "submit-proposal",
		"--type", "text",
		"--title", prop.Title,
		"--description", prop.Description,
		"--deposit", prop.Deposit,
	}
	if prop.Expedited {
		command = append(command, "--is-expedited=true")
	}
	return hn.ExecTx(ctx, keyName, command...)
}

// ParamChangeProposal submits a param change proposal to the chain, signed by keyName.
func (hn *HubNode) ParamChangeProposal(ctx context.Context, keyName string, prop *paramsutils.ParamChangeProposalJSON) (string, error) {
	content, err := json.Marshal(prop)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(content)
	proposalFilename := fmt.Sprintf("%x.json", hash)
	err = hn.WriteFile(ctx, content, proposalFilename)
	if err != nil {
		return "", fmt.Errorf("writing param change proposal: %w", err)
	}

	proposalPath := filepath.Join(hn.HomeDir(), proposalFilename)

	command := []string{
		"gov", "submit-proposal",
		"param-change",
		proposalPath,
	}

	return hn.ExecTx(ctx, keyName, command...)
}

// QueryParam returns the state and details of a subspace param.
func (hn *HubNode) QueryParam(ctx context.Context, subspace, key string) (*ParamChange, error) {
	stdout, _, err := hn.ExecQuery(ctx, "params", "subspace", subspace, key)
	if err != nil {
		return nil, err
	}
	var param ParamChange
	err = json.Unmarshal(stdout, &param)
	if err != nil {
		return nil, err
	}
	return &param, nil
}

// QueryBankMetadata returns the bank metadata of a token denomination.
func (hn *HubNode) QueryBankMetadata(ctx context.Context, denom string) (*BankMetaData, error) {
	stdout, _, err := hn.ExecQuery(ctx, "bank", "denom-metadata", "--denom", denom)
	if err != nil {
		return nil, err
	}
	var meta BankMetaData
	err = json.Unmarshal(stdout, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

// DumpContractState dumps the state of a contract at a block height.
func (hn *HubNode) DumpContractState(ctx context.Context, contractAddress string, height int64) (*DumpContractStateResponse, error) {
	stdout, _, err := hn.ExecQuery(ctx,
		"wasm", "contract-state", "all", contractAddress,
		"--height", fmt.Sprint(height),
	)
	if err != nil {
		return nil, err
	}

	res := new(DumpContractStateResponse)
	if err := json.Unmarshal([]byte(stdout), res); err != nil {
		return nil, err
	}
	return res, nil
}

func (hn *HubNode) ExportState(ctx context.Context, height int64) (string, error) {
	hn.lock.Lock()
	defer hn.lock.Unlock()

	var (
		doc              = "state_export.json"
		docPath          = path.Join(hn.HomeDir(), doc)
		isNewerThanSdk47 = hn.IsAboveSDK47(ctx)
		command          = []string{"export", "--height", fmt.Sprint(height), "--home", hn.HomeDir()}
	)

	if isNewerThanSdk47 {
		command = append(command, "--output-document", docPath)
	}

	stdout, stderr, err := hn.ExecBin(ctx, command...)
	if err != nil {
		return "", err
	}

	if isNewerThanSdk47 {
		content, err := hn.ReadFile(ctx, doc)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}

	// output comes to stderr on older versions
	return string(stdout) + string(stderr), nil
}

func (hn *HubNode) UnsafeResetAll(ctx context.Context) error {
	hn.lock.Lock()
	defer hn.lock.Unlock()

	command := []string{hn.Chain.Config().Bin}
	if hn.IsAboveSDK47(ctx) {
		command = append(command, "comet")
	}

	command = append(command, "unsafe-reset-all", "--home", hn.HomeDir())

	_, _, err := hn.Exec(ctx, command, nil)
	return err
}

func (hn *HubNode) CreateNodeContainer(ctx context.Context) error {
	chainCfg := hn.Chain.Config()

	var cmd []string
	if chainCfg.NoHostMount {
		cmd = []string{"sh", "-c", fmt.Sprintf("cp -r %s %s_nomnt && %s start --home %s_nomnt --x-crisis-skip-assert-invariants", hn.HomeDir(), hn.HomeDir(), chainCfg.Bin, hn.HomeDir())}
	} else {
		cmd = []string{chainCfg.Bin, "start", "--home", hn.HomeDir(), "--x-crisis-skip-assert-invariants"}
	}

	return hn.containerLifecycle.CreateContainer(ctx, hn.TestName, hn.NetworkID, hn.Image, sentryPorts, hn.Bind(), hn.HostName(), cmd, nil)
}

func (hn *HubNode) StartContainer(ctx context.Context) error {
	if err := hn.containerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := hn.containerLifecycle.GetHostPorts(ctx, rpcPort, grpcPort, apiPort)
	if err != nil {
		return err
	}
	hn.hostRPCPort, hn.hostGRPCPort, hn.hostAPIPort = hostPorts[0], hostPorts[1], hostPorts[2]

	err = hn.NewClient("tcp://" + hn.hostRPCPort)
	if err != nil {
		return err
	}

	time.Sleep(5 * time.Second)
	return retry.Do(func() error {
		stat, err := hn.Client.Status(ctx)
		if err != nil {
			return err
		}
		// TODO: reenable this check, having trouble with it for some reason
		if stat != nil && stat.SyncInfo.CatchingUp {
			return fmt.Errorf("still catching up: height(%d) catching-up(%t)",
				stat.SyncInfo.LatestBlockHeight, stat.SyncInfo.CatchingUp)
		}
		return nil
	}, retry.Context(ctx), retry.Attempts(40), retry.Delay(3*time.Second), retry.DelayType(retry.FixedDelay))
}

func (hn *HubNode) PauseContainer(ctx context.Context) error {
	return hn.containerLifecycle.PauseContainer(ctx)
}

func (hn *HubNode) UnpauseContainer(ctx context.Context) error {
	return hn.containerLifecycle.UnpauseContainer(ctx)
}

func (hn *HubNode) StopContainer(ctx context.Context) error {
	return hn.containerLifecycle.StopContainer(ctx)
}

func (hn *HubNode) RemoveContainer(ctx context.Context) error {
	return hn.containerLifecycle.RemoveContainer(ctx)
}

// InitValidatorFiles creates the node files and signs a genesis transaction
func (hn *HubNode) InitValidatorGenTx(
	ctx context.Context,
	chainType *ibc.ChainConfig,
	genesisAmounts []types.Coin,
	genesisSelfDelegation types.Coin,
) error {
	if err := hn.CreateKey(ctx, valKey); err != nil {
		return err
	}
	bech32, err := hn.AccountKeyBech32(ctx, valKey)
	if err != nil {
		return err
	}
	if err := hn.AddGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
		return err
	}
	return hn.Gentx(ctx, valKey, genesisSelfDelegation)
}

func (hn *HubNode) InitFullNodeFiles(ctx context.Context) error {
	if err := hn.InitHomeFolder(ctx); err != nil {
		return err
	}

	return hn.SetTestConfig(ctx)
}

// NodeID returns the persistent ID of a given node.
func (hn *HubNode) NodeID(ctx context.Context) (string, error) {
	// This used to call p2p.LoadNodeKey against the file on the host,
	// but because we are transitioning to operating on Docker volumes,
	// we only have to tmjson.Unmarshal the raw content.
	j, err := hn.ReadFile(ctx, "config/node_key.json")
	if err != nil {
		return "", fmt.Errorf("getting node_key.json content: %w", err)
	}

	var nk p2p.NodeKey
	if err := tmjson.Unmarshal(j, &nk); err != nil {
		return "", fmt.Errorf("unmarshaling node_key.json: %w", err)
	}

	return string(nk.ID()), nil
}

// KeyBech32 retrieves the named key's address in bech32 format from the node.
// bech is the bech32 prefix (acc|val|cons). If empty, defaults to the account key (same as "acc").
func (hn *HubNode) KeyBech32(ctx context.Context, name string, bech string) (string, error) {
	command := []string{hn.Chain.Config().Bin, "keys", "show", "--address", name,
		"--home", hn.HomeDir(),
		"--keyring-backend", keyring.BackendTest,
	}

	if bech != "" {
		command = append(command, "--bech", bech)
	}

	stdout, stderr, err := hn.Exec(ctx, command, nil)
	if err != nil {
		return "", fmt.Errorf("failed to show key %q (stderr=%q): %w", name, stderr, err)
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
}

// AccountKeyBech32 retrieves the named key's address in bech32 account format.
func (hn *HubNode) AccountKeyBech32(ctx context.Context, name string) (string, error) {
	return hn.KeyBech32(ctx, name, "")
}

// PeerString returns the string for connecting the nodes passed in
func (nodes HubNodes) PeerString(ctx context.Context) string {
	addrs := make([]string, len(nodes))
	for i, n := range nodes {
		id, err := n.NodeID(ctx)
		if err != nil {
			// TODO: would this be better to panic?
			// When would NodeId return an error?
			break
		}
		hostName := n.HostName()
		ps := fmt.Sprintf("%s@%s:26656", id, hostName)
		nodes.logger().Info("Peering",
			zap.String("host_name", hostName),
			zap.String("peer", ps),
			zap.String("container", n.Name()),
		)
		addrs[i] = ps
	}
	return strings.Join(addrs, ",")
}

// LogGenesisHashes logs the genesis hashes for the various nodes
func (nodes HubNodes) LogGenesisHashes(ctx context.Context) error {
	for _, n := range nodes {
		gen, err := n.GenesisFileContent(ctx)
		if err != nil {
			return err
		}

		n.logger().Info("Genesis", zap.String("hash", fmt.Sprintf("%X", sha256.Sum256(gen))))
	}
	return nil
}

func (nodes HubNodes) logger() *zap.Logger {
	if len(nodes) == 0 {
		return zap.NewNop()
	}
	return nodes[0].logger()
}

func (hn *HubNode) Exec(ctx context.Context, cmd []string, env []string) ([]byte, []byte, error) {
	job := dockerutil.NewImage(hn.logger(), hn.DockerClient, hn.NetworkID, hn.TestName, hn.Image.Repository, hn.Image.Version)
	opts := dockerutil.ContainerOptions{
		Env:   env,
		Binds: hn.Bind(),
	}
	res := job.Run(ctx, cmd, opts)
	return res.Stdout, res.Stderr, res.Err
}

func (hn *HubNode) logger() *zap.Logger {
	return hn.log.With(
		zap.String("chain_id", hn.Chain.Config().ChainID),
		zap.String("test", hn.TestName),
	)
}

// RegisterICA will attempt to register an interchain account on the counterparty chain.
func (hn *HubNode) RegisterICA(ctx context.Context, keyName, connectionID string) (string, error) {
	return hn.ExecTx(ctx, keyName,
		"intertx", "register",
		"--connection-id", connectionID,
	)
}

// QueryICA will query for an interchain account controlled by the specified address on the counterparty chain.
func (hn *HubNode) QueryICA(ctx context.Context, connectionID, address string) (string, error) {
	stdout, _, err := hn.ExecQuery(ctx,
		"intertx", "interchainaccounts", connectionID, address,
	)
	if err != nil {
		return "", err
	}

	// at this point stdout should look like this:
	// interchain_account_address: cosmos1p76n3mnanllea4d3av0v0e42tjj03cae06xq8fwn9at587rqp23qvxsv0j
	// we split the string at the : and then just grab the address before returning.
	parts := strings.SplitN(string(stdout), ":", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("malformed stdout from command: %s", stdout)
	}
	return strings.TrimSpace(parts[1]), nil
}

// SendICABankTransfer builds a bank transfer message for a specified address and sends it to the specified
// interchain account.
func (hn *HubNode) SendICABankTransfer(ctx context.Context, connectionID, fromAddr string, amount ibc.WalletAmount) error {
	msg, err := json.Marshal(map[string]any{
		"@type":        "/cosmos.bank.v1beta1.MsgSend",
		"from_address": fromAddr,
		"to_address":   amount.Address,
		"amount": []map[string]any{
			{
				"denom":  amount.Denom,
				"amount": amount.Amount.String(),
			},
		},
	})
	if err != nil {
		return err
	}

	_, err = hn.ExecTx(ctx, fromAddr,
		"intertx", "submit", string(msg),
		"--connection-id", connectionID,
	)
	return err
}
