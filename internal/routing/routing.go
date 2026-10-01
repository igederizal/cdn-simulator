package routing

import (
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/yourusername/cdn-simulator/internal/config"
	"github.com/yourusername/cdn-simulator/pkg/types"
)

type Router interface {
	SelectEdge(req *types.Request) (*types.EdgeNode, error)
	SelectOrigin(req *types.Request) (*types.OriginServer, error)
	RegisterEdge(node *types.EdgeNode)
	RegisterOrigin(server *types.OriginServer)
	DeregisterEdge(id string)
	DeregisterOrigin(id string)
	GetHealthyEdges() []*types.EdgeNode
	GetHealthyOrigins() []*types.OriginServer
}

type LatencyRouter struct {
	edges    map[string]*types.EdgeNode
	origins  map[string]*types.OriginServer
	mu       sync.RWMutex
	config   *config.RouterConfig
	latencies map[string]map[string]int64
}

func NewLatencyRouter(cfg *config.RouterConfig) *LatencyRouter {
	return &LatencyRouter{
		edges:     make(map[string]*types.EdgeNode),
		origins:   make(map[string]*types.OriginServer),
		config:    cfg,
		latencies: make(map[string]map[string]int64),
	}
}

func (r *LatencyRouter) RegisterEdge(node *types.EdgeNode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.edges[node.ID] = node
	r.initLatencies(node.ID)
}

func (r *LatencyRouter) RegisterOrigin(server *types.OriginServer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.origins[server.ID] = server
}

func (r *LatencyRouter) DeregisterEdge(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.edges, id)
	delete(r.latencies, id)
}

func (r *LatencyRouter) DeregisterOrigin(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.origins, id)
}

func (r *LatencyRouter) initLatencies(edgeID string) {
	r.latencies[edgeID] = make(map[string]int64)
	for originID := range r.origins {
		r.latencies[edgeID][originID] = rand.Int63n(100) + 10
	}
}

func (r *LatencyRouter) SelectEdge(req *types.Request) (*types.EdgeNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*types.EdgeNode
	for _, edge := range r.edges {
		if edge.Healthy && edge.Load < edge.Capacity {
			candidates = append(candidates, edge)
		}
	}

	if len(candidates) == 0 {
		return nil, ErrNoHealthyEdge
	}

	if req.Region != "" {
		for _, edge := range candidates {
			if edge.Region == req.Region {
				return edge, nil
			}
		}
	}

	return r.leastLoaded(candidates), nil
}

func (r *LatencyRouter) SelectOrigin(req *types.Request) (*types.OriginServer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*types.OriginServer
	for _, origin := range r.origins {
		if origin.Healthy {
			candidates = append(candidates, origin)
		}
	}

	if len(candidates) == 0 {
		return nil, ErrNoHealthyOrigin
	}

	return r.weightedRandom(candidates), nil
}

func (r *LatencyRouter) leastLoaded(edges []*types.EdgeNode) *types.EdgeNode {
	minLoad := edges[0].Load
	selected := edges[0]
	for _, edge := range edges[1:] {
		if edge.Load < minLoad {
			minLoad = edge.Load
			selected = edge
		}
	}
	return selected
}

func (r *LatencyRouter) weightedRandom(origins []*types.OriginServer) *types.OriginServer {
	totalWeight := 0
	for _, o := range origins {
		totalWeight += o.Weight
	}
	if totalWeight == 0 {
		return origins[rand.Intn(len(origins))]
	}

	roll := rand.Intn(totalWeight)
	for _, o := range origins {
		roll -= o.Weight
		if roll < 0 {
			return o
		}
	}
	return origins[len(origins)-1]
}

func (r *LatencyRouter) GetHealthyEdges() []*types.EdgeNode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var edges []*types.EdgeNode
	for _, e := range r.edges {
		if e.Healthy {
			edges = append(edges, e)
		}
	}
	return edges
}

func (r *LatencyRouter) GetHealthyOrigins() []*types.OriginServer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var origins []*types.OriginServer
	for _, o := range r.origins {
		if o.Healthy {
			origins = append(origins, o)
		}
	}
	return origins
}

func (r *LatencyRouter) UpdateLatency(edgeID, originID string, latencyMs int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.latencies[edgeID]; ok {
		r.latencies[edgeID][originID] = latencyMs
	}
}

func (r *LatencyRouter) StartHealthChecks(interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			r.checkEdges()
			r.checkOrigins()
		}
	}()
}

func (r *LatencyRouter) checkEdges() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, edge := range r.edges {
		healthy := r.ping(edge.Address)
		edge.Healthy = healthy
		if !healthy {
			edge.Load = edge.Capacity
		}
		_ = id
	}
}

func (r *LatencyRouter) checkOrigins() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, origin := range r.origins {
		origin.Healthy = r.ping(origin.Address)
		_ = id
	}
}

func (r *LatencyRouter) ping(addr string) bool {
	_, err := net.DialTimeout("tcp", addr, 2*time.Second)
	return err == nil
}

var (
	ErrNoHealthyEdge  = &RoutingError{Message: "no healthy edge nodes available"}
	ErrNoHealthyOrigin = &RoutingError{Message: "no healthy origin servers available"}
)

type RoutingError struct {
	Message string
}

func (e *RoutingError) Error() string {
	return e.Message
}