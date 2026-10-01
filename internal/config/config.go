package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"github.com/yourusername/cdn-simulator/pkg/types"
)

type Config struct {
	Edge    EdgeConfig    `mapstructure:"edge"`
	Origin  OriginConfig  `mapstructure:"origin"`
	Router  RouterConfig  `mapstructure:"router"`
	Cache   CacheConfig   `mapstructure:"cache"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Logging LoggingConfig `mapstructure:"logging"`
}

type EdgeConfig struct {
	ID                string `mapstructure:"id"`
	Region            string `mapstructure:"region"`
	Port              int    `mapstructure:"port"`
	RedisAddr         string `mapstructure:"redis_addr"`
	Capacity          int    `mapstructure:"capacity"`
	EnableCompression bool   `mapstructure:"enable_compression"`
	CompressionLevel  int    `mapstructure:"compression_level"`
	AdminAPIKey       string `mapstructure:"admin_api_key"`
}

type OriginConfig struct {
	ID       string   `mapstructure:"id"`
	Port     int      `mapstructure:"port"`
	Backends []string `mapstructure:"backends"`
}

type RouterConfig struct {
	Port                int      `mapstructure:"port"`
	Strategy            string   `mapstructure:"strategy"`
	HealthCheckInterval int      `mapstructure:"health_check_interval"`
	GeoDBPath           string   `mapstructure:"geo_db_path"`
	Edges               []string `mapstructure:"edges"`
}

type CacheConfig struct {
	DefaultTTL           int    `mapstructure:"default_ttl"`
	MaxTTL               int    `mapstructure:"max_ttl"`
	StaleWhileRevalidate int    `mapstructure:"stale_while_revalidate"`
	MaxObjectSize        int64  `mapstructure:"max_object_size"`
	EnableTags           bool   `mapstructure:"enable_tags"`
	EnableStale          bool   `mapstructure:"enable_stale"`
	RedisPassword        string `mapstructure:"redis_password"`
}

type MetricsConfig struct {
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
	Enabled bool   `mapstructure:"enabled"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./deployments")
	viper.AddConfigPath("/etc/cdn-simulator")
	viper.AddConfigPath("/")
	viper.AutomaticEnv()

	setDefaults()
	bindEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if v := os.Getenv("ORIGIN_BACKENDS"); v != "" {
		cfg.Origin.Backends = splitCSV(v)
	}

	if v := os.Getenv("ROUTER_EDGES"); v != "" {
		cfg.Router.Edges = splitCSV(v)
	}

	return &cfg, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func bindEnv() {
	viper.BindEnv("edge.id", "EDGE_ID")
	viper.BindEnv("edge.region", "EDGE_REGION")
	viper.BindEnv("edge.port", "EDGE_PORT")
	viper.BindEnv("edge.redis_addr", "REDIS_ADDR")
	viper.BindEnv("edge.admin_api_key", "EDGE_ADMIN_API_KEY")
	viper.BindEnv("cache.redis_password", "REDIS_PASSWORD")
	viper.BindEnv("origin.id", "ORIGIN_ID")
	viper.BindEnv("origin.port", "ORIGIN_PORT")
	viper.BindEnv("router.port", "ROUTER_PORT")
	viper.BindEnv("router.strategy", "ROUTER_STRATEGY")
}

func setDefaults() {
	viper.SetDefault("edge.id", "edge-local-1")
	viper.SetDefault("edge.region", "local")
	viper.SetDefault("edge.port", 8080)
	viper.SetDefault("edge.redis_addr", "localhost:6379")
	viper.SetDefault("edge.capacity", 10000)
	viper.SetDefault("edge.enable_compression", true)
	viper.SetDefault("edge.compression_level", 5)

	viper.SetDefault("origin.id", "origin-local-1")
	viper.SetDefault("origin.port", 9090)
	viper.SetDefault("origin.backends", []string{"http://localhost:9090"})

	viper.SetDefault("router.port", 80)
	viper.SetDefault("router.strategy", "latency")
	viper.SetDefault("router.health_check_interval", 10)

	viper.SetDefault("cache.default_ttl", 300)
	viper.SetDefault("cache.max_ttl", 86400)
	viper.SetDefault("cache.stale_while_revalidate", 60)
	viper.SetDefault("cache.max_object_size", 104857600)
	viper.SetDefault("cache.enable_tags", true)
	viper.SetDefault("cache.enable_stale", true)

	viper.SetDefault("metrics.port", 9091)
	viper.SetDefault("metrics.path", "/metrics")
	viper.SetDefault("metrics.enabled", true)

	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
}

func (c *Config) GetEdgeNode() *types.EdgeNode {
	return &types.EdgeNode{
		ID:       c.Edge.ID,
		Region:   c.Edge.Region,
		Address:  ":" + strconv.Itoa(c.Edge.Port),
		Healthy:  true,
		Capacity: c.Edge.Capacity,
		Load:     0,
	}
}
