package cosmos

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
	"github.com/decentrio/rollup-e2e-testing/blockdb"
	"github.com/decentrio/rollup-e2e-testing/dockerutil"
	"github.com/decentrio/rollup-e2e-testing/dymension"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	volumetypes "github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"golang.org/x/exp/rand"
	"golang.org/x/sync/errgroup"
)

// Node represents a node in the test network that is being created
type Node struct {
	VolumeName   string
	Index        int
	Chain        ibc.Chain
	Validator    bool
	NetworkID    string
	DockerClient *dockerclient.Client
	Client       rpcclient.Client
	TestName     string
	Image        ibc.DockerImage
	Sidecars     SidecarProcesses

	lock sync.Mutex
	log  *zap.Logger

	containerLifecycle *dockerutil.ContainerLifecycle

	// Ports set during StartContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
}

func (node *Node) NewSidecarProcess(
	ctx context.Context,
	preStart bool,
	processName string,
	cli *dockerclient.Client,
	networkID string,
	image ibc.DockerImage,
	homeDir string,
	ports []string,
	startCmd []string,
) error {
	s := NewSidecar(node.log, true, preStart, node.Chain, cli, networkID, processName, node.TestName, image, homeDir, node.Index, ports, startCmd)
	v, err := cli.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{
			dockerutil.CleanupLabel:   node.TestName,
			dockerutil.NodeOwnerLabel: s.Name(),
		},
	})
	if err != nil {
		return fmt.Errorf("creating volume for sidecar process: %w", err)
	}
	s.VolumeName = v.Name
	if err := dockerutil.SetVolumeOwner(ctx, dockerutil.VolumeOwnerOptions{
		Log:        node.log,
		Client:     cli,
		VolumeName: v.Name,
		ImageRef:   image.Ref(),
		TestName:   node.TestName,
		UidGid:     image.UidGid,
	}); err != nil {
		return fmt.Errorf("set volume owner: %w", err)
	}
	node.Sidecars = append(node.Sidecars, s)
	return nil
}

func NewNode(log *zap.Logger, validator bool, chain *CosmosChain, dockerClient *dockerclient.Client, networkID string, testName string, image ibc.DockerImage, index int) *Node {
	node := &Node{
		log: log,

		Validator: validator,

		Chain:        chain,
		DockerClient: dockerClient,
		NetworkID:    networkID,
		TestName:     testName,
		Image:        image,
		Index:        index,
	}

	node.containerLifecycle = dockerutil.NewContainerLifecycle(log, dockerClient, node.Name())

	return node
}

// Nodes is a collection of Node
type Nodes []*Node

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

// NewClient creates and assigns a new Tendermint RPC client to the Node
func (node *Node) NewClient(addr string) error {
	httpClient, err := libclient.DefaultHTTPClient(addr)
	if err != nil {
		return err
	}

	httpClient.Timeout = 10 * time.Second
	rpcClient, err := rpchttp.NewWithClient(addr, "/websocket", httpClient)
	if err != nil {
		return err
	}

	node.Client = rpcClient
	return nil
}

