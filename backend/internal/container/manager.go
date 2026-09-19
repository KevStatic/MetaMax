package container

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	dockerclient "github.com/moby/moby/client"
)

type PackageManager string

const (
	PackageManagerNPM PackageManager = "npm"
	PackageManagerPIP PackageManager = "pip"
	PackageManagerAPT PackageManager = "apt"
)

type IDEType string

const (
	IDEVSCode  IDEType = "vscode"
	IDEJupyter IDEType = "jupyter"
)

type DBType string

const (
	DBPostgres DBType = "postgres"
	DBMongo    DBType = "mongo"
	DBRedis    DBType = "redis"
	DBMySQL    DBType = "mysql"
)

// CreateOpts describes a container to create.
type CreateOpts struct {
	TeamID    string
	SessionID string
	Name      string
	Image     string
	RAMMb     int64
	CPUCores  float64
	Ports     []string
	VaultKey  string
}

// ContainerInfo is returned after a container is created.
type ContainerInfo struct {
	ID          string
	Name        string
	Status      string
	Ports       map[string]string
	StoragePath string
}

// HealthStatus reports whether a container is running.
type HealthStatus struct {
	Running bool
	Status  string
}

// Manager wraps the Docker client with LUKS and port-registry helpers.
type Manager struct {
	client     *dockerclient.Client
	storageMu  sync.RWMutex
	storageReg map[string]string
	deployMu   sync.RWMutex
	deployReg  map[string]map[string]string
}

// NewManager creates a new Manager connected to the Docker daemon.
func NewManager(host string) (*Manager, error) {
	var (
		cli *dockerclient.Client
		err error
	)
	if host == "" || host == "unix:///var/run/docker.sock" {
		cli, err = dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	} else {
		cli, err = dockerclient.NewClientWithOpts(
			dockerclient.WithHost(host),
			dockerclient.WithAPIVersionNegotiation(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Manager{
		client:     cli,
		storageReg: make(map[string]string),
		deployReg:  make(map[string]map[string]string),
	}, nil
}

func (m *Manager) RegisterDeploy(containerID string, ports map[string]string) {
	m.deployMu.Lock()
	defer m.deployMu.Unlock()
	m.deployReg[containerID] = ports
}

func (m *Manager) LookupDeployPort(containerID string) (string, bool) {
	m.deployMu.RLock()
	defer m.deployMu.RUnlock()
	ports, ok := m.deployReg[containerID]
	if !ok {
		return "", false
	}
	for _, hp := range ports {
		if hp != "" {
			return hp, true
		}
	}
	return "", false
}

func (m *Manager) CreateContainer(ctx context.Context, opts CreateOpts) (*ContainerInfo, error) {
	pull, err := m.client.ImagePull(ctx, opts.Image, image.PullOptions{})
	if err != nil {
		return nil, fmt.Errorf("pull image %s: %w", opts.Image, err)
	}
	io.Copy(io.Discard, pull)
	pull.Close()

	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}

	for _, p := range opts.Ports {
		portStr := strings.TrimSuffix(p, "/tcp")
		port, err := nat.NewPort("tcp", portStr)
		if err != nil {
			return nil, fmt.Errorf("parse port %s: %w", p, err)
		}
		exposedPorts[port] = struct{}{}
		hostPort := port.Port()
		if !isPortAvailable(hostPort) {
			hostPort = ""
		}
		portBindings[port] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: hostPort}}
	}

	storageDir := fmt.Sprintf("/vm-storage/containers/%s-%s-%s", opts.TeamID, opts.Name, randomHex(6))
	if err := os.MkdirAll(storageDir, 0o700); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}

	appPath, luksErr := setupLUKSHome(storageDir, opts.VaultKey)
	if luksErr != nil {
		log.Printf("[container] LUKS setup failed (%v) — falling back to unencrypted /app", luksErr)
		appPath = storageDir + "/app"
		if err := os.MkdirAll(appPath, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir fallback app dir: %w", err)
		}
	}

	containerName := fmt.Sprintf("comput3-%s-%s", opts.TeamID, opts.Name)
	resp, err := m.client.ContainerCreate(
		ctx,
		&container.Config{
			Image:        opts.Image,
			ExposedPorts: exposedPorts,
			Cmd:          []string{"sh", "-c", "tail -f /dev/null"},
			Labels: map[string]string{
				"comput3.team":      opts.TeamID,
				"comput3.session":   opts.SessionID,
				"comput3.name":      opts.Name,
				"comput3.encrypted": fmt.Sprintf("%v", luksErr == nil),
			},
		},
		&container.HostConfig{
			PortBindings: portBindings,
			Binds:        []string{appPath + ":/app"},
			Resources: container.Resources{
				Memory:   opts.RAMMb * 1024 * 1024,
				NanoCPUs: int64(opts.CPUCores * 1e9),
			},
		},
		nil,
		nil,
		containerName,
	)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}

	if err := m.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}

	inspect, _ := m.client.ContainerInspect(ctx, resp.ID)
	ports := make(map[string]string)
	if inspect.NetworkSettings != nil {
		for cPort, bindings := range inspect.NetworkSettings.Ports {
			for _, b := range bindings {
				if b.HostPort != "" {
					ports[string(cPort)] = b.HostPort
				}
			}
		}
	}

	shortID := resp.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	m.storageMu.Lock()
	m.storageReg[shortID] = storageDir
	m.storageMu.Unlock()

	log.Printf("[container] created %s encrypted=%v", shortID, luksErr == nil)
	return &ContainerInfo{
		ID:          shortID,
		Name:        opts.Name,
		Status:      "running",
		Ports:       ports,
		StoragePath: storageDir,
	}, nil
}

