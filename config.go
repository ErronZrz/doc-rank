package main

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port              string
	DataDir           string
	TopKDefault       int
	SnapshotInterval  time.Duration
	WALSyncEveryWrite bool
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustAtoi(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

func mustParseDuration(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

// LoadConfig 加载配置
func LoadConfig() Config {
	return Config{
		Port:              getenv("PORT", "8080"),
		DataDir:           getenv("DATA_DIR", "./data"),
		TopKDefault:       mustAtoi(getenv("TOPK_DEFAULT", "100"), 100),
		SnapshotInterval:  mustParseDuration(getenv("SNAPSHOT_INTERVAL", "60s"), 60*time.Second),
		WALSyncEveryWrite: getenv("WAL_SYNC_EVERY_WRITE", "true") == "true",
	}
}
