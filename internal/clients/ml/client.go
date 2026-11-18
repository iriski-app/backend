package ml

import (
	"context"
	"errors"
	"time"

	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client connects to the ML service over an internal, unencrypted gRPC connection.
type Client struct {
	conn    *grpc.ClientConn
	service mlv1.MLServiceClient
	timeout time.Duration
}

func NewClient(address string, timeout time.Duration) (*Client, error) {
	if address == "" {
		return nil, errors.New("ML service address is required")
	}
	if timeout <= 0 {
		return nil, errors.New("ML service timeout must be positive")
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, service: mlv1.NewMLServiceClient(conn), timeout: timeout}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) GetReadiness(ctx context.Context) (*mlv1.ReadinessResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.service.GetReadiness(ctx, &mlv1.ReadinessRequest{})
}

func (c *Client) EstimateMissingMetric(ctx context.Context, req *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.service.EstimateMissingMetric(ctx, req)
}
