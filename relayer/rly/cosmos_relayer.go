package relayer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

const (
	RlyDefaultUidGid = "100:1000"
)

// CosmosRelayer is the ibc.Relayer implementation for github.com/cosmos/relayer.
type CosmosRelayer struct {
	// Embedded DockerRelayer so commands just work.
	*relayer.DockerRelayer
}

func NewCosmosRelayer(log *zap.Logger, testName string, cli *client.Client, relayerName, networkID string, options ...relayer.RelayerOption) *CosmosRelayer {
	c := commander{log: log}
	for _, opt := range options {
		switch o := opt.(type) {
		case relayer.RelayerOptionExtraStartFlags:
			c.extraStartFlags = o.Flags
		}
	}
	dr, err := relayer.NewDockerRelayer(context.TODO(), log, testName, cli, relayerName, networkID, c, options...)
	if err != nil {
		panic(err) // TODO: return
	}

	r := &CosmosRelayer{
		DockerRelayer: dr,
	}

	return r
}

type CosmosRelayerChainConfig struct {
	Type  string `json:"type"`
	Value Value  `json:"value"`
}
type Value struct {
	AccountPrefix  string        `json:"account-prefix"`
	ChainID        string        `json:"chain-id"`
	Debug          bool          `json:"debug"`
	GasAdjustment  float64       `json:"gas-adjustment"`
	GasPrices      string        `json:"gas-prices"`
	Key            string        `json:"key"`
	KeyringBackend string        `json:"keyring-backend"`
	OutputFormat   string        `json:"output-format"`
	RPCAddr        string        `json:"rpc-addr"`
	SignMode       string        `json:"sign-mode"`
	Timeout        string        `json:"timeout"`
	ClientType     string        `json:"client-type"`
	HttpAddr       string        `json:"http-addr" yaml:"http-addr"`           // added to support http queries to Dym Hub
	DymHub         bool          `json:"is-dym-hub" yaml:"is-dym-hub"`         // added to force wait for canonical client with Hub
	DymRollapp     bool          `json:"is-dym-rollapp" yaml:"is-dym-rollapp"` // added to support custom trust levels
	TrustPeriod    time.Duration `json:"trust-period" yaml:"trust-period"`
}

const (
	DefaultContainerImage   = "ghcr.io/cosmos/relayer"
	DefaultContainerVersion = "v2.3.1"
)

const (
	tmClientType = "07-tendermint"
	dmClientType = "01-dymint"
)

func ConfigToCosmosRelayerChainConfig(chainConfig ibc.ChainConfig, keyName, rpcAddr, apiAddr string) CosmosRelayerChainConfig {
	// by default clientType should be tmClientType
	clientType := tmClientType
	isHub := false
	isRA := false
	chainType := strings.Split(chainConfig.Type, "-")

	if chainType[0] == "rollapp" && chainType[1] == "dym" {
		clientType = dmClientType
		isHub = false
		isRA = true
	} else if chainType[0] == "hub" && chainType[1] == "dym" {
		isHub = true
		isRA = false
	}

	return CosmosRelayerChainConfig{
		Type: "cosmos",
		Value: Value{
			Key:            keyName,
			ChainID:        chainConfig.ChainID,
			RPCAddr:        rpcAddr,
			AccountPrefix:  chainConfig.Bech32Prefix,
			KeyringBackend: keyring.BackendTest,
			GasAdjustment:  chainConfig.GasAdjustment,
			GasPrices:      chainConfig.GasPrices,
			Debug:          true,
			Timeout:        "10s",
			OutputFormat:   "json",
			SignMode:       "direct",
			ClientType:     clientType,
			HttpAddr:       apiAddr,
			DymHub:         isHub,
			DymRollapp:     isRA,
			TrustPeriod:    390 * time.Second,
		},
	}
}

// commander satisfies relayer.RelayerCommander.
type commander struct {
	log             *zap.Logger
	extraStartFlags []string
}

