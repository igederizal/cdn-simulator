package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/cobra"

	"github.com/yourusername/cdn-simulator/internal/cache"
	"github.com/yourusername/cdn-simulator/internal/config"
	"github.com/yourusername/cdn-simulator/pkg/types"
)

var (
	cfgFile string
	redisAddr string
)

var rootCmd = &cobra.Command{
	Use:   "cdnctl",
	Short: "CDN Simulator CLI",
	Long:  "Control plane for CDN Simulator - cache invalidation, stats, and management",
}

var purgeCmd = &cobra.Command{
	Use:   "purge [keys...]",
	Short: "Purge cache by keys",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c, err := newCacheClient()
		if err != nil {
			fatal(err)
		}
		defer c.Close()

		ctx := context.Background()
		for _, key := range args {
			if err := c.Delete(ctx, types.CacheKey(key)); err != nil {
				fmt.Printf("Failed to purge %s: %v\n", key, err)
			} else {
				fmt.Printf("Purged: %s\n", key)
			}
		}
	},
}

var purgeTagsCmd = &cobra.Command{
	Use:   "purge-tags [tags...]",
	Short: "Purge cache by tags",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c, err := newCacheClient()
		if err != nil {
			fatal(err)
		}
		defer c.Close()

		ctx := context.Background()
		if err := c.InvalidateByTags(ctx, args); err != nil {
			fatal(err)
		}
		fmt.Printf("Purged tags: %v\n", args)
	},
}

var purgePatternCmd = &cobra.Command{
	Use:   "purge-pattern [pattern]",
	Short: "Purge cache by wildcard pattern",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c, err := newCacheClient()
		if err != nil {
			fatal(err)
		}
		defer c.Close()

		ctx := context.Background()
		if err := c.InvalidateByPattern(ctx, args[0]); err != nil {
			fatal(err)
		}
		fmt.Printf("Purged pattern: %s\n", args[0])
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cache statistics",
	Run: func(cmd *cobra.Command, args []string) {
		c, err := newCacheClient()
		if err != nil {
			fatal(err)
		}
		defer c.Close()

		ctx := context.Background()
		stats, err := c.GetStats(ctx)
		if err != nil {
			fatal(err)
		}

		fmt.Printf("Cache Statistics:\n")
		fmt.Printf("  Hits:       %d\n", stats.Hits)
		fmt.Printf("  Misses:     %d\n", stats.Misses)
		fmt.Printf("  Stale Hits: %d\n", stats.StaleHits)
		fmt.Printf("  Errors:     %d\n", stats.Errors)
		fmt.Printf("  Keys:       %d\n", stats.KeysCount)
		fmt.Printf("  Memory:     %d bytes\n", stats.MemoryUsed)
		if stats.Hits+stats.Misses > 0 {
			hitRatio := float64(stats.Hits) / float64(stats.Hits+stats.Misses) * 100
			fmt.Printf("  Hit Ratio:  %.2f%%\n", hitRatio)
		}
	},
}

var warmCmd = &cobra.Command{
	Use:   "warm [urls...]",
	Short: "Warm cache with URLs",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c, err := newCacheClient()
		if err != nil {
			fatal(err)
		}
		defer c.Close()

		ctx := context.Background()
		for _, url := range args {
			key := types.CacheKey(url)
			if _, err := c.Get(ctx, key); err == cache.ErrCacheMiss {
				fmt.Printf("Warming: %s\n", url)
			} else {
				fmt.Printf("Already cached: %s\n", url)
			}
		}
	},
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check Redis connection health",
	Run: func(cmd *cobra.Command, args []string) {
		client := redis.NewClient(&redis.Options{
			Addr: redisAddr,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Ping(ctx).Err(); err != nil {
			fmt.Printf("Redis unhealthy: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Redis healthy")
	},
}

func newCacheClient() (*cache.RedisCache, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return cache.NewRedisCache(&cfg.Cache, redisAddr)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cdn-simulator.yaml)")
	rootCmd.PersistentFlags().StringVar(&redisAddr, "redis", "localhost:6379", "Redis address")

	rootCmd.AddCommand(purgeCmd)
	rootCmd.AddCommand(purgeTagsCmd)
	rootCmd.AddCommand(purgePatternCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(warmCmd)
	rootCmd.AddCommand(healthCmd)
}

func initConfig() {
	if cfgFile != "" {
		return
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}