func (m *Manager) InstallPackages(ctx context.Context, containerID string, packages []string, mgr PackageManager) error {
	var cmd []string
	switch mgr {
	case PackageManagerNPM:
		cmd = append([]string{"npm", "install", "-g"}, packages...)
	case PackageManagerPIP:
		cmd = append([]string{"pip", "install"}, packages...)
	case PackageManagerAPT:
		cmd = []string{"sh", "-c", "apt-get update -qq && apt-get install -y -qq " + strings.Join(packages, " ")}
	default:
		return fmt.Errorf("unknown package manager: %s", mgr)
	}
	return m.exec(ctx, containerID, cmd)
}

func (m *Manager) CreateNetwork(ctx context.Context, teamID string) error {
	name := "comput3-" + teamID
	nets, err := m.client.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}
	for _, n := range nets {
		if n.Name == name {
			return nil
		}
	}
	_, err = m.client.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{"comput3.team": teamID},
	})
	return err
}

func (m *Manager) ConnectContainers(ctx context.Context, teamID string, containerIDs []string) error {
	netName := "comput3-" + teamID
	for _, id := range containerIDs {
		err := m.client.NetworkConnect(ctx, netName, id, &network.EndpointSettings{})
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("connect %s: %w", id, err)
		}
	}
	return nil
}

func (m *Manager) SetupIDE(ctx context.Context, containerID string, ideType IDEType) error {
	switch ideType {
	case IDEVSCode:
		return m.exec(ctx, containerID, []string{"sh", "-c",
			"curl -fsSL https://code-server.dev/install.sh | sh && code-server --bind-addr 0.0.0.0:8443 --auth none &"})
	case IDEJupyter:
		if err := m.exec(ctx, containerID, []string{"pip", "install", "jupyterlab"}); err != nil {
			return err
		}
		return m.exec(ctx, containerID, []string{"sh", "-c",
			"jupyter lab --ip=0.0.0.0 --port=8888 --no-browser --allow-root &"})
	default:
		return fmt.Errorf("unknown IDE type: %s", ideType)
	}
}

func (m *Manager) SetupDatabase(ctx context.Context, teamID, sessionID string, dbType DBType, version string) (*ContainerInfo, error) {
	imageMap := map[DBType]string{
		DBPostgres: "postgres",
		DBMongo:    "mongo",
		DBRedis:    "redis",
		DBMySQL:    "mysql",
	}
	portMap := map[DBType]string{
		DBPostgres: "5432/tcp",
		DBMongo:    "27017/tcp",
		DBRedis:    "6379/tcp",
		DBMySQL:    "3306/tcp",
	}
	img := imageMap[dbType]
	if img == "" {
		return nil, fmt.Errorf("unsupported db type: %s", dbType)
	}
	if version != "" {
		img = img + ":" + version
	}
	return m.CreateContainer(ctx, CreateOpts{
		TeamID:    teamID,
		SessionID: sessionID,
		Name:      string(dbType),
		Image:     img,
		RAMMb:     512,
		CPUCores:  0.5,
		Ports:     []string{portMap[dbType]},
	})
}