func (commander) Name() string {
	return "rly"
}

func (commander) DockerUser() string {
	return RlyDefaultUidGid // docker run -it --rm --entrypoint echo ghcr.io/cosmos/relayer "$(id -u):$(id -g)"
}

func (commander) AddChainConfiguration(containerFilePath, homeDir string) []string {
	return []string{
		"rly", "chains", "add", "-f", containerFilePath,
		"--home", homeDir,
	}
}

func (commander) AddKey(chainID, keyName, coinType, homeDir string) []string {
	return []string{
		"rly", "keys", "add", chainID, keyName,
		"--coin-type", fmt.Sprint(coinType), "--home", homeDir,
	}
}

func (commander) CreateChannel(pathName string, opts ibc.CreateChannelOptions, homeDir string) []string {
	return []string{
		"rly", "tx", "channel", pathName,
		"--src-port", opts.SourcePortName,
		"--dst-port", opts.DestPortName,
		"--order", opts.Order.String(),
		"--version", opts.Version,
		"--max-retries", "30", "--timeout", "40s", "--debug",
		"--home", homeDir,
	}
}

func (commander) CreateClients(pathName string, opts ibc.CreateClientOptions, homeDir string) []string {
	return []string{
		"rly", "tx", "clients", pathName, "--max-clock-drift", "70m",
		"--home", homeDir,
	}
}

// passing a value of 0 for customeClientTrustingPeriod will use default
func (commander) CreateClient(pathName, homeDir, customeClientTrustingPeriod string) []string {
	return []string{
		"rly", "tx", "client", pathName,
		"--home", homeDir,
	}
}

func (commander) CreateConnections(pathName string, homeDir string) []string {
	return []string{
		"rly", "tx", "connection", pathName, "--max-retries", "30", "--timeout", "40s", "--debug",
		"--home", homeDir,
	}
}

func (commander) CreateConnectionsWithNumberOfRetries(pathName string, homeDir string, retries string) []string {
	return []string{
		"rly", "tx", "connection", pathName, "--max-retries", retries, "--timeout", "40s", "--debug",
		"--home", homeDir,
	}
}

func (commander) Flush(pathName, channelID, homeDir string) []string {
	cmd := []string{"rly", "tx", "flush"}
	if pathName != "" {
		cmd = append(cmd, pathName)
		if channelID != "" {
			cmd = append(cmd, channelID)
		}
	}
	cmd = append(cmd, "--home", homeDir)
	return cmd
}

func (commander) GeneratePath(srcChainID, dstChainID, pathName, homeDir string) []string {
	return []string{
		"rly", "paths", "new", srcChainID, dstChainID, pathName,
		"--home", homeDir,
	}
}

func (commander) UpdatePath(pathName, homeDir string, filter ibc.ChannelFilter) []string {
	return []string{
		"rly", "paths", "update", pathName,
		"--home", homeDir,
		"--filter-rule", filter.Rule,
		"--filter-channels", strings.Join(filter.ChannelList, ","),
	}
}

func (commander) GetChannels(chainID, homeDir string) []string {
	return []string{
		"rly", "q", "channels", chainID,
		"--home", homeDir,
	}
}

func (commander) GetConnections(chainID, homeDir string) []string {
	return []string{
		"rly", "q", "connections", chainID,
		"--home", homeDir,
	}
}

func (commander) GetClients(chainID, homeDir string) []string {
	return []string{
		"rly", "q", "clients", chainID,
		"--home", homeDir,
	}
}

func (commander) LinkPath(pathName, homeDir string, channelOpts ibc.CreateChannelOptions, clientOpt ibc.CreateClientOptions) []string {
	return []string{
		"rly", "tx", "link", pathName,
		"--src-port", channelOpts.SourcePortName,
		"--dst-port", channelOpts.DestPortName,
		"--order", channelOpts.Order.String(),
		"--version", channelOpts.Version,
		"--client-tp", clientOpt.TrustingPeriod,
		"--debug",

		"--home", homeDir,
	}
}

