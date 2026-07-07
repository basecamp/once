package docker

// RingBuffer is a fixed-size circular buffer. It is not thread-safe;
// callers must synchronize access externally.
type RingBuffer[T any] struct {
	items []T
	head  int
	count int
}

func NewRingBuffer[T any](size int) *RingBuffer[T] {
	return &RingBuffer[T]{
		items: make([]T, size),
	}
}

func (b *RingBuffer[T]) Add(item T) {
	b.items[b.head] = item
	b.head = (b.head + 1) % len(b.items)
	if b.count < len(b.items) {
		b.count++
	}
}

func (b *RingBuffer[T]) Len() int {
	return b.count
}

// FetchOldestFirst returns up to n items in chronological order (oldest first).
func (b *RingBuffer[T]) FetchOldestFirst(n int) []T {
	available := min(n, b.count)
	if available == 0 {
		return nil
	}

	result := make([]T, available)
	startIdx := (b.head - b.count + len(b.items)) % len(b.items)
	offset := b.count - available

	for i := range available {
		idx := (startIdx + offset + i) % len(b.items)
		result[i] = b.items[idx]
	}

	return result
}

// FetchNewestFirst returns up to n items in reverse chronological order (newest first).
func (b *RingBuffer[T]) FetchNewestFirst(n int) []T {
	available := min(n, b.count)
	if available == 0 {
		return nil
	}

	result := make([]T, available)
	for i := range available {
		idx := (b.head - 1 - i + len(b.items)) % len(b.items)
		result[i] = b.items[idx]
	}

	return result
}