// CliContext creates a new Cosmos SDK client context
func (node *Node) CliContext() client.Context {
	cfg := node.Chain.Config()
	return client.Context{
		Client:            node.Client,
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
func (node *Node) Name() string {
	var nodeType string
	if node.Validator {
		nodeType = "val"
	} else {
		nodeType = "fn"
	}

	if node.Chain.Config().Type == "rollapp-dym" {
		return fmt.Sprintf("ra-%s-%s-%d-%s", node.Chain.Config().ChainID, nodeType, node.Index, dockerutil.SanitizeContainerName(node.TestName))
	}

	return fmt.Sprintf("%s-%s-%d-%s", node.Chain.Config().ChainID, nodeType, node.Index, dockerutil.SanitizeContainerName(node.TestName))
}

func (node *Node) ContainerID() string {
	return node.containerLifecycle.ContainerID()
}

// hostname of the test node container
func (node *Node) HostName() string {
	return dockerutil.CondenseHostName(node.Name())
}

func (node *Node) GenesisFileContent(ctx context.Context) ([]byte, error) {
	gen, err := node.ReadFile(ctx, "config/genesis.json")
	if err != nil {
		return nil, fmt.Errorf("getting genesis.json content: %w", err)
	}

	return gen, nil
}

func (node *Node) OverwriteGenesisFile(ctx context.Context, content []byte) error {
	err := node.WriteFile(ctx, content, "config/genesis.json")
	if err != nil {
		return fmt.Errorf("overwriting genesis.json: %w", err)
	}

	return nil
}

func (node *Node) ExtractPrivateValKeyFile(ctx context.Context) (PrivValidatorKeyFile, error) {
	contents, err := node.ReadFile(ctx, "config/priv_validator_key.json")
	if err != nil {
		return PrivValidatorKeyFile{}, fmt.Errorf("fail to getting priv_validator_key.json content: %w", err)
	}

	var privValidatorKeyFile PrivValidatorKeyFile
	err = json.Unmarshal(contents, &privValidatorKeyFile)
	if err != nil {
		return PrivValidatorKeyFile{}, err
	}

	return privValidatorKeyFile, nil
}

func (node *Node) CopyGentx(ctx context.Context, destVal *Node) error {
	return node.copyGentx(ctx, destVal)
}

func (node *Node) copyGentx(ctx context.Context, destVal *Node) error {
	nid, err := node.NodeID(ctx)
	if err != nil {
		return fmt.Errorf("getting node ID: %w", err)
	}

	relPath := fmt.Sprintf("config/gentx/gentx-%s.json", nid)

	gentx, err := node.ReadFile(ctx, relPath)
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
func (node *Node) Bind() []string {
	return []string{fmt.Sprintf("%s:%s", "/tmp", "/var/cosmos-chain"), fmt.Sprintf("%s:%s", "/tmp/celestia", "/home/celestia")}
}

func (node *Node) HomeDir() string {
	return path.Join("/var/cosmos-chain", node.Chain.Config().Name+node.VolumeName)
}

// SetTestConfig modifies the config to reasonable values for use within e2e-test.
func (node *Node) SetTestConfig(ctx context.Context) error {
	c := make(testutil.Toml)

	// Set Log Level to info
	c["log_level"] = "debug"

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
		node.logger(),
		node.DockerClient,
		node.TestName,
		node.VolumeName,
		node.Chain.Config().Name,
		"config/config.toml",
		c,
	); err != nil {
		return err
	}

	a := make(testutil.Toml)
	a["minimum-gas-prices"] = node.Chain.Config().GasPrices

	grpc := make(testutil.Toml)

	// Enable public GRPC
	grpc["address"] = "0.0.0.0:9090"
	grpc["enable"] = true
	a["grpc"] = grpc

	api := make(testutil.Toml)

	// Enable public REST API
	api["enable"] = true
	api["swagger"] = true
	api["address"] = "tcp://0.0.0.0:1317"

	a["api"] = api

	return testutil.ModifyTomlConfigFile(
		ctx,
		node.logger(),
		node.DockerClient,
		node.TestName,
		node.VolumeName,
		node.Chain.Config().Name,
		"config/app.toml",
		a,
	)
}

// SetPeers modifies the config persistent_peers for a node
func (node *Node) SetPeers(ctx context.Context, peers string) error {
	c := make(testutil.Toml)
	p2p := make(testutil.Toml)

	// Set peers
	p2p["persistent_peers"] = peers
	c["p2p"] = p2p

	return testutil.ModifyTomlConfigFile(
		ctx,
		node.logger(),
		node.DockerClient,
		node.TestName,
		node.VolumeName,
		node.Chain.Config().Name,
		"config/config.toml",
		c,
	)
}

func (node *Node) Height(ctx context.Context) (int64, error) {
	res, err := node.Client.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("tendermint rpc client status: %w", err)
	}
	height := res.SyncInfo.LatestBlockHeight
	return int64(height), nil
}

// FindTxs implements blockdb.BlockSaver.
func (node *Node) FindTxs(ctx context.Context, height int64) ([]blockdb.Tx, error) {
	h := int64(height)
	var eg errgroup.Group
	var blockRes *coretypes.ResultBlockResults
	var block *coretypes.ResultBlock
	eg.Go(func() (err error) {
		blockRes, err = node.Client.BlockResults(ctx, &h)
		return err
	})
	eg.Go(func() (err error) {
		block, err = node.Client.Block(ctx, &h)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	interfaceRegistry := node.Chain.Config().EncodingConfig.InterfaceRegistry
	txs := make([]blockdb.Tx, 0, len(block.Block.Txs)+2)
	for i, tx := range block.Block.Txs {
		var newTx blockdb.Tx
		newTx.Data = []byte(fmt.Sprintf(`{"data":"%s"}`, hex.EncodeToString(tx)))

		sdkTx, err := decodeTX(interfaceRegistry, tx)
		if err != nil {
			node.logger().Info("Failed to decode tx", zap.Int64("height", height), zap.Error(err))
			continue
		}
		b, err := encodeTxToJSON(interfaceRegistry, sdkTx)
		if err != nil {
			node.logger().Info("Failed to marshal tx to json", zap.Int64("height", height), zap.Error(err))
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
	if len(blockRes.BeginBlockEvents) > 0 {
		beginBlockTx := blockdb.Tx{
			Data: []byte(`{"data":"begin_block","note":"this is a transaction artificially created for debugging purposes"}`),
		}
		beginBlockTx.Events = make([]blockdb.Event, len(blockRes.BeginBlockEvents))
		for i, e := range blockRes.BeginBlockEvents {
			attrs := make([]blockdb.EventAttribute, len(e.Attributes))
			for j, attr := range e.Attributes {
				attrs[j] = blockdb.EventAttribute{
					Key:   string(attr.Key),
					Value: string(attr.Value),
				}
			}
			beginBlockTx.Events[i] = blockdb.Event{
				Type:       e.Type,
				Attributes: attrs,
			}
		}
		txs = append(txs, beginBlockTx)
	}
	if len(blockRes.EndBlockEvents) > 0 {
		endBlockTx := blockdb.Tx{
			Data: []byte(`{"data":"end_block","note":"this is a transaction artificially created for debugging purposes"}`),
		}
		endBlockTx.Events = make([]blockdb.Event, len(blockRes.EndBlockEvents))
		for i, e := range blockRes.EndBlockEvents {
			attrs := make([]blockdb.EventAttribute, len(e.Attributes))
			for j, attr := range e.Attributes {
				attrs[j] = blockdb.EventAttribute{
					Key:   string(attr.Key),
					Value: string(attr.Value),
				}
			}
			endBlockTx.Events[i] = blockdb.Event{
				Type:       e.Type,
				Attributes: attrs,
			}
		}
		txs = append(txs, endBlockTx)
	}

	return txs, nil
}

// TxCommand is a helper to retrieve a full command for broadcasting a tx
// with the chain node binary.
func (node *Node) TxCommand(keyName string, command ...string) []string {
	command = append([]string{"tx"}, command...)
	var gasPriceFound, gasAdjustmentFound = false, false
	for i := 0; i < len(command); i++ {
		if command[i] == "--gas-prices" {
			gasPriceFound = true
		}
		if command[i] == "--gas-adjustment" {
			gasAdjustmentFound = true
		}
	}
	if !gasPriceFound {
		command = append(command, "--gas-prices", node.Chain.Config().GasPrices)
	}
	if !gasAdjustmentFound {
		command = append(command, "--gas-adjustment", fmt.Sprint(node.Chain.Config().GasAdjustment))
	}
	return node.NodeCommand(append(command,
		"--from", keyName,
		"--keyring-backend", keyring.BackendTest,
		"--output", "json",
		"-y",
	)...)
}

// ExecTx executes a transaction, waits for 2 blocks if successful, then returns the tx hash.
func (node *Node) ExecTx(ctx context.Context, keyName string, command ...string) (string, error) {
	node.lock.Lock()
	defer node.lock.Unlock()

	stdout, _, err := node.Exec(ctx, node.TxCommand(keyName, command...), nil)
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
	if node.Chain.Config().Type == "rollapp-dym" {
		return output.TxHash, nil
	}

	if err := testutil.WaitForBlocks(ctx, 5, node); err != nil {
		return "", err
	}
	return output.TxHash, nil
}

// NodeCommand is a helper to retrieve a full command for a chain node binary.
// when interactions with the RPC endpoint are necessary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to return the full command.
// Will include additional flags for node URL, home directory, and chain ID.
func (node *Node) NodeCommand(command ...string) []string {
	command = node.BinCommand(command...)
	return append(command,
		"--node", fmt.Sprintf("tcp://%s:26657", node.HostName()),
		"--chain-id", node.Chain.Config().ChainID,
	)
}

// BinCommand is a helper to retrieve a full command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to return the full command.
// Will include additional flags for home directory and chain ID.
func (node *Node) BinCommand(command ...string) []string {
	command = append([]string{node.Chain.Config().Bin}, command...)
	return append(command,
		"--home", node.HomeDir(),
	)
}

// ExecBin is a helper to execute a command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to execute the command against the node.
// Will include additional flags for home directory and chain ID.
func (node *Node) ExecBin(ctx context.Context, command ...string) ([]byte, []byte, error) {
	return node.Exec(ctx, node.BinCommand(command...), nil)
}

// QueryCommand is a helper to retrieve the full query command. For example,
// if chain node binary is gaiad, and desired command is `gaiad query gov params`,
// pass ("gov", "params") for command to return the full command with all necessary
// flags to query the specific node.
func (node *Node) QueryCommand(command ...string) []string {
	command = append([]string{"query"}, command...)
	return node.NodeCommand(append(command,
		"--output", "json",
	)...)
}

// ExecQuery is a helper to execute a query command. For example,
// if chain node binary is gaiad, and desired command is `gaiad query gov params`,
// pass ("gov", "params") for command to execute the query against the node.
// Returns response in json format.
func (node *Node) ExecQuery(ctx context.Context, command ...string) ([]byte, []byte, error) {
	return node.Exec(ctx, node.QueryCommand(command...), nil)
}

// ExecInit with custom home. This is a helper function to create new sequencer pubkey
func (node *Node) ExecInit(ctx context.Context, name, home string) ([]byte, []byte, error) {
	command := []string{}
	command = append(command, node.Chain.Config().Bin, "init", name, "--home", home)
	return node.Exec(ctx, command, nil)
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
	suffix := "-" + strconv.FormatInt(int64(h.Sum32()), 36)

	wantLen := stakingtypes.MaxMonikerLength - len(suffix)

	// Half of the want length, minus 2 to account for half of the ... we add in the middle.
	keepLen := (wantLen / 2) - 2

	return m[:keepLen] + "..." + m[len(m)-keepLen:] + suffix
}

// InitHomeFolder initializes a home folder for the given node
func (node *Node) InitHomeFolder(ctx context.Context) error {
	node.lock.Lock()
	defer node.lock.Unlock()

	_, _, err := node.ExecBin(ctx,
		"init", CondenseMoniker(node.Name()),
		"--chain-id", node.Chain.Config().ChainID,
	)
	return err
}

// WriteFile accepts file contents in a byte slice and writes the contents to
// the docker filesystem. relPath describes the location of the file in the
// docker volume relative to the home directory
func (node *Node) WriteFile(ctx context.Context, content []byte, relPath string) error {
	fw := dockerutil.NewFileWriter(node.logger(), node.DockerClient, node.TestName)
	return fw.WriteFile(ctx, node.VolumeName, node.Chain.Config().Name, relPath, content)
}

// CopyFile adds a file from the host filesystem to the docker filesystem
// relPath describes the location of the file in the docker volume relative to
// the home directory
func (node *Node) CopyFile(ctx context.Context, srcPath, dstPath string) error {
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return node.WriteFile(ctx, content, dstPath)
}

// ReadFile reads the contents of a single file at the specified path in the docker filesystem.
// relPath describes the location of the file in the docker volume relative to the home directory.
func (node *Node) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	fr := dockerutil.NewFileRetriever(node.logger(), node.DockerClient, node.TestName)
	gen, err := fr.SingleFileContent(ctx, node.VolumeName, node.Chain.Config().Name, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file at %s: %w", relPath, err)
	}
	return gen, nil
}

// CreateKey creates a key in the keyring backend test for the given node
func (node *Node) CreateKey(ctx context.Context, name string) error {
	node.lock.Lock()
	defer node.lock.Unlock()

	_, _, err := node.ExecBin(ctx,
		"keys", "add", name,
		"--coin-type", node.Chain.Config().CoinType,
		"--keyring-backend", keyring.BackendTest,
	)
	return err
}

// CreateKeyWithKeyDir creates a key in the keyring backend test for the given node
func (node *Node) CreateKeyWithKeyDir(ctx context.Context, name string, keyDir string) error {
	node.lock.Lock()
	defer node.lock.Unlock()

	_, _, err := node.ExecBin(ctx,
		"keys", "add", name,
		"--coin-type", node.Chain.Config().CoinType,
		"--keyring-backend", keyring.BackendTest,
		"--keyring-dir", keyDir+"/sequencer_keys",
	)
	return err
}

// RecoverKey restores a key from a given mnemonic.
func (node *Node) RecoverKey(ctx context.Context, keyName, mnemonic string) error {
	command := []string{
		"sh",
		"-c",
		fmt.Sprintf(`echo %q | %s keys add %s --recover --keyring-backend %s --coin-type %s --home %s --output json`, mnemonic, node.Chain.Config().Bin, keyName, keyring.BackendTest, node.Chain.Config().CoinType, node.HomeDir()),
	}

	node.lock.Lock()
	defer node.lock.Unlock()

	_, _, err := node.Exec(ctx, command, nil)
	return err
}

func (node *Node) IsAboveSDK47(ctx context.Context) bool {
	// In SDK v47, a new genesis core command was added. This spec has many state breaking features
	// so we use this to switch between new and legacy SDK logic.
	// https://github.com/cosmos/cosmos-sdk/pull/14149
	return node.HasCommand(ctx, "genesis")
}

// AddGenesisAccount adds a genesis account for each key
func (node *Node) AddGenesisAccount(ctx context.Context, address string, genesisAmount []types.Coin) error {
	amount := ""
	for i, coin := range genesisAmount {
		if i != 0 {
			amount += ","
		}
		amount += fmt.Sprintf("%s%s", coin.Amount.String(), coin.Denom)
	}

	node.lock.Lock()
	defer node.lock.Unlock()

	// Adding a genesis account should complete instantly,
	// so use a 1-minute timeout to more quickly detect if Docker has locked up.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	var command []string
	if node.IsAboveSDK47(ctx) {
		command = append(command, "genesis")
	}

	command = append(command, "add-genesis-account", address, amount)

	if node.Chain.Config().UsingChainIDFlagCLI {
		command = append(command, "--chain-id", node.Chain.Config().ChainID)
	}

	_, _, err := node.ExecBin(ctx, command...)

	return err
}

// Gentx generates the gentx for a given node
func (node *Node) Gentx(ctx context.Context, name string, genesisSelfDelegation types.Coin) error {
	node.lock.Lock()
	defer node.lock.Unlock()

	var command []string
	if node.IsAboveSDK47(ctx) {
		command = append(command, "genesis")
	}

	command = append(command, "gentx", valKey, fmt.Sprintf("%s%s", genesisSelfDelegation.Amount.String(), genesisSelfDelegation.Denom),
		"--keyring-backend", keyring.BackendTest,
		"--chain-id", node.Chain.Config().ChainID)

	_, _, err := node.ExecBin(ctx, command...)
	return err
}

func (node *Node) RegisterRollAppToHub(ctx context.Context, keyName, bech32, rollappChainID, sequencerAddr, bech32Prefix, keyDir string, flags map[string]string) error {
	var command []string
	var vmtype string
	const charset = "abcdefghijklmnopqrstuvwxyz"
	seededRand := rand.New(rand.NewSource(uint64(time.Now().UnixNano())))
	alias := make([]byte, 5)
	for i := range alias {
		alias[i] = charset[seededRand.Intn(len(charset))]
	}
	lastThree := node.TestName[len(node.TestName)-3:]
	checksum := "aaa"
	keyPath := keyDir + "/sequencer_keys"

	if lastThree == "EVM" {
		vmtype = "EVM"
		command = append(
			command, "rollapp", "create-rollapp",
			rollappChainID, string(alias), vmtype, "--bech32-prefix", bech32Prefix, "--init-sequencer", sequencerAddr, "--genesis-checksum", checksum, "--metadata", keyDir+"/metadata.json", "--genesis-accounts", bech32+":"+dymension.GenesisEventAmount.String(),
			"--native-denom", keyDir+"/native_denom.json", "--initial-supply", "100000000010100000000000000000000",
			"--broadcast-mode", "async", "--keyring-dir", keyPath)
	} else {
		vmtype = "WASM"
		command = append(
			command, "rollapp", "create-rollapp",
			rollappChainID, string(alias), vmtype, "--bech32-prefix", bech32Prefix, "--init-sequencer", sequencerAddr, "--genesis-checksum", checksum, "--metadata", keyDir+"/metadata.json", "--genesis-accounts", bech32+":"+dymension.GenesisEventAmount.String(),
			"--native-denom", keyDir+"/native_denom.json", "--initial-supply", "10200000000000000000000",
			"--broadcast-mode", "async", "--keyring-dir", keyPath)
	}

	for flagName := range flags {
		command = append(command, "--"+flagName, flags[flagName])
	}
	_, _ = node.ExecTx(ctx, keyName, command...)
	_, err := node.ExecTx(ctx, keyName, command...)
	return err
}

func (node *Node) RegisterSequencerToHub(ctx context.Context, keyName, rollappChainID, seq, keyDir string) error {
	var command []string
	keyPath := keyDir + "/sequencer_keys"
	command = append(command, "sequencer", "create-sequencer", seq, rollappChainID, "1000000000adym", keyDir+"/metadata_sequencer.json",
		"--broadcast-mode", "async", "--keyring-dir", keyPath, "--gas", "auto")

	_, err := node.ExecTx(ctx, keyName, command...)
	return err
}

func (node *Node) RegisterEVMValidatorToHub(ctx context.Context, keyName string) error {
	var command []string
	addr, err := node.KeyBech32(ctx, "validator", "val")
	if err != nil {
		return err
	}
	command = append(command, "qgb", "register", addr, "0x966e6f22781EF6a6A82BBB4DB3df8E225DfD9488",
		"--broadcast-mode", "block")
	_, err = node.ExecTx(ctx, keyName, command...)

	return err
}

func (node *Node) Unbond(ctx context.Context, keyName, keyDir string) error {
	var command []string
	if keyDir != "" {
		keyPath := keyDir + "/sequencer_keys"
		command = append(command, "sequencer", "unbond",
			"--broadcast-mode", "async", "--gas", "auto", "--keyring-dir", keyPath)
	} else {
		command = append(command, "sequencer", "unbond",
			"--broadcast-mode", "async", "--gas", "auto")
	}

	_, err := node.ExecTx(ctx, keyName, command...)
	return err
}

// CollectGentxs runs collect gentxs on the node's home folders
func (node *Node) CollectGentxs(ctx context.Context) error {
	command := []string{node.Chain.Config().Bin}
	if node.IsAboveSDK47(ctx) {
		command = append(command, "genesis")
	}

	command = append(command, "collect-gentxs", "--home", node.HomeDir())

	node.lock.Lock()
	defer node.lock.Unlock()

	_, _, err := node.Exec(ctx, command, nil)
	return err
}

type CosmosTx struct {
	TxHash string `json:"txhash"`
	Code   int    `json:"code"`
	RawLog string `json:"raw_log"`
}

func (node *Node) SendIBCTransfer(
	ctx context.Context,
	channelID string,
	keyName string,
	toWallet ibc.WalletData,
	options ibc.TransferOptions,
) (string, error) {
	command := []string{
		"ibc-transfer", "transfer", "transfer", channelID,
		toWallet.Address, fmt.Sprintf("%s%s", toWallet.Amount.String(), toWallet.Denom),
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
	return node.ExecTx(ctx, keyName, command...)
}

func (node *Node) ConvertCoin(ctx context.Context, keyName, coin, receiver string) (string, error) {
	command := []string{
		"erc20", "convert-coin", coin, receiver,
		"--gas", "auto",
	}

	return node.ExecTx(ctx, keyName, command...)
}

func (node *Node) ConvertErc20(ctx context.Context, keyName, contractAddress, amount, sender, receiver, chainId string) (string, error) {
	command := []string{"erc20", "convert-erc20", contractAddress, amount, receiver, "--gas", "auto"}
	return node.ExecTx(ctx, keyName, command...)
}

func (node *Node) QueryErc20TokenPair(ctx context.Context, token string) (TokenPair, error) {
	command := []string{"erc20", "token-pair", token}
	stdout, _, err := node.ExecQuery(ctx, command...)
	if err != nil {
		return TokenPair{}, err
	}

	var tokenPair Erc20TokenPairResponse
	err = json.Unmarshal(stdout, &tokenPair)
	if err != nil {
		return TokenPair{}, err
	}

	return tokenPair.TokenPair, nil
}

func (node *Node) GetIbcTxFromTxHash(ctx context.Context, txHash string) (tx ibc.Tx, _ error) {
	txResp, err := node.getTransaction(node.CliContext(), txHash)
	if err != nil {
		return tx, fmt.Errorf("failed to get transaction %s: %w", txHash, err)
	}
	if txResp.Code != 0 {
		return tx, fmt.Errorf("error in transaction (code: %d): %s", txResp.Code, txResp.RawLog)
	}
	tx.Height = int64(txResp.Height)
	tx.TxHash = txHash
	// In cosmos, user is charged for entire gas requested, not the actual gas used.
	tx.GasSpent = txResp.GasWanted

	const evType = "send_packet"
	events := txResp.Events

	var (
		seq, _           = AttributeValue(events, evType, "packet_sequence")
		srcPort, _       = AttributeValue(events, evType, "packet_src_port")
		srcChan, _       = AttributeValue(events, evType, "packet_src_channel")
		dstPort, _       = AttributeValue(events, evType, "packet_dst_port")
		dstChan, _       = AttributeValue(events, evType, "packet_dst_channel")
		timeoutHeight, _ = AttributeValue(events, evType, "packet_timeout_height")
		timeoutTs, _     = AttributeValue(events, evType, "packet_timeout_timestamp")
		data, _          = AttributeValue(events, evType, "packet_data")
	)
	tx.Packet.SourcePort = srcPort
	tx.Packet.SourceChannel = srcChan
	tx.Packet.DestPort = dstPort
	tx.Packet.DestChannel = dstChan
	tx.Packet.TimeoutHeight = timeoutHeight
	tx.Packet.Data = []byte(data)

	seqNum, err := strconv.Atoi(seq)
	if err != nil {
		return tx, fmt.Errorf("invalid packet sequence from events %s: %w", seq, err)
	}
	tx.Packet.Sequence = uint64(seqNum)

	timeoutNano, err := strconv.ParseUint(timeoutTs, 10, 64)
	if err != nil {
		return tx, fmt.Errorf("invalid packet timestamp timeout %s: %w", timeoutTs, err)
	}
	tx.Packet.TimeoutTimestamp = ibc.Nanoseconds(timeoutNano)

	return tx, nil
}

func (node *Node) SendFunds(ctx context.Context, keyName string, toWallet ibc.WalletData) error {
	_, err := node.ExecTx(ctx,
		keyName, "bank", "send", keyName,
		toWallet.Address, fmt.Sprintf("%s%s", toWallet.Amount.String(), toWallet.Denom),
	)
	return err
}

func (node *Node) GetNodeId(ctx context.Context) (string, error) {
	command := []string{
		"rollappd", "dymint", "show-node-id", "--home", node.HomeDir(),
	}

	stdout, _, err := node.Exec(ctx, command, nil)
	if err != nil {
		return "", err
	}
	return string(stdout), nil
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

// QuerySequencerStatus queries the status of a given sequencer address, returns all sequencers if sequencerAddress is empty.
func (node *Node) QuerySequencerStatus(ctx context.Context, sequencerAddress string) (*QuerySequencersResponse, error) {
	var command []string
	command = append(command, "sequencer", "list-sequencer")

	stdout, _, err := node.ExecQuery(ctx, command...)
	if err != nil {
		return nil, err
	}

	// Unmarshal the response
	var sqcStatuses QuerySequencersResponse
	err = json.Unmarshal(stdout, &sqcStatuses)
	fmt.Println(sqcStatuses)
	if err != nil {
		fmt.Println("Error on unmarshal stdout: ", err)
		return nil, err
	}

	// If sequencerAddress is empty, return all sequencers
	if sequencerAddress == "" {
		return &sqcStatuses, nil
	}

	// Filter sequencers by the given sequencerAddress
	filteredSequencers := []Sequencer{}
	for _, sequencer := range sqcStatuses.Sequencers {
		if sequencer.Address == sequencerAddress {
			filteredSequencers = append(filteredSequencers, sequencer)
		}
	}

	// Return the filtered result
	return &QuerySequencersResponse{
		Sequencers: filteredSequencers,
		Pagination: sqcStatuses.Pagination,
	}, nil
}

// StoreContract takes a file path to smart contract and stores it on-chain. Returns the contracts code id.
func (node *Node) StoreContract(ctx context.Context, keyName string, fileName string, extraExecTxArgs ...string) (string, error) {
	_, file := filepath.Split(fileName)
	err := node.CopyFile(ctx, fileName, file)
	if err != nil {
		return "", fmt.Errorf("writing contract file to docker volume: %w", err)
	}

	cmd := []string{"wasm", "store", path.Join(node.HomeDir(), file), "--gas", "auto"}
	cmd = append(cmd, extraExecTxArgs...)

	if _, err := node.ExecTx(ctx, keyName, cmd...); err != nil {
		return "", err
	}

	err = testutil.WaitForBlocks(ctx, 5, node.Chain)
	if err != nil {
		return "", fmt.Errorf("wait for blocks: %w", err)
	}

	stdout, _, err := node.ExecQuery(ctx, "wasm", "list-code", "--reverse")
	if err != nil {
		return "", err
	}

	res := CodeInfosResponse{}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		return "", err
	}

	return res.CodeInfos[0].CodeID, nil
}

func (node *Node) getTransaction(clientCtx client.Context, txHash string) (*types.TxResponse, error) {
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
func (node *Node) HasCommand(ctx context.Context, command ...string) bool {
	_, _, err := node.ExecBin(ctx, command...)
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

// VoteOnProposal submits a vote for the specified proposal.
func (node *Node) VoteOnProposal(ctx context.Context, keyName string, proposalID string, vote string) error {
	_, err := node.ExecTx(ctx, keyName,
		"gov", "vote",
		proposalID, vote, "--gas", "auto",
	)
	return err
}

// QueryLatestState returns the latest state info of a rollapp based on rollapp id.
func (node *Node) QueryLatestStateIndex(ctx context.Context, rollappChainID string) (*StateIndexResponse, error) {
	var command []string
	command = append(command, "rollapp", "latest-state-index", rollappChainID)

	stdout, _, err := node.ExecQuery(ctx, command...)
	if err != nil {
		return nil, err
	}

	var stateIndex StateIndexResponse
	err = json.Unmarshal(stdout, &stateIndex)
	if err != nil {
		return nil, err
	}
	return &stateIndex, nil
}

// QueryDenomMetadata returns denom metadata of a given denom
func (node *Node) QueryDenomMetadata(ctx context.Context, denom string) (*DenomMetadata, error) {
	var command []string
	command = append(command, "bank", "denom-metadata", "--denom", denom)

	stdout, _, err := node.ExecQuery(ctx, command...)
	if err != nil {
		return nil, err
	}

	var denomMetadata DenomMetadataResponse
	err = json.Unmarshal(stdout, &denomMetadata)
	if err != nil {
		return nil, err
	}
	return &denomMetadata.Metadata, nil
}

// QueryAllDenomMetadata returns denom metadata of a given denom
func (node *Node) QueryAllDenomMetadata(ctx context.Context) (*QueryDenomsMetadataResponse, error) {
	var command []string
	command = append(command, "bank", "denom-metadata")

	stdout, _, err := node.ExecQuery(ctx, command...)
	if err != nil {
		return nil, err
	}

	var denomMetadata QueryDenomsMetadataResponse
	err = json.Unmarshal(stdout, &denomMetadata)
	if err != nil {
		return nil, err
	}
	return &denomMetadata, nil
}

// QueryProposal returns the state and details of a governance proposal.
func (node *Node) QueryProposal(ctx context.Context, proposalID string) (*ProposalResponse, error) {
	stdout, _, err := node.ExecQuery(ctx, "gov", "proposal", proposalID)
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

// QueryModuleAccount returns the information about a module account
func (node *Node) QueryModuleAccount(ctx context.Context, moduleName string) (*ModuleAccountResponse, error) {
	stdout, _, err := node.ExecQuery(ctx, "auth", "module-account", moduleName, "--output=json")
	if err != nil {
		return nil, err
	}
	var moduleAccount ModuleAccountResponse
	err = json.Unmarshal(stdout, &moduleAccount)
	if err != nil {
		return nil, err
	}
	return &moduleAccount, nil
}

// QueryEscrowAddress returns the escrow address of a given channel.
func (node *Node) QueryEscrowAddress(ctx context.Context, portID, channelID string) (string, error) {
	stdout, _, err := node.ExecQuery(ctx, "ibc-transfer", "escrow-address", portID, channelID, "--output=json")
	if err != nil {
		return "", err
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
}

// QueryHubGenesisState query hub genesis state
func (node *Node) QueryHubGenesisState(ctx context.Context) (HubGenesisState, error) {
	stdout, _, err := node.ExecQuery(ctx, "hubgenesis", "state")
	if err != nil {
		return HubGenesisState{}, err
	}

	var hubGenesisState HubGenesisState
	err = json.Unmarshal(stdout, &hubGenesisState)
	if err != nil {
		return HubGenesisState{}, err
	}

	return hubGenesisState, nil
}

func (node *Node) QueryPacketCommitments(ctx context.Context,
	portID string,
	channelID string,
) (*QueryPacketCommitmentsResponse, error) {
	stdout, _, err := node.ExecQuery(ctx, "ibc", "channel", "packet-commitments", portID, channelID)
	if err != nil {
		return nil, err
	}
	var resp QueryPacketCommitmentsResponse
	err = json.Unmarshal(stdout, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (node *Node) QueryClientStatus(ctx context.Context, clientId string) (*QueryClientStatusResponse, error) {
	stdout, _, err := node.ExecQuery(ctx, "ibc", "client", "status", clientId)
	if err != nil {
		return nil, err
	}
	var resp QueryClientStatusResponse
	err = json.Unmarshal(stdout, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// SubmitFraudProposal a fraud proposal to the chain.
func (node *Node) SubmitFraudProposal(ctx context.Context, keyName string, rollappId, height, proposerAddr, clientId, title, description, deposit string) (string, error) {
	var command []string
	command = append(command, "gov", "submit-legacy-proposal", "submit-fraud-proposal",
		rollappId, height, proposerAddr, clientId, "--title=fraud", "--description=fraud",
		"--gas", "auto", "--broadcast-mode", "async", "--deposit", deposit)
	return node.ExecTx(ctx, keyName, command...)
}

// SubmitUpdateClientProposal a update client proposal to the chain.
func (node *Node) SubmitUpdateClientProposal(ctx context.Context, keyName, subjectClientId, substituteClientId, deposit string) (string, error) {
	var command []string
	command = append(command, "gov", "submit-legacy-proposal", "update-client", subjectClientId, substituteClientId,
		"--title=update_client", "--description=update_client",
		"--gas", "auto", "--broadcast-mode", "async", "--deposit", deposit)

	return node.ExecTx(ctx, keyName, command...)
}

// SubmitProposal submits a gov v1 proposal to the chain.
func (node *Node) SubmitProposal(ctx context.Context, keyName string, prop TxProposalv1) (string, error) {
	// Write msg to container
	file := "proposal.json"
	propJson, err := json.MarshalIndent(prop, "", " ")
	if err != nil {
		return "", err
	}
	fw := dockerutil.NewFileWriter(node.logger(), node.DockerClient, node.TestName)
	if err := fw.WriteFile(ctx, node.VolumeName, node.Chain.Config().Name, file, propJson); err != nil {
		return "", fmt.Errorf("writing contract file to docker volume: %w", err)
	}

	command := []string{
		"gov", "submit-proposal",
		path.Join(node.HomeDir(), file), "--gas", "auto",
	}

	return node.ExecTx(ctx, keyName, command...)
}

// UpgradeProposal submits a software-upgrade governance proposal to the chain.
func (node *Node) UpgradeLegacyProposal(ctx context.Context, keyName string, prop SoftwareUpgradeProposal) (string, error) {
	command := []string{
		"gov", "submit-legacy-proposal",
		"software-upgrade", prop.Name,
		"--upgrade-height", strconv.FormatInt(prop.Height, 10),
		"--title", prop.Title,
		"--description", prop.Description,
		"--deposit", prop.Deposit,
		"--gas=auto",
		"--no-validate",
	}

	if prop.Info != "" {
		command = append(command, "--upgrade-info", prop.Info)
	}

	return node.ExecTx(ctx, keyName, command...)
}

// TextProposal submits a text governance proposal to the chain.
func (node *Node) TextProposal(ctx context.Context, keyName string, prop TextProposal) (string, error) {
	command := []string{
		"gov", "submit-legacy-proposal",
		"--type", "text",
		"--title", prop.Title,
		"--description", prop.Description,
		"--deposit", prop.Deposit,
	}
	if prop.Expedited {
		command = append(command, "--is-expedited=true")
	}
	return node.ExecTx(ctx, keyName, command...)
}

// ParamChangeProposal submits a param change proposal to the chain, signed by keyName.
func (node *Node) ParamChangeProposal(ctx context.Context, keyName string, prop *paramsutils.ParamChangeProposalJSON) (string, error) {
	content, err := json.Marshal(prop)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(content)
	proposalFilename := fmt.Sprintf("%x.json", hash)
	err = node.WriteFile(ctx, content, proposalFilename)
	if err != nil {
		return "", fmt.Errorf("writing param change proposal: %w", err)
	}

	proposalPath := filepath.Join(node.HomeDir(), proposalFilename)

	command := []string{
		"gov", "submit-legacy-proposal",
		"param-change",
		proposalPath,
		"--gas=auto",
	}

	return node.ExecTx(ctx, keyName, command...)
}

func (node *Node) RegisterIBCTokenDenomProposal(ctx context.Context, keyName, deposit, proposalPath string) (string, error) {
	command := []string{
		"gov", "submit-legacy-proposal",
		"register-coin",
		proposalPath,
		"--title", "Register IBC token denom proposal",
		"--description", "Register IBC token denom proposal",
		"--deposit", deposit,
		"--gas=auto",
	}

	return node.ExecTx(ctx, keyName, command...)
}

// CrisisInvariant run crisis module invariant-broken command
func (node *Node) CrisisInvariant(ctx context.Context, keyName string, module, invariant string) (string, error) {
	command := []string{
		"crisis", "invariant-broken",
		module, invariant,
	}
	return node.ExecTx(ctx, keyName, command...)
}

// QueryParam returns the state and details of a subspace param.
func (node *Node) QueryParam(ctx context.Context, subspace, key string) (*ParamChange, error) {
	stdout, _, err := node.ExecQuery(ctx, "params", "subspace", subspace, key)
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

func (node *Node) QueryIbcTransferParams(ctx context.Context) (*Params, error) {
	stdout, _, err := node.ExecQuery(ctx, "ibc-transfer", "params")
	if err != nil {
		return nil, err
	}

	var param Params
	err = json.Unmarshal(stdout, &param)
	if err != nil {
		return nil, err
	}
	return &param, nil
}

func (node *Node) QueryDelayedACKParams(ctx context.Context) (DelayedACKParams, error) {
	stdout, _, err := node.ExecQuery(ctx, "delayedack", "params")
	if err != nil {
		return DelayedACKParams{}, err
	}

	var param DelayedACKParams
	err = json.Unmarshal(stdout, &param)
	if err != nil {
		return DelayedACKParams{}, err
	}
	return param, nil
}

func (node *Node) ExportState(ctx context.Context, height int64) (string, error) {
	node.lock.Lock()
	defer node.lock.Unlock()

	stdout, stderr, err := node.ExecBin(ctx, "export", "--height", fmt.Sprint(height))
	if err != nil {
		return "", err
	}
	// output comes to stderr on older versions
	return string(stdout) + string(stderr), nil
}

func (node *Node) UnsafeResetAll(ctx context.Context) error {
	node.lock.Lock()
	defer node.lock.Unlock()

	_, _, err := node.ExecBin(ctx, "unsafe-reset-all")
	return err
}

func (node *Node) GetHashOfBlockHeight(ctx context.Context, height string) (string, error) {
	command := []string{"celestia-appd", "query", "block", height, "--node", fmt.Sprintf("tcp://%s:26657", node.HostName())}

	stdout, _, err := node.Exec(ctx, command, nil)
	if err != nil {
		return "", err
	}
	var jsonResult map[string]interface{}
	if err := json.Unmarshal(stdout, &jsonResult); err != nil {
		return "", err
	}

	blockId, ok := jsonResult["block_id"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed to parse block id")
	}

	hash, ok := blockId["hash"].(string)
	if !ok {
		return "", fmt.Errorf("failed to parse block hash from block id ")
	}

	return hash, nil
}

func (node *Node) GetHashOfBlockHeightWithCustomizeRpcEndpoint(ctx context.Context, height, rpcEndpoint string) (string, error) {
	command := []string{"celestia-appd", "query", "block", height, "--node", rpcEndpoint}

	stdout, _, err := node.Exec(ctx, command, nil)
	if err != nil {
		return "", err
	}
	var jsonResult map[string]interface{}
	if err := json.Unmarshal(stdout, &jsonResult); err != nil {
		return "", err
	}

	blockId, ok := jsonResult["block_id"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed to parse block id")
	}

	hash, ok := blockId["hash"].(string)
	if !ok {
		return "", fmt.Errorf("failed to parse block hash from block id ")
	}

	return hash, nil
}

func (node *Node) CreateNodeContainer(ctx context.Context, command []string) error {
	chainCfg := node.Chain.Config()

	var cmd []string
	if chainCfg.NoHostMount {
		cmd = []string{"sh", "-c", fmt.Sprintf("cp -r %s %s_nomnt && %s start --home %s_nomnt --x-crisis-skip-assert-invariants", node.HomeDir(), node.HomeDir(), chainCfg.Bin, node.HomeDir())}
	} else {
		cmd = []string{chainCfg.Bin, "start", "--home", node.HomeDir(), "--x-crisis-skip-assert-invariants"}
	}
	if _, ok := node.Chain.(ibc.RollApp); ok {
		cmd = []string{chainCfg.Bin, "start", "--home", node.HomeDir()}
	}
	chainType := strings.Split(chainCfg.Type, "-")

	if chainType[0] == "rollapp" && chainType[1] == "gm" {
		cmd = append(command, "--home", node.HomeDir())
	}

	if chainType[0] == "hub" && chainType[1] == "celes" {
		cmd = []string{"/bin/bash", "/opt/start.sh", node.HomeDir()}
	}
	return node.containerLifecycle.CreateContainer(ctx, node.TestName, node.NetworkID, node.Image, sentryPorts, node.Bind(), node.HostName(), cmd)
}

func (node *Node) StartContainer(ctx context.Context) error {

	for _, s := range node.Sidecars {
		err := s.containerLifecycle.Running(ctx)
		if s.preStart && err != nil {
			if err := s.CreateContainer(ctx); err != nil {
				return err
			}
			if err := s.StartContainer(ctx); err != nil {
				return err
			}
		}
	}

	if err := node.containerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := node.containerLifecycle.GetHostPorts(ctx, rpcPort, grpcPort, apiPort)
	if err != nil {
		return err
	}
	node.hostRPCPort, node.hostGRPCPort, node.hostAPIPort = hostPorts[0], hostPorts[1], hostPorts[2]

	err = node.NewClient("tcp://" + node.hostRPCPort)
	if err != nil {
		return err
	}

	time.Sleep(5 * time.Second)
	return retry.Do(func() error {
		stat, err := node.Client.Status(ctx)
		if err != nil {
			return err
		}
		if stat != nil && stat.SyncInfo.CatchingUp {
			return fmt.Errorf("still catching up: height(%d) catching-up(%t)",
				stat.SyncInfo.LatestBlockHeight, stat.SyncInfo.CatchingUp)
		}
		return nil
	}, retry.Context(ctx), retry.Attempts(40), retry.Delay(3*time.Second), retry.DelayType(retry.FixedDelay))
}

func (node *Node) StopContainer(ctx context.Context) error {
	for _, s := range node.Sidecars {
		if err := s.StopContainer(ctx); err != nil {
			return err
		}
	}
	return node.containerLifecycle.StopContainer(ctx)
}

func (node *Node) RemoveContainer(ctx context.Context) error {
	for _, s := range node.Sidecars {
		if err := s.RemoveContainer(ctx); err != nil {
			return err
		}
	}
	return node.containerLifecycle.RemoveContainer(ctx)
}

// InitValidatorFiles creates the node files and signs a genesis transaction
func (node *Node) InitValidatorGenTx(
	ctx context.Context,
	chainConfig *ibc.ChainConfig,
	genesisAmounts []types.Coin,
	genesisSelfDelegation types.Coin,
) error {
	if err := node.CreateKey(ctx, valKey); err != nil {
		return err
	}
	bech32, err := node.AccountKeyBech32(ctx, valKey)
	if err != nil {
		return err
	}
	if err := node.AddGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
		return err
	}

	return node.Gentx(ctx, valKey, genesisSelfDelegation)
}

func (node *Node) InitFullNodeFiles(ctx context.Context) error {
	if err := node.InitHomeFolder(ctx); err != nil {
		return err
	}

	return node.SetTestConfig(ctx)
}

// NodeID returns the persistent ID of a given node.
func (node *Node) NodeID(ctx context.Context) (string, error) {
	// This used to call p2p.LoadNodeKey against the file on the host,
	// but because we are transitioning to operating on Docker volumes,
	// we only have to tmjson.Unmarshal the raw content.
	j, err := node.ReadFile(ctx, "config/node_key.json")
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
func (node *Node) KeyBech32(ctx context.Context, name string, bech string) (string, error) {
	command := []string{node.Chain.Config().Bin, "keys", "show", "--address", name,
		"--home", node.HomeDir(),
		"--keyring-backend", keyring.BackendTest,
	}

	if bech != "" {
		command = append(command, "--bech", bech)
	}

	stdout, stderr, err := node.Exec(ctx, command, nil)
	if err != nil {
		return "", fmt.Errorf("failed to show key %q (stderr=%q): %w", name, stderr, err)
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
}

// KeyBech32WithKeyDir retrieves the named key's address in bech32 format from the node.
// bech is the bech32 prefix (acc|val|cons). If empty, defaults to the account key (same as "acc").
func (node *Node) KeyBech32WithKeyDir(ctx context.Context, name string, keyDir string, bech string) (string, error) {
	command := []string{node.Chain.Config().Bin, "keys", "show", "--address", name,
		"--home", node.HomeDir(),
		"--keyring-backend", keyring.BackendTest,
		"--keyring-dir", keyDir + "/sequencer_keys",
	}

	if bech != "" {
		command = append(command, "--bech", bech)
	}

	stdout, stderr, err := node.Exec(ctx, command, nil)
	if err != nil {
		return "", fmt.Errorf("failed to show key %q (stderr=%q): %w", name, stderr, err)
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
}

// AccountKeyBech32 retrieves the named key's address in bech32 account format.
func (node *Node) AccountKeyBech32(ctx context.Context, name string) (string, error) {
	return node.KeyBech32(ctx, name, "")
}

// AccountHubKeyBech32 retrieves the named key's address in bech32 account format.
func (node *Node) AccountKeyBech32WithKeyDir(ctx context.Context, name string, keyDir string) (string, error) {
	return node.KeyBech32WithKeyDir(ctx, name, keyDir, "")
}

// PeerString returns the string for connecting the nodes passed in
func (nodes Nodes) PeerString(ctx context.Context) string {
	addrs := make([]string, len(nodes))
	for i, n := range nodes {
		id, err := n.NodeID(ctx)
		if err != nil {
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
func (nodes Nodes) LogGenesisHashes(ctx context.Context) error {
	for _, n := range nodes {
		gen, err := n.GenesisFileContent(ctx)
		if err != nil {
			return err
		}

		n.logger().Info("Genesis", zap.String("hash", fmt.Sprintf("%X", sha256.Sum256(gen))))
	}
	return nil
}

func (nodes Nodes) logger() *zap.Logger {
	if len(nodes) == 0 {
		return zap.NewNop()
	}
	return nodes[0].logger()
}

func (node *Node) Exec(ctx context.Context, cmd []string, env []string) ([]byte, []byte, error) {
	job := dockerutil.NewImage(node.logger(), node.DockerClient, node.NetworkID, node.TestName, node.Image.Repository, node.Image.Version)
	opts := dockerutil.ContainerOptions{
		Env:   env,
		Binds: node.Bind(),
	}
	res := job.Run(ctx, cmd, opts)
	return res.Stdout, res.Stderr, res.Err
}

func (node *Node) logger() *zap.Logger {
	return node.log.With(
		zap.String("chain_id", node.Chain.Config().ChainID),
		zap.String("test", node.TestName),
	)
}

func (node *Node) Logger() *zap.Logger {
	return node.logger()
}

// Celestia DA functions

// InitCelestiaDaBridge init Celestia DA bridge
func (node *Node) InitCelestiaDaBridge(ctx context.Context, nodeStore string, env []string) error {
	command := []string{"celestia", "bridge", "init", "--node.store", nodeStore}

	_, stderr, err := node.Exec(ctx, command, env)
	if err != nil {
		return fmt.Errorf("failed to init celesta DA bridge (stderr=%q): %w", stderr, err)
	}
	return nil
}

// StartCelestiaDaBridge start Celestia DA bridge
func (node *Node) StartCelestiaDaBridge(ctx context.Context, nodeStore, coreIp, accName, gatewayAddr, rpcAddr string, env []string) error {
	command := []string{"celestia", "bridge", "start", "--node.store", nodeStore, "--gateway", "--core.ip", coreIp,
		"--keyring.accname", accName, "--gateway.addr", gatewayAddr, "--rpc.addr", rpcAddr}

	_, stderr, err := node.Exec(ctx, command, env)
	if err != nil {
		return fmt.Errorf("failed to start celesta DA bridge (stderr=%q): %w", stderr, err)
	}
	return nil
}

// GetAuthTokenCelestiaDaBridge get token auth of Celestia DA bridge
func (node *Node) GetAuthTokenCelestiaDaBridge(ctx context.Context, nodeStore string) (token string, err error) {
	command := []string{"celestia", "bridge", "auth", "admin", "--node.store", nodeStore}

	stdout, stderr, err := node.Exec(ctx, command, nil)
	if err != nil {
		return "", fmt.Errorf("failed to start celesta DA bridge (stderr=%q): %w", stderr, err)
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
}

func (node *Node) InitCelestiaDaLightNode(ctx context.Context, nodeStore, p2pNetwork string, env []string) error {
	command := []string{"celestia", "light", "init", "--node.store", nodeStore, "--p2p.network", p2pNetwork}

	_, stderr, err := node.Exec(ctx, command, env)
	if err != nil {
		return fmt.Errorf("failed to init celesta DA light node (stderr=%q): %w", stderr, err)
	}
	return nil
}

// StartCelestiaDaBridge start Celestia DA bridge
func (node *Node) StartCelestiaDaLightNode(ctx context.Context, nodeStore, coreIp, p2pNetwork, accName string, env []string) error {
	command := []string{"celestia", "light", "start", "--node.store", nodeStore, "--gateway", "--core.ip", coreIp, "--p2p.network", p2pNetwork, "--keyring.accname", accName}

	_, stderr, err := node.Exec(ctx, command, env)
	if err != nil {
		return fmt.Errorf("failed to start celesta DA light node (stderr=%q): %w", stderr, err)
	}
	return nil
}

// GetAuthTokenCelestiaDaLight get token auth of Celestia DA Light client
func (node *Node) GetAuthTokenCelestiaDaLight(ctx context.Context, p2pnetwork, nodeStore string) (token string, err error) {
	command := []string{"celestia", "light", "auth", "admin", "--p2p.network", p2pnetwork, "--node.store", nodeStore}

	stdout, stderr, err := node.Exec(ctx, command, nil)
	if err != nil {
		return "", fmt.Errorf("failed to start celesta DA light client (stderr=%q): %w", stderr, err)
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
}

func (node *Node) GetDABlockHeight(ctx context.Context) (string, error) {
	command := []string{"curl", fmt.Sprintf("http://%s:26657/block", node.HostName())}

	stdout, stderr, err := node.Exec(ctx, command, nil)
	if err != nil {
		return "", fmt.Errorf("failed to start celesta DA bridge (stderr=%q): %w", stderr, err)
	}

	var celestiaResult CelestiaResponse
	if err := json.Unmarshal(stdout, &celestiaResult); err != nil {
		return "", fmt.Errorf("celestia block response unmarshal failed: %w", err)
	}

	return celestiaResult.Result.Block.Header.Height, nil
}

// Rollkit roll app functions
func (node *Node) ModifyConsensusGenesis(ctx context.Context) error {
	// get genesis file content
	genbz, err := node.GenesisFileContent(ctx)
	if err != nil {
		return err
	}

	appGenesis := map[string]interface{}{}
	err = json.Unmarshal(genbz, &appGenesis)
	if err != nil {
		return err
	}

	privateKeys, err := node.ExtractPrivateValKeyFile(ctx)
	if err != nil {
		return err
	}

	consensusGenesis := appGenesis["consensus"].(map[string]interface{})
	consensusGenesis["validators"] = []map[string]interface{}{
		{
			"address": privateKeys.Address,
			"pub_key": map[string]string{
				"type":  privateKeys.PubKey.Type,
				"value": privateKeys.PubKey.Value,
			},

			"power": "50000000000000",
			"name":  "Rollkit Sequencer",
		},
	}

	appGenesis["consensus"] = consensusGenesis

	genbz, err = json.Marshal(appGenesis)
	if err != nil {
		return err
	}

	err = node.OverwriteGenesisFile(ctx, genbz)
	if err != nil {
		return err
	}

	return nil
}

func (node *Node) FinalizePacketsUntilHeight(ctx context.Context, keyName, rollappID, height string) (string, error) {
	command := []string{
		"delayedack", "finalize-packets-until-height", rollappID, height,
		"--gas", "auto",
	}

	return node.ExecTx(ctx, keyName, command...)
}