func (commander) RestoreKey(chainID, keyName, coinType, mnemonic, homeDir string) []string {
	return []string{
		"rly", "keys", "restore", chainID, keyName, mnemonic,
		"--coin-type", fmt.Sprint(coinType), "--home", homeDir,
	}
}

func (c commander) StartRelayer(homeDir string, pathNames ...string) []string {
	cmd := []string{
		"rly", "start", "--debug",
		"--home", homeDir,
	}
	cmd = append(cmd, c.extraStartFlags...)
	cmd = append(cmd, pathNames...)
	return cmd
}

func (commander) UpdateClients(pathName, homeDir string) []string {
	return []string{
		"rly", "tx", "update-clients", pathName,
		"--home", homeDir,
	}
}

func (commander) ConfigContent(ctx context.Context, cfg ibc.ChainConfig, keyName, rpcAddr, grpcAddr, apiAddr string) ([]byte, error) {
	cosmosRelayerChainConfig := ConfigToCosmosRelayerChainConfig(cfg, keyName, rpcAddr, apiAddr)

	jsonBytes, err := json.Marshal(cosmosRelayerChainConfig)
	if err != nil {
		return nil, err
	}
	return jsonBytes, nil
}

func (commander) DefaultContainerImage() string {
	return DefaultContainerImage
}

func (commander) DefaultContainerVersion() string {
	return DefaultContainerVersion
}

func (commander) ParseAddKeyOutput(stdout, stderr string) (ibc.Wallet, error) {
	var wallet WalletModel
	err := json.Unmarshal([]byte(stdout), &wallet)
	rlyWallet := NewWallet("", wallet.Address, wallet.Mnemonic)
	return rlyWallet, err
}

func (commander) ParseRestoreKeyOutput(stdout, stderr string) string {
	return strings.Replace(stdout, "\n", "", 1)
}

func (c commander) ParseGetChannelsOutput(stdout, stderr string) ([]ibc.ChannelOutput, error) {
	var channels []ibc.ChannelOutput
	channelSplit := strings.Split(stdout, "\n")
	for _, channel := range channelSplit {
		if strings.TrimSpace(channel) == "" {
			continue
		}
		var channelOutput ibc.ChannelOutput
		err := json.Unmarshal([]byte(channel), &channelOutput)
		if err != nil {
			c.log.Error("Failed to parse channels json", zap.Error(err))
			continue
		}
		channels = append(channels, channelOutput)
	}

	return channels, nil
}

func (c commander) ParseGetConnectionsOutput(stdout, stderr string) (ibc.ConnectionOutputs, error) {
	var connections ibc.ConnectionOutputs
	for _, connection := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(connection) == "" {
			continue
		}

		var connectionOutput ibc.ConnectionOutput
		if err := json.Unmarshal([]byte(connection), &connectionOutput); err != nil {
			c.log.Error(
				"Error parsing connection json",
				zap.Error(err),
			)

			continue
		}
		connections = append(connections, &connectionOutput)
	}

	return connections, nil
}

func (c commander) ParseGetClientsOutput(stdout, stderr string) (ibc.ClientOutputs, error) {
	var clients ibc.ClientOutputs
	for _, client := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(client) == "" {
			continue
		}

		var clientOutput ibc.ClientOutput
		if err := json.Unmarshal([]byte(client), &clientOutput); err != nil {
			c.log.Error(
				"Error parsing client json",
				zap.Error(err),
			)

			continue
		}
		clients = append(clients, &clientOutput)
	}

	return clients, nil
}

func (commander) Init(homeDir string) []string {
	return []string{
		"rly", "config", "init",
		"--home", homeDir,
	}
}

func (c commander) CreateWallet(keyName, address, mnemonic string) ibc.Wallet {
	return NewWallet(keyName, address, mnemonic)
}
