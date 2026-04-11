package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Admin    AdminConfig
	Cache    CacheConfig
	S3       S3Config
	Store    StoreConfig
	Fetch    FetchConfig
	Convert  ConvertConfig
	Fallback FallbackConfig
	Security SecurityConfig
	Log      LogConfig
}

type ServerConfig struct {
	Host           string
	Port           int
	RequestTimeout time.Duration
	CDNName        string
}

type AdminConfig struct {
	Port int
}

type CacheConfig struct {
	Dir            string
	MaxBytes       int64
	TargetBytes    int64
	ControlSuccess string
	Control4XX     string
	Control5XX     string
	ControlFailover string
}

type S3Config struct {
	Enabled        bool
	Endpoint       string
	Bucket         string
	AccessKey      string
	SecretKey      string
	LifecycleDays  int
	RenewAfterDays int
}

type StoreConfig struct {
	Backend       string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
}

type FetchConfig struct {
	Timeout      time.Duration
	MaxRedirects int
	MaxFileSize  int64
}

type ConvertConfig struct {
	Concurrency    int
	WebPQuality    int
	PNGCompression int
	AnimQuality    int
}

type FallbackConfig struct {
	Avatar  string
	Emoji   string
	Badge   string
	Default string
}

type SecurityConfig struct {
	AllowedPrivateCIDRs []string
	BlacklistDomains    []string
	BlacklistIPs        []string
	BlacklistFile       string
	CircuitBreaker      CircuitConfig
	NegativeCache       NegCacheConfig
}

type CircuitConfig struct {
	Enabled   bool
	Threshold int
	Timeout   time.Duration
}

type NegCacheConfig struct {
	TTL40X time.Duration
	TTL5XX time.Duration
}

type LogConfig struct {
	Level string
}

