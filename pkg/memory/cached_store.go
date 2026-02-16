package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"marinai/pkg/cache"
	"time"
)

type CachedStore struct {
	Store
	cache *cache.Cache
}

func NewCachedStore(store Store, cache *cache.Cache) *CachedStore {
	return &CachedStore{
		Store: store,
		cache: cache,
	}
}

func (c *CachedStore) GetRecentMessages(userID string) ([]RecentMessageItem, error) {
	ctx := context.Background()
	key := c.cache.Key("recent_messages", userID)

	data, err := c.cache.LRange(ctx, key, 0, -1)
	if err == nil && len(data) > 0 {
		var messages []RecentMessageItem
		for _, d := range data {
			var msg RecentMessageItem
			if unmarshalErr := json.Unmarshal([]byte(d), &msg); unmarshalErr != nil {
				continue
			}
			messages = append(messages, msg)
		}
		if len(messages) > 0 {
			for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
				messages[i], messages[j] = messages[j], messages[i]
			}
			return messages, nil
		}
	}

	messages, err := c.Store.GetRecentMessages(userID)
	if err != nil {
		return nil, err
	}

	if len(messages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			msgJSON, marshalErr := json.Marshal(messages[i])
			if marshalErr != nil {
				continue
			}
			if pushErr := c.cache.LPush(ctx, key, string(msgJSON)); pushErr != nil {
				break
			}
		}
		_ = c.cache.LTrim(ctx, key, 0, 14)
		_ = c.cache.Expire(ctx, key, cache.RecentMessagesTTL)
	}

	return messages, nil
}

func (c *CachedStore) AddRecentMessage(userID, role, message string) error {
	err := c.Store.AddRecentMessage(userID, role, message)
	if err != nil {
		return err
	}

	ctx := context.Background()
	key := c.cache.Key("recent_messages", userID)

	msg := RecentMessageItem{
		Role:      role,
		Text:      message,
		Timestamp: time.Now().Unix(),
	}
	msgJSON, marshalErr := json.Marshal(msg)
	if marshalErr != nil {
		return nil
	}

	if pushErr := c.cache.LPush(ctx, key, string(msgJSON)); pushErr != nil {
		return nil
	}

	_ = c.cache.LTrim(ctx, key, 0, 14)
	_ = c.cache.Expire(ctx, key, cache.RecentMessagesTTL)

	return nil
}

func (c *CachedStore) ClearRecentMessages(userID string) error {
	err := c.Store.ClearRecentMessages(userID)
	if err != nil {
		return err
	}

	ctx := context.Background()
	key := c.cache.Key("recent_messages", userID)
	_ = c.cache.Delete(ctx, key)

	return nil
}

func (c *CachedStore) GetCachedEmojis(guildID string) ([]string, error) {
	ctx := context.Background()
	key := c.cache.Key("emoji_cache", guildID)

	var emojis []string
	err := c.cache.GetJSON(ctx, key, &emojis)
	if err == nil && len(emojis) > 0 {
		return emojis, nil
	}

	emojis, err = c.Store.GetCachedEmojis(guildID)
	if err != nil {
		return nil, err
	}

	if len(emojis) > 0 {
		_ = c.cache.SetJSON(ctx, key, emojis, cache.EmojiCacheTTL)
	}

	return emojis, nil
}

func (c *CachedStore) SetCachedEmojis(guildID string, emojis []string) error {
	err := c.Store.SetCachedEmojis(guildID, emojis)
	if err != nil {
		return fmt.Errorf("failed to set emojis in store: %w", err)
	}

	ctx := context.Background()
	key := c.cache.Key("emoji_cache", guildID)

	if len(emojis) > 0 {
		_ = c.cache.SetJSON(ctx, key, emojis, cache.EmojiCacheTTL)
	} else {
		_ = c.cache.Delete(ctx, key)
	}

	return nil
}
