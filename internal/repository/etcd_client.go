package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/kirychukyurii/webitel-dc-switcher/internal/config"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/model"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/util"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	// etcd key prefixes
	keyActiveDatacenter = "dc-switcher/active-datacenter"
	keyHeartbeatPrefix  = "dc-switcher/heartbeats/"
)

// EtcdRepository defines the interface for etcd operations
type EtcdRepository interface {
	// WriteActiveDatacenter writes the active datacenter information to etcd
	WriteActiveDatacenter(ctx context.Context, info *model.ActiveDatacenter) error

	// ReadActiveDatacenter reads the active datacenter information from etcd
	ReadActiveDatacenter(ctx context.Context) (*model.ActiveDatacenter, error)

	// WriteHeartbeat writes heartbeat for a specific datacenter
	WriteHeartbeat(ctx context.Context, datacenter string) error

	// ReadHeartbeat reads heartbeat for a specific datacenter
	ReadHeartbeat(ctx context.Context, datacenter string) (*model.HeartbeatInfo, error)

	// Close closes the etcd client connection
	Close() error
}

// etcdClient implements EtcdRepository
type etcdClient struct {
	client *clientv3.Client
	logger *slog.Logger
}

// NewEtcdRepository creates a new etcd repository
func NewEtcdRepository(cfg config.EtcdConfig, logger *slog.Logger) (EtcdRepository, error) {
	etcdCfg := clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
		Username:    cfg.Username,
		Password:    cfg.Password,
	}

	// Configure TLS if provided
	if cfg.TLS != nil {
		tlsConfig, err := util.LoadTLSConfig(cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}
		etcdCfg.TLS = tlsConfig
	}

	client, err := clientv3.New(etcdCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Status(ctx, cfg.Endpoints[0])
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	logger.Info("Connected to etcd cluster", "endpoints", cfg.Endpoints)

	return &etcdClient{
		client: client,
		logger: logger,
	}, nil
}

// WriteActiveDatacenter writes the active datacenter information to etcd
func (e *etcdClient) WriteActiveDatacenter(ctx context.Context, info *model.ActiveDatacenter) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal active datacenter info: %w", err)
	}

	_, err = e.client.Put(ctx, keyActiveDatacenter, string(data))
	if err != nil {
		return fmt.Errorf("failed to write active datacenter to etcd: %w", err)
	}

	e.logger.Debug("Wrote active datacenter to etcd",
		"datacenter", info.Datacenter,
		"last_heartbeat", info.LastHeartbeat)

	return nil
}

// ReadActiveDatacenter reads the active datacenter information from etcd
func (e *etcdClient) ReadActiveDatacenter(ctx context.Context) (*model.ActiveDatacenter, error) {
	resp, err := e.client.Get(ctx, keyActiveDatacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to read active datacenter from etcd: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("no active datacenter found in etcd")
	}

	var info model.ActiveDatacenter
	if err := json.Unmarshal(resp.Kvs[0].Value, &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal active datacenter info: %w", err)
	}

	return &info, nil
}

// WriteHeartbeat writes heartbeat for a specific datacenter
func (e *etcdClient) WriteHeartbeat(ctx context.Context, datacenter string) error {
	heartbeat := model.HeartbeatInfo{
		Datacenter: datacenter,
		LastSeen:   time.Now(),
	}

	data, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat info: %w", err)
	}

	key := keyHeartbeatPrefix + datacenter
	_, err = e.client.Put(ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to write heartbeat to etcd: %w", err)
	}

	e.logger.Debug("Wrote heartbeat to etcd", "datacenter", datacenter)

	return nil
}

// ReadHeartbeat reads heartbeat for a specific datacenter
func (e *etcdClient) ReadHeartbeat(ctx context.Context, datacenter string) (*model.HeartbeatInfo, error) {
	key := keyHeartbeatPrefix + datacenter
	resp, err := e.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to read heartbeat from etcd: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("no heartbeat found for datacenter %s", datacenter)
	}

	var heartbeat model.HeartbeatInfo
	if err := json.Unmarshal(resp.Kvs[0].Value, &heartbeat); err != nil {
		return nil, fmt.Errorf("failed to unmarshal heartbeat info: %w", err)
	}

	return &heartbeat, nil
}

// Close closes the etcd client connection
func (e *etcdClient) Close() error {
	if e.client != nil {
		return e.client.Close()
	}
	return nil
}
