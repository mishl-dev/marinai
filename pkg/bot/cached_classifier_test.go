package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCachedClassifier for testing
type MockCachedClassifier struct {
	mock.Mock
}

func (m *MockCachedClassifier) Classify(text string, labels []string) (string, float64, error) {
	args := m.Called(text, labels)
	return args.String(0), args.Get(1).(float64), args.Error(2)
}

func TestCachedClassifier_Classify(t *testing.T) {
	t.Run("Cache Hit", func(t *testing.T) {
		mockClassifier := new(MockCachedClassifier)
		// Initialize with size 10
		cachedClassifier := NewCachedClassifier(mockClassifier, 10, "test-model")

		text := "hello"
		labels := []string{"greeting", "farewell"}

		// Pre-populate cache manually (internal implementation detail, but needed for white-box testing)
		// Or simpler: call Classify once to populate, then call again to test hit.

		// 1. First call - Cache Miss
		mockClassifier.On("Classify", text, labels).Return("greeting", 0.95, nil).Once()

		label, score, err := cachedClassifier.Classify(text, labels)
		assert.NoError(t, err)
		assert.Equal(t, "greeting", label)
		assert.Equal(t, 0.95, score)

		// 2. Second call - Cache Hit
		// We do NOT expect Classify to be called again
		label2, score2, err2 := cachedClassifier.Classify(text, labels)
		assert.NoError(t, err2)
		assert.Equal(t, "greeting", label2)
		assert.Equal(t, 0.95, score2)

		mockClassifier.AssertExpectations(t)
	})

	t.Run("Cache Miss", func(t *testing.T) {
		mockClassifier := new(MockCachedClassifier)
		cachedClassifier := NewCachedClassifier(mockClassifier, 10, "test-model")

		text := "bye"
		labels := []string{"greeting", "farewell"}

		// Expectation: Real classifier called.
		mockClassifier.On("Classify", text, labels).Return("farewell", 0.88, nil)

		label, score, err := cachedClassifier.Classify(text, labels)

		assert.NoError(t, err)
		assert.Equal(t, "farewell", label)
		assert.Equal(t, 0.88, score)

		mockClassifier.AssertExpectations(t)
	})

	t.Run("Classifier Error", func(t *testing.T) {
		mockClassifier := new(MockCachedClassifier)
		cachedClassifier := NewCachedClassifier(mockClassifier, 10, "test-model")

		text := "error"
		labels := []string{"greeting", "farewell"}

		mockClassifier.On("Classify", text, labels).Return("", 0.0, assert.AnError)

		label, score, err := cachedClassifier.Classify(text, labels)

		assert.Error(t, err)
		assert.Equal(t, "", label)
		assert.Equal(t, 0.0, score)

		mockClassifier.AssertExpectations(t)
	})

	t.Run("LRU Eviction", func(t *testing.T) {
		mockClassifier := new(MockCachedClassifier)
		// Small cache size of 2
		cachedClassifier := NewCachedClassifier(mockClassifier, 2, "test-model")
		labels := []string{"A", "B"}

		// 1. Add Item 1
		mockClassifier.On("Classify", "1", labels).Return("A", 0.9, nil).Once()
		cachedClassifier.Classify("1", labels)

		// 2. Add Item 2
		mockClassifier.On("Classify", "2", labels).Return("B", 0.8, nil).Once()
		cachedClassifier.Classify("2", labels)

		// Cache is now [2, 1] (MRU -> LRU)

		// 3. Access Item 1 (makes it MRU)
		// Should be a hit
		cachedClassifier.Classify("1", labels)
		// Cache is now [1, 2]

		// 4. Add Item 3 (evicts LRU, which is 2)
		mockClassifier.On("Classify", "3", labels).Return("A", 0.7, nil).Once()
		cachedClassifier.Classify("3", labels)
		// Cache is now [3, 1]

		// 5. Access Item 2 (should be a miss and re-fetch)
		mockClassifier.On("Classify", "2", labels).Return("B", 0.8, nil).Once()
		cachedClassifier.Classify("2", labels)

		mockClassifier.AssertExpectations(t)
	})

	t.Run("Key Generation", func(t *testing.T) {
		// Verify that different texts produce different keys (implicitly tested by hits/misses)
		// and different models produce different keys.

		mockClassifier := new(MockCachedClassifier)
		c1 := NewCachedClassifier(mockClassifier, 10, "model-A")
		c2 := NewCachedClassifier(mockClassifier, 10, "model-B")

		text := "hello"
		labels := []string{"L"}

		// Call on c1
		mockClassifier.On("Classify", text, labels).Return("L", 0.9, nil).Twice() // Once for c1, once for c2

		c1.Classify(text, labels)

		// Call on c2 - should be a miss because model name is part of key (and they are different instances anyway)
		// But even if they shared a static cache (they don't), the key includes model name.
		c2.Classify(text, labels)

		// Verify c1 hit
		c1.Classify(text, labels)

		mockClassifier.AssertExpectations(t)
	})

	t.Run("Different Labels", func(t *testing.T) {
		mockClassifier := new(MockCachedClassifier)
		cachedClassifier := NewCachedClassifier(mockClassifier, 10, "test-model")

		text := "ambiguous text"
		labelsA := []string{"LabelA1", "LabelA2"}
		labelsB := []string{"LabelB1", "LabelB2"}

		// 1. First call with Labels A
		mockClassifier.On("Classify", text, labelsA).Return("LabelA1", 0.9, nil).Once()
		label, _, _ := cachedClassifier.Classify(text, labelsA)
		assert.Equal(t, "LabelA1", label)

		// 2. Second call with Labels B
		// Should be a CACHE MISS because labels are different
		mockClassifier.On("Classify", text, labelsB).Return("LabelB1", 0.8, nil).Once()
		label2, _, _ := cachedClassifier.Classify(text, labelsB)
		assert.Equal(t, "LabelB1", label2)

		mockClassifier.AssertExpectations(t)
	})
}