func Load() (*Config, error) {
	cfg := &Config{}

	cfg.Server.Host = getEnv("HOST", "0.0.0.0")
	cfg.Server.Port = getEnvInt("PORT", 3000)
	cfg.Server.RequestTimeout = getEnvDuration("REQUEST_TIMEOUT", 60*time.Second)
	cfg.Server.CDNName = os.Getenv("CDN_NAME")

	cfg.Admin.Port = getEnvInt("ADMIN_PORT", 3001)

	cfg.Cache.Dir = getEnv("CACHE_DIR", "/cache")
	cfg.Cache.MaxBytes = getEnvBytes("CACHE_MAX_BYTES", 4*1024*1024*1024)   // 4GiB
	cfg.Cache.TargetBytes = getEnvBytes("CACHE_TARGET_BYTES", 3*1024*1024*1024) // 3GiB
	cfg.Cache.ControlSuccess = getEnv("CACHE_CONTROL_SUCCESS", "max-age=31536000, immutable")
	cfg.Cache.Control4XX = getEnv("CACHE_CONTROL_4XXERROR", "max-age=3600")
	cfg.Cache.Control5XX = getEnv("CACHE_CONTROL_5XXERROR", "max-age=120, must-revalidate")
	cfg.Cache.ControlFailover = getEnv("CACHE_CONTROL_FAILOVER", "max-age=86400")

	cfg.S3.Enabled = getEnvBool("S3_ENABLED", false)
	cfg.S3.Endpoint = os.Getenv("S3_ENDPOINT")
	cfg.S3.Bucket = os.Getenv("S3_BUCKET")
	cfg.S3.AccessKey = os.Getenv("S3_ACCESS_KEY")
	cfg.S3.SecretKey = os.Getenv("S3_SECRET_KEY")
	cfg.S3.LifecycleDays = getEnvInt("S3_LIFECYCLE_DAYS", 42)
	cfg.S3.RenewAfterDays = getEnvInt("S3_RENEW_AFTER_DAYS", 28)

	cfg.Store.Backend = getEnv("STORE_BACKEND", "memory")
	cfg.Store.RedisAddr = os.Getenv("REDIS_ADDR")
	cfg.Store.RedisPassword = os.Getenv("REDIS_PASSWORD")
	cfg.Store.RedisDB = getEnvInt("REDIS_DB", 0)

	cfg.Fetch.Timeout = getEnvDuration("FETCH_TIMEOUT", 30*time.Second)
	cfg.Fetch.MaxRedirects = getEnvInt("FETCH_MAX_REDIRECTS", 3)
	cfg.Fetch.MaxFileSize = getEnvBytes("MAX_FILE_SIZE", 250*1024*1024) // 250MiB

	cfg.Convert.Concurrency = getEnvInt("CONVERT_CONCURRENCY", runtime.NumCPU())
	cfg.Convert.WebPQuality = getEnvInt("CONVERT_WEBP_QUALITY", 80)
	cfg.Convert.PNGCompression = getEnvInt("CONVERT_PNG_COMPRESSION", 6)
	cfg.Convert.AnimQuality = getEnvInt("CONVERT_ANIM_QUALITY", 75)

	cfg.Fallback.Avatar = os.Getenv("FALLBACK_AVATAR")
	cfg.Fallback.Emoji = os.Getenv("FALLBACK_EMOJI")
	cfg.Fallback.Badge = os.Getenv("FALLBACK_BADGE")
	cfg.Fallback.Default = os.Getenv("FALLBACK_DEFAULT")

	allowedNets := os.Getenv("ORIGIN_ALLOWED_PRIVATE_NETWORKS")
	if allowedNets != "" {
		cfg.Security.AllowedPrivateCIDRs = splitComma(allowedNets)
	}
	blacklistDomains := os.Getenv("ORIGIN_BLACKLIST_DOMAINS")
	if blacklistDomains != "" {
		cfg.Security.BlacklistDomains = splitComma(blacklistDomains)
	}
	blacklistIPs := os.Getenv("ORIGIN_BLACKLIST_IPS")
	if blacklistIPs != "" {
		cfg.Security.BlacklistIPs = splitComma(blacklistIPs)
	}
	cfg.Security.BlacklistFile = os.Getenv("ORIGIN_BLACKLIST_FILE")

	cfg.Security.CircuitBreaker.Enabled = getEnvBool("CIRCUIT_BREAKER_ENABLED", true)
	cfg.Security.CircuitBreaker.Threshold = getEnvInt("CIRCUIT_BREAKER_THRESHOLD", 5)
	cfg.Security.CircuitBreaker.Timeout = getEnvDuration("CIRCUIT_BREAKER_TIMEOUT", 5*time.Minute)

	cfg.Security.NegativeCache.TTL40X = getEnvDuration("NEGATIVE_CACHE_TTL_40X", 24*time.Hour)
	cfg.Security.NegativeCache.TTL5XX = getEnvDuration("NEGATIVE_CACHE_TTL_5XX", 5*time.Minute)

	cfg.Log.Level = getEnv("LOG_LEVEL", "INFO")

	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	if c.S3.Enabled {
		if c.S3.RenewAfterDays >= c.S3.LifecycleDays {
			return fmt.Errorf("S3_RENEW_AFTER_DAYS (%d) must be less than S3_LIFECYCLE_DAYS (%d)",
				c.S3.RenewAfterDays, c.S3.LifecycleDays)
		}
		if c.S3.Endpoint == "" {
			return fmt.Errorf("S3_ENDPOINT is required when S3_ENABLED=true")
		}
		if c.S3.Bucket == "" {
			return fmt.Errorf("S3_BUCKET is required when S3_ENABLED=true")
		}
	}
	if c.Store.Backend == "redis" && c.Store.RedisAddr == "" {
		return fmt.Errorf("REDIS_ADDR is required when STORE_BACKEND=redis")
	}
	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// getEnvBytes parses byte sizes. Supports plain integers and GiB/MiB/KiB suffixes.
func getEnvBytes(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	multipliers := map[string]int64{
		"GiB": 1024 * 1024 * 1024,
		"MiB": 1024 * 1024,
		"KiB": 1024,
		"GB":  1000 * 1000 * 1000,
		"MB":  1000 * 1000,
		"KB":  1000,
	}
	for suffix, mult := range multipliers {
		if strings.HasSuffix(v, suffix) {
			n, err := strconv.ParseInt(strings.TrimSuffix(v, suffix), 10, 64)
			if err != nil {
				return def
			}
			return n * mult
		}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}
