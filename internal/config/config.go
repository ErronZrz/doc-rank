package config

import "time"

type Config struct {
	Port                string
	DataDir             string
	WALPath             string
	SnapshotPath        string
	SnapshotInterval    time.Duration
	WALGroupCommitEvery time.Duration
	WALGroupBatch       int
	RecentWindowSeconds int
	SSEPushInterval     time.Duration
	SSETopK             int
}

func Default() Config {
	return Config{
		Port:                "8080",
		DataDir:             "data",
		WALPath:             "data/wal.log",
		SnapshotPath:        "data/snapshot.json",
		SnapshotInterval:    time.Minute,
		WALGroupCommitEvery: 10 * time.Millisecond,
		WALGroupBatch:       256,
		RecentWindowSeconds: 600,
		SSEPushInterval:     100 * time.Millisecond,
		SSETopK:             100,
	}
}
