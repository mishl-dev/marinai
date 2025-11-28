package bot

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"

	lru "github.com/hashicorp/golang-lru/v2"
)

type CachedClassifier struct {
	classifier Classifier
	cache      *lru.Cache[string, classificationResult]
	model      string
}

type classificationResult struct {
	Label string
	Score float64
}

func NewCachedClassifier(classifier Classifier, cacheSize int, model string) *CachedClassifier {
	cache, err := lru.New[string, classificationResult](cacheSize)
	if err != nil {
		// This should only happen if cacheSize <= 0
		log.Printf("Error creating LRU cache: %v. Using size 1000.", err)
		cache, _ = lru.New[string, classificationResult](1000)
	}

	return &CachedClassifier{
		classifier: classifier,
		cache:      cache,
		model:      model,
	}
}

func (c *CachedClassifier) Classify(text string, labels []string) (string, float64, error) {
	// Create a cache key based on text, model, AND labels
	// We hash the text and labels to keep keys short
	h := md5.New()
	h.Write([]byte(text))
	for _, label := range labels {
		h.Write([]byte(label))
	}
	keyHash := hex.EncodeToString(h.Sum(nil))
	key := fmt.Sprintf("%s:%s", c.model, keyHash)

	// Check cache
	if result, ok := c.cache.Get(key); ok {
		return result.Label, result.Score, nil
	}

	// Cache miss - call real classifier
	label, score, err := c.classifier.Classify(text, labels)
	if err != nil {
		return "", 0, err
	}

	// Cache the result
	c.cache.Add(key, classificationResult{
		Label: label,
		Score: score,
	})

	return label, score, nil
}
