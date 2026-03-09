package userstats

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/axiomhq/hyperloglog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessLine(t *testing.T) {
	s := NewScraper("once")

	line := mustMarshal(t, logEntry{
		Time:       time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		RemoteAddr: "192.168.1.1",
		Service:    "campfire",
	})

	s.processLine(line)

	require.Contains(t, s.live, "campfire")
	live := s.live["campfire"]

	unixHour := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix() / 3600
	idx := unixHour % NumBuckets

	assert.Equal(t, unixHour, live.bucketHours[idx])
	assert.NotNil(t, live.buckets[idx])
	assert.Equal(t, uint64(1), live.buckets[idx].Estimate())
}

func TestProcessLineSkipsIncomplete(t *testing.T) {
	s := NewScraper("once")

	// Missing service
	s.processLine(mustMarshal(t, logEntry{
		Time:       time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		RemoteAddr: "192.168.1.1",
	}))

	// Missing remote_addr
	s.processLine(mustMarshal(t, logEntry{
		Time:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Service: "campfire",
	}))

	// Invalid JSON
	s.processLine([]byte("not json"))

	assert.Empty(t, s.live)
}

func TestProcessLineSkipsHealthCheck(t *testing.T) {
	s := NewScraper("once")
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "1.2.3.4", Service: "campfire", Path: "/up"}))

	assert.Empty(t, s.live)
}

func TestProcessLineSkipsLoopback(t *testing.T) {
	s := NewScraper("once")
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "127.0.0.1", Service: "campfire"}))
	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "::1", Service: "campfire"}))

	assert.Empty(t, s.live)
}

func TestProcessLineHandlesXFF(t *testing.T) {
	s := NewScraper("once")
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "1.2.3.4, 5.6.7.8", Service: "campfire"}))

	require.Contains(t, s.live, "campfire")
	live := s.live["campfire"]

	unixHour := ts.Unix() / 3600
	idx := unixHour % NumBuckets

	assert.Equal(t, uint64(1), live.buckets[idx].Estimate())

	// Adding the same first IP again should not increase the count
	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "1.2.3.4, 9.9.9.9", Service: "campfire"}))
	assert.Equal(t, uint64(1), live.buckets[idx].Estimate())
}

func TestProcessLineUniqueUsers(t *testing.T) {
	s := NewScraper("once")
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	addEntry := func(ip string) {
		s.processLine(mustMarshal(t, logEntry{
			Time:       ts,
			RemoteAddr: ip,
			Service:    "campfire",
		}))
	}

	addEntry("192.168.1.1")
	addEntry("192.168.1.2")
	addEntry("192.168.1.1") // duplicate

	live := s.live["campfire"]
	unixHour := ts.Unix() / 3600
	idx := unixHour % NumBuckets

	assert.Equal(t, uint64(2), live.buckets[idx].Estimate())
}

func TestProcessLineMultipleServices(t *testing.T) {
	s := NewScraper("once")
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "1.1.1.1", Service: "campfire"}))
	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "2.2.2.2", Service: "hey"}))

	assert.Contains(t, s.live, "campfire")
	assert.Contains(t, s.live, "hey")
}

func TestBucketCircularOverwrite(t *testing.T) {
	s := NewScraper("once")

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	baseHour := baseTime.Unix() / 3600
	idx := baseHour % NumBuckets

	// Insert at hour H
	s.processLine(mustMarshal(t, logEntry{
		Time:       baseTime,
		RemoteAddr: "1.1.1.1",
		Service:    "campfire",
	}))

	live := s.live["campfire"]
	assert.Equal(t, baseHour, live.bucketHours[idx])
	assert.Equal(t, uint64(1), live.buckets[idx].Estimate())

	// Insert at hour H + NumBuckets (same index, different hour)
	futureTime := baseTime.Add(NumBuckets * time.Hour)
	futureHour := futureTime.Unix() / 3600

	s.processLine(mustMarshal(t, logEntry{
		Time:       futureTime,
		RemoteAddr: "2.2.2.2",
		Service:    "campfire",
	}))

	assert.Equal(t, futureHour, live.bucketHours[idx])
	// Old data is gone, only the new IP
	assert.Equal(t, uint64(1), live.buckets[idx].Estimate())
}

func TestSyncLiveToStore(t *testing.T) {
	s := NewScraper("once")
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "1.1.1.1", Service: "campfire"}))
	s.processLine(mustMarshal(t, logEntry{Time: ts, RemoteAddr: "2.2.2.2", Service: "campfire"}))

	s.syncLiveToStore()

	require.Contains(t, s.store.Services, "campfire")
	svc := s.store.Services["campfire"]

	unixHour := ts.Unix() / 3600
	idx := unixHour % NumBuckets

	assert.Equal(t, unixHour, svc.BucketHours[idx])
	require.NotNil(t, svc.Buckets[idx])

	// Verify the serialized sketch has correct count
	var sketch hyperloglog.Sketch
	require.NoError(t, sketch.UnmarshalBinary(svc.Buckets[idx]))
	assert.Equal(t, uint64(2), sketch.Estimate())
}

func TestLastTimestampTracking(t *testing.T) {
	s := NewScraper("once")

	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	s.processLine(mustMarshal(t, logEntry{Time: t1, RemoteAddr: "1.1.1.1", Service: "campfire"}))
	assert.Equal(t, t1, s.store.LastTimestamp)

	s.processLine(mustMarshal(t, logEntry{Time: t2, RemoteAddr: "1.1.1.1", Service: "campfire"}))
	assert.Equal(t, t2, s.store.LastTimestamp)

	// Earlier timestamp doesn't move it back
	s.processLine(mustMarshal(t, logEntry{Time: t1, RemoteAddr: "2.2.2.2", Service: "campfire"}))
	assert.Equal(t, t2, s.store.LastTimestamp)
}

// Helpers

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
