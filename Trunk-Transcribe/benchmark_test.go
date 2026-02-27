package main

import (
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

func BenchmarkExtractSlackMeta(b *testing.B) {
	meta := createTestMetadata()
	meta.AudioText = "incident at Bancroft and Telegraph with weapon involved"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractSlackMeta(meta, BERKELEY, notifsMap)
	}
}

func BenchmarkDedupeDispatch(b *testing.B) {
	// Clear cache before benchmark
	newCache, _ := lru.New[string, bool](1000)
	dedupeCache = newCache

	meta := createTestMetadata()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dedupeDispatch(meta)
	}
}

func BenchmarkIsValid(b *testing.B) {
	call := Call{
		Audio:     make([]byte, 100),
		DateTime:  time.Now(),
		System:    1,
		Talkgroup: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = call.IsValid()
	}
}
