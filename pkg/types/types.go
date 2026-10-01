package types

type CacheKey string

type CacheEntry struct {
	Key         CacheKey
	Value       []byte
	Headers     map[string]string
	StatusCode  int
	CreatedAt   int64
	ExpiresAt   int64
	StaleUntil  int64
	Tags        []string
	Region      string
	HitCount    int64
}

type Request struct {
	Method      string
	Path        string
	Headers     map[string]string
	QueryParams map[string]string
	Body        []byte
	ClientIP    string
	Region      string
}

type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	FromCache  bool
	Region     string
	LatencyMs  int64
}

type InvalidationRequest struct {
	Keys   []CacheKey
	Tags   []string
	Wildcard string
}

type EdgeNode struct {
	ID       string
	Region   string
	Address  string
	Healthy  bool
	Capacity int
	Load     int
}

type OriginServer struct {
	ID      string
	Address string
	Healthy bool
	Weight  int
}

type RoutingDecision struct {
	EdgeNode    *EdgeNode
	OriginServer *OriginServer
	Reason      string
}

const (
	CacheStatusHit       = "HIT"
	CacheStatusMiss      = "MISS"
	CacheStatusStale     = "STALE"
	CacheStatusBypass    = "BYPASS"
	CacheStatusError     = "ERROR"
)