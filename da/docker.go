package da

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/decentrio/rollup-e2e-testing/dockerutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/docker/docker/api/types"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

const (
	defaultDANodeHomeDirectory = "/home/da"
)

type DAChainEndPoint interface {
	GetRPCAddress() string
	GetGRPCAddress() string
}

type DANodeCommand interface {
	Init(string) []string
	Start() []string
}

type DockerDANode struct {
	log *zap.Logger

	c DANodeCommand

	networkID  string
	client     *client.Client
	volumeName string
	testName   string

	customImage *ibc.DockerImage
	pullImage   bool

	endpointRpcAddress  string
	endPointGrpcAddress string

	// The ID of the container created by StartRelayer.
	containerLifecycle *dockerutil.ContainerLifecycle

	homeDir string
}

// TODO: Add container
const (
	DefaultContainerImage   = ""
	DefaultContainerVersion = ""
	DefaultUidGid           = ""
)

func DefaultDANodeContainerImage() string {
	return DefaultContainerImage
}

func DefaultDANodeContainerVersion() string {
	return DefaultContainerVersion
}

func DockerDANodeUser() string {
	return DefaultUidGid
}

func NewDockerDANode(ctx context.Context, log *zap.Logger, testName string, cli *client.Client, networkID string, c DANodeCommand, endpoint DAChainEndPoint) (*DockerDANode, error) {
	daNode := DockerDANode{
		log:                 log,
		c:                   c,
		networkID:           networkID,
		client:              cli,
		testName:            testName,
		endpointRpcAddress:  endpoint.GetRPCAddress(),
		endPointGrpcAddress: endpoint.GetGRPCAddress(),
		pullImage:           true,
	}

	daNode.homeDir = defaultDANodeHomeDirectory

	containerImage := daNode.containerImage()
	if err := daNode.pullContainerImageIfNecessary(containerImage); err != nil {
		return nil, fmt.Errorf("pulling container image %s: %w", containerImage.Ref(), err)
	}

	v, err := cli.VolumeCreate(ctx, volumetypes.CreateOptions{
		// Have to leave Driver unspecified for Docker Desktop compatibility.

		Labels: map[string]string{dockerutil.CleanupLabel: testName},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume: %w", err)
	}
	daNode.volumeName = v.Name

	// The volume is created owned by root,
	// but we configure the relayer to run as a non-root user,
	// so set the node home (where the volume is mounted) to be owned
	// by the relayer user.
	if err := dockerutil.SetVolumeOwner(ctx, dockerutil.VolumeOwnerOptions{
		Log: daNode.log,

		Client: daNode.client,

		VolumeName: daNode.volumeName,
		ImageRef:   containerImage.Ref(),
		TestName:   testName,
		UidGid:     containerImage.UidGid,
	}); err != nil {
		return nil, fmt.Errorf("set volume owner: %w", err)
	}

	daNode.Init(daNode.HomeDir())
	// Initialization should complete immediately,
	// but add a 1-minute timeout in case Docker hangs on a developer workstation.
	_, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	return &daNode, nil
}

func (daNode *DockerDANode) HomeDir() string {
	return daNode.homeDir
}

func (daNode *DockerDANode) Init(homeDir string) {
	daNode.c.Init(homeDir)
}

func (daNode *DockerDANode) Start() {
	// start DA Node with rpc endpoint
}

func (daNode *DockerDANode) containerImage() ibc.DockerImage {
	if daNode.customImage != nil {
		return *daNode.customImage
	}
	return ibc.DockerImage{
		Repository: DefaultDANodeContainerImage(),
		Version:    DefaultDANodeContainerVersion(),
		UidGid:     DockerDANodeUser(),
	}
}

func (daNode *DockerDANode) pullContainerImageIfNecessary(containerImage ibc.DockerImage) error {
	if !daNode.pullImage {
		return nil
	}

	rc, err := daNode.client.ImagePull(context.TODO(), containerImage.Ref(), types.ImagePullOptions{})
	if err != nil {
		return err
	}

	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
	return nil
}
