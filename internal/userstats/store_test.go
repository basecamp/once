package userstats

import (
	"testing"
	"time"

	"github.com/axiomhq/hyperloglog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	sketch := hyperloglog.New()
	sketch.Insert([]byte("192.168.1.1"))
	sketch.Insert([]byte("192.168.1.2"))
	sketch.Insert([]byte("10.0.0.1"))

	sketchData, err := sketch.MarshalBinary()
	require.NoError(t, err)

	original := &BucketStore{
		LastTimestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Services: map[string]*ServiceData{
			"campfire": {
				Buckets:     [NumBuckets][]byte{0: sketchData},
				BucketHours: [NumBuckets]int64{0: 473850},
			},
		},
	}

	data, err := encodeStore(original)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	decoded, err := decodeStore(data)
	require.NoError(t, err)

	assert.Equal(t, original.LastTimestamp, decoded.LastTimestamp)
	require.Contains(t, decoded.Services, "campfire")
	assert.Equal(t, original.Services["campfire"].BucketHours, decoded.Services["campfire"].BucketHours)

	var restored hyperloglog.Sketch
	require.NoError(t, restored.UnmarshalBinary(decoded.Services["campfire"].Buckets[0]))
	assert.Equal(t, uint64(3), restored.Estimate())
}

func TestEncodeDecodeEmptyStore(t *testing.T) {
	original := &BucketStore{
		Services: make(map[string]*ServiceData),
	}

	data, err := encodeStore(original)
	require.NoError(t, err)

	decoded, err := decodeStore(data)
	require.NoError(t, err)

	assert.True(t, decoded.LastTimestamp.IsZero())
	assert.Empty(t, decoded.Services)
}

func TestComputeSummary(t *testing.T) {
	now := time.Now()
	currentHour := now.Unix() / 3600

	buildSketch := func(ips ...string) []byte {
		s := hyperloglog.New()
		for _, ip := range ips {
			s.Insert([]byte(ip))
		}
		data, _ := s.MarshalBinary()
		return data
	}

	svc := &ServiceData{}

	// Add data in recent hour (within 24h)
	recentIdx := currentHour % NumBuckets
	svc.Buckets[recentIdx] = buildSketch("1.1.1.1", "2.2.2.2")
	svc.BucketHours[recentIdx] = currentHour

	// Add data 3 days ago (within 7d but outside 24h)
	oldHour := currentHour - 72
	oldIdx := oldHour % NumBuckets
	if oldIdx == recentIdx {
		// Avoid collision
		oldHour--
		oldIdx = oldHour % NumBuckets
	}
	svc.Buckets[oldIdx] = buildSketch("3.3.3.3", "4.4.4.4")
	svc.BucketHours[oldIdx] = oldHour

	store := &BucketStore{
		LastTimestamp: now,
		Services:     map[string]*ServiceData{"campfire": svc},
	}

	summary := computeSummary(store)

	require.Contains(t, summary.Services, "campfire")
	cs := summary.Services["campfire"]

	assert.Equal(t, uint64(2), cs.UniqueUsers24h)
	assert.Equal(t, uint64(4), cs.UniqueUsers7d)
}

func TestComputeSummaryExpiredBuckets(t *testing.T) {
	now := time.Now()
	currentHour := now.Unix() / 3600

	buildSketch := func(ips ...string) []byte {
		s := hyperloglog.New()
		for _, ip := range ips {
			s.Insert([]byte(ip))
		}
		data, _ := s.MarshalBinary()
		return data
	}

	svc := &ServiceData{}

	// Add data from 10 days ago (outside 7d window)
	expiredHour := currentHour - 240
	expiredIdx := expiredHour % NumBuckets
	svc.Buckets[expiredIdx] = buildSketch("1.1.1.1")
	svc.BucketHours[expiredIdx] = expiredHour

	store := &BucketStore{
		LastTimestamp: now,
		Services:     map[string]*ServiceData{"campfire": svc},
	}

	summary := computeSummary(store)

	cs := summary.Services["campfire"]
	assert.Equal(t, uint64(0), cs.UniqueUsers24h)
	assert.Equal(t, uint64(0), cs.UniqueUsers7d)
}
