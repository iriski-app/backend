package ml

import (
	"context"
	"net"
	"testing"
	"time"

	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testServer struct {
	mlv1.UnimplementedMLServiceServer
}

func (s *testServer) AnalyzeScenario(_ context.Context, req *mlv1.AnalyzeScenarioRequest) (*mlv1.AnalyzeScenarioResponse, error) {
	return &mlv1.AnalyzeScenarioResponse{RequestId: req.RequestId, CalculationId: req.CalculationId}, nil
}

func (s *testServer) ExplainCalculation(_ context.Context, req *mlv1.ExplainCalculationRequest) (*mlv1.ExplainCalculationResponse, error) {
	return &mlv1.ExplainCalculationResponse{RequestId: req.RequestId, CalculationId: req.CalculationId, Text: req.TotalRub}, nil
}

func (s *testServer) GetReadiness(context.Context, *mlv1.ReadinessRequest) (*mlv1.ReadinessResponse, error) {
	return &mlv1.ReadinessResponse{MetricReady: true}, nil
}

func (s *testServer) EstimateMissingMetric(ctx context.Context, req *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
	if req.RequestId == "timeout" {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if req.Features == nil || req.Features.YearsToMaturity == nil || *req.Features.YearsToMaturity != 0 || req.Features.IsGovernment == nil || *req.Features.IsGovernment || req.Features.CouponPercent != nil {
		return nil, status.Error(codes.InvalidArgument, "optional fields lost their presence")
	}
	return &mlv1.EstimateMissingMetricResponse{RequestId: req.RequestId, Value: 1.25}, nil
}

func TestClientGRPC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	mlv1.RegisterMLServiceServer(server, &testServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	client, err := NewClient(listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ready, err := client.GetReadiness(context.Background())
	if err != nil || !ready.GetMetricReady() {
		t.Fatalf("readiness=%v error=%v", ready, err)
	}
	zero, no := 0.0, false
	response, err := client.EstimateMissingMetric(context.Background(), &mlv1.EstimateMissingMetricRequest{RequestId: "test", Features: &mlv1.BondFeatures{YearsToMaturity: &zero, IsGovernment: &no}})
	if err != nil || response.GetRequestId() != "test" || response.GetValue() != 1.25 {
		t.Fatalf("response=%v error=%v", response, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	scenario, err := client.AnalyzeScenario(context.Background(), &mlv1.AnalyzeScenarioRequest{RequestId: "r", CalculationId: "c", ShiftBp: 100})
	if err != nil || scenario.CalculationId != "c" {
		t.Fatalf("scenario=%v err=%v", scenario, err)
	}
	explanation, err := client.ExplainCalculation(context.Background(), &mlv1.ExplainCalculationRequest{RequestId: "r", CalculationId: "c", TotalRub: "1.25"})
	if err != nil || explanation.Text != "1.25" {
		t.Fatalf("explanation=%v err=%v", explanation, err)
	}
	_, err = client.EstimateMissingMetric(ctx, &mlv1.EstimateMissingMetricRequest{RequestId: "timeout"})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