func (m *Manager) HealthCheck(ctx context.Context, containerID string) (*HealthStatus, error) {
	result, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}
	if result.State == nil {
		return &HealthStatus{Running: false, Status: "unknown"}, nil
	}
	return &HealthStatus{
		Running: result.State.Running,
		Status:  result.State.Status,
	}, nil
}

func (m *Manager) GetLogs(ctx context.Context, containerID string, lines int) (string, error) {
	reader, err := m.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", lines),
	})
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}
	defer reader.Close()
	var sb strings.Builder
	io.Copy(&sb, reader)
	return sb.String(), nil
}

func (m *Manager) Destroy(ctx context.Context, containerID string) error {
	timeout := 10
	if err := m.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	if err := m.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	m.storageMu.Lock()
	storageDir, ok := m.storageReg[containerID]
	if ok {
		delete(m.storageReg, containerID)
	}
	m.storageMu.Unlock()
	if ok {
		teardownLUKSHome(storageDir)
		if err := os.RemoveAll(storageDir); err != nil {
			log.Printf("[container] cleanup storageDir %s: %v", storageDir, err)
		}
	}
	return nil
}

func (m *Manager) CloneRepo(ctx context.Context, containerID, repoURL, dir string) (string, error) {
	if dir == "" {
		dir = "/app"
	}
	_ = m.exec(ctx, containerID, []string{"sh", "-c",
		"which git > /dev/null 2>&1 || (apt-get update -qq && apt-get install -y -qq git)"})
	return m.execWithOutput(ctx, containerID, []string{"git", "clone", "--depth=1", repoURL, dir}, "/", nil)
}

func (m *Manager) RunCommand(ctx context.Context, containerID, shellCmd, workDir string, env map[string]string) (string, error) {
	var envSlice []string
	for k, v := range env {
		envSlice = append(envSlice, k+"="+v)
	}
	return m.execWithOutput(ctx, containerID, []string{"sh", "-c", shellCmd}, workDir, envSlice)
}

func (m *Manager) StartProcess(ctx context.Context, containerID, shellCmd, workDir string, env map[string]string) (string, error) {
	var envSlice []string
	for k, v := range env {
		envSlice = append(envSlice, k+"="+v)
	}
	wrapped := fmt.Sprintf("nohup sh -c %q > /proc/1/fd/1 2>&1 &", shellCmd)
	return m.execWithOutput(ctx, containerID, []string{"sh", "-c", wrapped}, workDir, envSlice)
}

func (m *Manager) WriteFile(ctx context.Context, containerID, path, content string) error {
	fname := filepath.Base(path)
	dir := filepath.ToSlash(filepath.Dir(path))

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:    fname,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		return err
	}
	tw.Close()

	return m.client.CopyToContainer(ctx, containerID, dir, &buf, container.CopyToContainerOptions{})
}

func (m *Manager) execWithOutput(ctx context.Context, containerID string, cmd []string, workDir string, env []string) (string, error) {
	execID, err := m.client.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   workDir,
		Env:          env,
	})
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}

	attach, err := m.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("exec attach: %w", err)
	}
	defer attach.Close()

	var sb strings.Builder
	io.Copy(&sb, attach.Reader)

	for {
		inspect, err := m.client.ContainerExecInspect(ctx, execID.ID)
		if err != nil {
			return sb.String(), err
		}
		if !inspect.Running {
			if inspect.ExitCode != 0 {
				return sb.String(), fmt.Errorf("exited %d: %s", inspect.ExitCode, sb.String())
			}
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	return sb.String(), nil
}

func (m *Manager) exec(ctx context.Context, containerID string, cmd []string) error {
	execID, err := m.client.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}
	attach, err := m.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("exec attach: %w", err)
	}
	defer attach.Close()
	io.Copy(io.Discard, attach.Reader)
	return nil
}

func isPortAvailable(port string) bool {
	if port == "" {
		return false
	}
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
