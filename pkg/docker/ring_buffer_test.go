package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRingBufferAdd(t *testing.T) {
	b := NewRingBuffer[int](5)
	assert.Equal(t, 0, b.Len())

	b.Add(1)
	b.Add(2)
	b.Add(3)
	assert.Equal(t, 3, b.Len())
}

func TestRingBufferFetchOldestFirst(t *testing.T) {
	b := NewRingBuffer[string](10)
	b.Add("a")
	b.Add("b")
	b.Add("c")

	result := b.FetchOldestFirst(10)
	require.Len(t, result, 3)
	assert.Equal(t, "a", result[0])
	assert.Equal(t, "b", result[1])
	assert.Equal(t, "c", result[2])
}

func TestRingBufferFetchOldestFirstWithLimit(t *testing.T) {
	b := NewRingBuffer[string](10)
	for i := range 5 {
		b.Add(string(rune('a' + i)))
	}

	result := b.FetchOldestFirst(2)
	require.Len(t, result, 2)
	assert.Equal(t, "d", result[0])
	assert.Equal(t, "e", result[1])
}

func TestRingBufferFetchNewestFirst(t *testing.T) {
	b := NewRingBuffer[int](10)
	b.Add(1)
	b.Add(2)
	b.Add(3)

	result := b.FetchNewestFirst(10)
	require.Len(t, result, 3)
	assert.Equal(t, 3, result[0])
	assert.Equal(t, 2, result[1])
	assert.Equal(t, 1, result[2])
}

func TestRingBufferFetchNewestFirstWithLimit(t *testing.T) {
	b := NewRingBuffer[int](10)
	for i := range 5 {
		b.Add(i + 1)
	}

	result := b.FetchNewestFirst(2)
	require.Len(t, result, 2)
	assert.Equal(t, 5, result[0])
	assert.Equal(t, 4, result[1])
}

func TestRingBufferWrap(t *testing.T) {
	b := NewRingBuffer[string](3)
	for i := range 5 {
		b.Add(string(rune('a' + i)))
	}

	assert.Equal(t, 3, b.Len())

	oldest := b.FetchOldestFirst(10)
	require.Len(t, oldest, 3)
	assert.Equal(t, "c", oldest[0])
	assert.Equal(t, "d", oldest[1])
	assert.Equal(t, "e", oldest[2])

	newest := b.FetchNewestFirst(10)
	require.Len(t, newest, 3)
	assert.Equal(t, "e", newest[0])
	assert.Equal(t, "d", newest[1])
	assert.Equal(t, "c", newest[2])
}

func TestRingBufferFetchEmpty(t *testing.T) {
	b := NewRingBuffer[int](10)
	assert.Nil(t, b.FetchOldestFirst(10))
	assert.Nil(t, b.FetchNewestFirst(10))
}
