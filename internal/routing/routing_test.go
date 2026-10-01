package routing

import (
	"testing"

	"github.com/yourusername/cdn-simulator/internal/config"
	"github.com/yourusername/cdn-simulator/pkg/types"
)

func newTestRouter() *LatencyRouter {
	return NewLatencyRouter(&config.RouterConfig{
		Strategy:            "latency",
		HealthCheckInterval: 10,
	})
}

func TestSelectEdge_NoEdges(t *testing.T) {
	r := newTestRouter()

	_, err := r.SelectEdge(&types.Request{})
	if err != ErrNoHealthyEdge {
		t.Fatalf("expected ErrNoHealthyEdge, got %v", err)
	}
}

func TestSelectEdge_LeastLoaded(t *testing.T) {
	r := newTestRouter()
	r.RegisterEdge(&types.EdgeNode{ID: "busy", Address: "127.0.0.1:1", Healthy: true, Capacity: 100, Load: 90})
	r.RegisterEdge(&types.EdgeNode{ID: "free", Address: "127.0.0.1:2", Healthy: true, Capacity: 100, Load: 10})

	edge, err := r.SelectEdge(&types.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if edge.ID != "free" {
		t.Fatalf("expected least loaded edge 'free', got %q", edge.ID)
	}
}

func TestSelectEdge_SkipsUnhealthy(t *testing.T) {
	r := newTestRouter()
	r.RegisterEdge(&types.EdgeNode{ID: "down", Address: "127.0.0.1:1", Healthy: false, Capacity: 100})
	r.RegisterEdge(&types.EdgeNode{ID: "up", Address: "127.0.0.1:2", Healthy: true, Capacity: 100})

	edge, err := r.SelectEdge(&types.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if edge.ID != "up" {
		t.Fatalf("expected healthy edge 'up', got %q", edge.ID)
	}
}

func TestSelectEdge_SkipsFullCapacity(t *testing.T) {
	r := newTestRouter()
	r.RegisterEdge(&types.EdgeNode{ID: "full", Address: "127.0.0.1:1", Healthy: true, Capacity: 10, Load: 10})
	r.RegisterEdge(&types.EdgeNode{ID: "open", Address: "127.0.0.1:2", Healthy: true, Capacity: 10, Load: 0})

	edge, err := r.SelectEdge(&types.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if edge.ID != "open" {
		t.Fatalf("expected edge 'open', got %q", edge.ID)
	}
}

func TestSelectEdge_RegionPreference(t *testing.T) {
	r := newTestRouter()
	r.RegisterEdge(&types.EdgeNode{ID: "eu", Region: "eu-west-1", Address: "127.0.0.1:1", Healthy: true, Capacity: 100, Load: 50})
	r.RegisterEdge(&types.EdgeNode{ID: "us", Region: "us-east-1", Address: "127.0.0.1:2", Healthy: true, Capacity: 100, Load: 0})

	edge, err := r.SelectEdge(&types.Request{Region: "eu-west-1"})
	if err != nil {
		t.Fatal(err)
	}
	if edge.ID != "eu" {
		t.Fatalf("expected region match 'eu', got %q", edge.ID)
	}
}

func TestSelectOrigin_NoHealthy(t *testing.T) {
	r := newTestRouter()

	_, err := r.SelectOrigin(&types.Request{})
	if err != ErrNoHealthyOrigin {
		t.Fatalf("expected ErrNoHealthyOrigin, got %v", err)
	}
}

func TestSelectOrigin_OnlyHealthyReturned(t *testing.T) {
	r := newTestRouter()
	r.RegisterOrigin(&types.OriginServer{ID: "down", Address: "127.0.0.1:1", Healthy: false, Weight: 1})
	r.RegisterOrigin(&types.OriginServer{ID: "up", Address: "127.0.0.1:2", Healthy: true, Weight: 1})

	origin, err := r.SelectOrigin(&types.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if origin.ID != "up" {
		t.Fatalf("expected origin 'up', got %q", origin.ID)
	}
}

func TestSelectOrigin_ZeroWeightFallsBackToRandom(t *testing.T) {
	r := newTestRouter()
	r.RegisterOrigin(&types.OriginServer{ID: "a", Address: "127.0.0.1:1", Healthy: true, Weight: 0})
	r.RegisterOrigin(&types.OriginServer{ID: "b", Address: "127.0.0.1:2", Healthy: true, Weight: 0})

	for i := 0; i < 20; i++ {
		origin, err := r.SelectOrigin(&types.Request{})
		if err != nil {
			t.Fatal(err)
		}
		if origin.ID != "a" && origin.ID != "b" {
			t.Fatalf("unexpected origin %q", origin.ID)
		}
	}
}

func TestRegisterDeregister(t *testing.T) {
	r := newTestRouter()
	r.RegisterEdge(&types.EdgeNode{ID: "e1", Address: "127.0.0.1:1", Healthy: true, Capacity: 1})
	r.RegisterOrigin(&types.OriginServer{ID: "o1", Address: "127.0.0.1:2", Healthy: true})

	if len(r.GetHealthyEdges()) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(r.GetHealthyEdges()))
	}
	if len(r.GetHealthyOrigins()) != 1 {
		t.Fatalf("expected 1 origin, got %d", len(r.GetHealthyOrigins()))
	}

	r.DeregisterEdge("e1")
	r.DeregisterOrigin("o1")

	if len(r.GetHealthyEdges()) != 0 {
		t.Fatalf("expected 0 edges after deregister")
	}
	if len(r.GetHealthyOrigins()) != 0 {
		t.Fatalf("expected 0 origins after deregister")
	}
}

func TestStartHealthChecks_ZeroIntervalUsesDefault(t *testing.T) {
	r := newTestRouter()
	r.RegisterEdge(&types.EdgeNode{ID: "e1", Address: "127.0.0.1:1", Healthy: true, Capacity: 1})

	r.StartHealthChecks(0)
	t.Cleanup(func() {})
}