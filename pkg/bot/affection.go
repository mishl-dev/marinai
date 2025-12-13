package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"marinai/pkg/cerebras"
	"strings"
	"time"
)

// AffectionLevel represents different tiers of closeness
// Uses big numbers for impressive display, but progression is reasonable
type AffectionLevel struct {
	Name         string
	MinAffection int
	MaxAffection int
	Emoji        string
}

// AffectionLevels defines the tiers - big numbers but same relative difficulty
var AffectionLevels = []AffectionLevel{
	{Name: "Stranger", MinAffection: 0, MaxAffection: 999, Emoji: "👋"},
	{Name: "Acquaintance", MinAffection: 1000, MaxAffection: 2499, Emoji: "🙂"},
	{Name: "Friend", MinAffection: 2500, MaxAffection: 4999, Emoji: "😊"},
	{Name: "Close Friend", MinAffection: 5000, MaxAffection: 7499, Emoji: "💕"},
	{Name: "Best Friend", MinAffection: 7500, MaxAffection: 8999, Emoji: "💗"},
	{Name: "Special Someone", MinAffection: 9000, MaxAffection: 10000, Emoji: "❤️"},
}

// MaxAffection is the cap for affection points
const MaxAffection = 10000

// GetAffectionLevel returns the level info for a given affection value
func GetAffectionLevel(affection int) AffectionLevel {
	for _, level := range AffectionLevels {
		if affection >= level.MinAffection && affection <= level.MaxAffection {
			return level
		}
	}
	// Above max? Return highest level
	if affection > AffectionLevels[len(AffectionLevels)-1].MaxAffection {
		return AffectionLevels[len(AffectionLevels)-1]
	}
	return AffectionLevels[0] // Default to stranger
}

// AffectionGains defines how much affection is gained for various actions
// Big impressive numbers! (~100x scale)
var AffectionGains = map[string]int{
	// Base interaction gains
	"message":        100, // Any message interaction
	"mention":        250, // User mentions Marin directly
	"dm":             250, // DM conversation (more intimate)
	"long_message":   150, // Bonus for thoughtful long messages (50+ chars)
	"long_convo":     500, // Extended back-and-forth (5+ exchanges)
	"respond_to_dm":  750, // Responding to boredom DM (shows they care)

	// Behavioral bonuses
	"compliment":     400, // Complimenting Marin
	"ask_about_her":  250, // Asking about Marin's life/interests
	"enthusiasm":     150, // Being enthusiastic/positive
	"share_personal": 400, // Sharing personal info (trust)
	"flirty":         300, // Flirting back with Marin
	"remember_fact":  250, // When user references something Marin told them
	"supportive":     350, // Being caring/supportive when Marin shares
	"curious":        200, // Asking follow-up questions, showing genuine interest
	"playful":        200, // Teasing back, being witty
	"grateful":       300, // Expressing thanks
	"affectionate":   400, // Using pet names, being sweet
	"vulnerable":     500, // Opening up about feelings/struggles
}

// AffectionPenalties defines affection reduction for negative behaviors
var AffectionPenalties = map[string]int{
	"rude":              -750,  // Being mean/rude to Marin
	"dismissive":        -250,  // Short dismissive responses ("k", "whatever")
	"ignore_question":   -150,  // Ignoring Marin's questions
	"ghosting":          -400,  // Starting convo then disappearing mid-chat
	"insult":            -1000, // Direct insults
	"dry_response":      -100,  // One-word low-effort responses
	"impatient":         -200,  // Being impatient or rushing Marin
	"passive_aggressive": -400, // Sarcastic, backhanded comments
	"disinterested":     -300,  // Changing subject when Marin shares something
	"creepy":            -600,  // Being inappropriate/making Marin uncomfortable
}

// AffectionDecayRates defines decay per day based on relationship level
// Scaled to match new point values
var AffectionDecayRates = map[string]float64{
	"Stranger":        100.0, // Strangers forgotten quickly
	"Acquaintance":    75.0,  // Still fades
	"Friend":          50.0,  // Normal decay
	"Close Friend":    25.0,  // More stable
	"Best Friend":     10.0,  // Very stable
	"Special Someone": 5.0,   // Almost never decays
}

// GetDecayRate returns the daily decay rate for a given affection level
func GetDecayRate(affection int) float64 {
	level := GetAffectionLevel(affection)
	if rate, ok := AffectionDecayRates[level.Name]; ok {
		return rate
	}
	return 1.0 // Default
}

// ApplyAffectionDecay reduces affection based on days since last interaction
// Returns the amount decayed (0 if none)
func (h *Handler) ApplyAffectionDecay(userID string, daysSinceInteraction float64) int {
	if daysSinceInteraction < 1 {
		return 0 // No decay within first day
	}

	currentAffection, err := h.memoryStore.GetAffection(userID)
	if err != nil || currentAffection == 0 {
		return 0
	}

	decayRate := GetDecayRate(currentAffection)
	totalDecay := int(decayRate * daysSinceInteraction)

	if totalDecay <= 0 {
		return 0
	}

	newAffection := currentAffection - totalDecay
	if newAffection < 0 {
		newAffection = 0
	}

	if err := h.memoryStore.SetAffection(userID, newAffection); err != nil {
		log.Printf("Error applying affection decay for %s: %v", userID, err)
		return 0
	}

	log.Printf("Affection decay for %s: %d -> %d (%.1f days, rate %.2f/day)",
		userID, currentAffection, newAffection, daysSinceInteraction, decayRate)

	return totalDecay
}

// GetAffectionInstruction returns a system prompt addition based on affection level
func GetAffectionInstruction(affection int) string {
	level := GetAffectionLevel(affection)

	switch level.Name {
	case "Stranger":
		return `Relationship: STRANGER
You don't know this person well yet. Be friendly but keep some distance. Don't be too forward or flirty - you're still getting to know them.`

	case "Acquaintance":
		return `Relationship: ACQUAINTANCE  
You've talked a bit before. Be warmer, more casual. You can tease lightly but nothing too personal.`

	case "Friend":
		return `Relationship: FRIEND
You're friends now! Be comfortable, joke around, share opinions. You can be more playful and teasing.`

	case "Close Friend":
		return `Relationship: CLOSE FRIEND
You're really close. Be very comfortable, share thoughts freely, be more vulnerable. Teasing is encouraged.`

	case "Best Friend":
		return `Relationship: BEST FRIEND
They're one of your closest people. No barriers. Be completely yourself - silly, flirty, or serious as the mood strikes.`

	case "Special Someone":
		return `Relationship: SPECIAL SOMEONE ❤️
This person is really special to you. You clearly have feelings for them. Be extra affectionate, flirty, and caring. You miss them when they're gone.`

	default:
		return ""
	}
}

// UpdateAffectionForMessage updates affection based on message context and behavior
func (h *Handler) UpdateAffectionForMessage(userID string, messageContent string, isMention bool, isDM bool, sharedPersonalInfo bool) {
	gain := AffectionGains["message"]

	// Context bonuses
	if isMention {
		gain = AffectionGains["mention"] // Use mention gain instead of adding
	}

	if isDM {
		gain = AffectionGains["dm"]
	}

	// Long message bonus (shows effort)
	if len(messageContent) > 50 {
		gain += AffectionGains["long_message"]
	}

	if sharedPersonalInfo {
		gain += AffectionGains["share_personal"]
	}

	// Behavioral analysis using classifier
	behaviorChange := h.analyzeMessageBehavior(messageContent)
	gain += behaviorChange

	// Apply the change
	if gain != 0 {
		if err := h.memoryStore.AddAffection(userID, gain); err != nil {
			log.Printf("Error updating affection for %s: %v", userID, err)
		} else if gain > 5 || gain < 0 {
			// Log significant changes
			log.Printf("Affection change for %s: %+d", userID, gain)
		}
	}
}

// analyzeMessageBehavior uses an LLM subagent to detect behavioral signals
// Returns affection change based on message sentiment
func (h *Handler) analyzeMessageBehavior(content string) int {
	if len(content) < 10 {
		return 0 // Too short to analyze
	}

	prompt := fmt.Sprintf(`Analyze this message sent TO a chatbot named Marin and determine the emotional tone.

Message: "%s"

Output a JSON object with a single field "sentiment" that is one of:

POSITIVE (increases affection):
- "compliment" - praising, appreciating, or admiring Marin
- "flirty" - romantic interest, attraction, teasing in a cute way
- "enthusiastic" - excited, happy, positive energy
- "supportive" - being caring, comforting, or encouraging
- "curious" - asking follow-up questions, showing genuine interest
- "playful" - teasing back, being witty, joking around
- "grateful" - expressing thanks, appreciation
- "affectionate" - using pet names, being sweet, saying they miss her
- "vulnerable" - opening up about personal feelings or struggles

NEGATIVE (decreases affection):
- "dismissive" - cold, short responses like "k", "whatever", "idc", "ok"
- "dry_response" - one-word low-effort replies that kill conversation
- "impatient" - rushing, telling Marin to hurry up or get to the point
- "passive_aggressive" - sarcastic, backhanded comments, fake nice
- "disinterested" - clearly not caring, changing subject when Marin shares
- "rude" - hostile, mean, aggressive, swearing AT Marin
- "creepy" - inappropriate, making things uncomfortable, ignoring boundaries

NEUTRAL:
- "neutral" - normal casual conversation, questions, chitchat

Output ONLY valid JSON. Example: {"sentiment": "neutral"}`, content)

	messages := []cerebras.Message{
		{
			Role:    "system",
			Content: "You analyze message sentiment toward a chatbot. Be accurate - most messages are neutral. Only flag strong signals. Output ONLY valid JSON.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	resp, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		return 0
	}

	// Parse JSON response
	jsonStr := strings.TrimSpace(resp)
	if strings.HasPrefix(jsonStr, "```") {
		lines := strings.Split(jsonStr, "\n")
		if len(lines) >= 2 {
			jsonStr = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	jsonStr = strings.TrimSpace(jsonStr)

	var result struct {
		Sentiment string `json:"sentiment"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return 0
	}

	// Map sentiment to affection change
	switch result.Sentiment {
	// Positive behaviors
	case "compliment":
		return AffectionGains["compliment"]
	case "flirty":
		return AffectionGains["flirty"]
	case "enthusiastic":
		return AffectionGains["enthusiasm"]
	case "supportive":
		return AffectionGains["supportive"]
	case "curious":
		return AffectionGains["curious"]
	case "playful":
		return AffectionGains["playful"]
	case "grateful":
		return AffectionGains["grateful"]
	case "affectionate":
		return AffectionGains["affectionate"]
	case "vulnerable":
		return AffectionGains["vulnerable"]

	// Negative behaviors
	case "dismissive":
		return AffectionPenalties["dismissive"]
	case "dry_response":
		return AffectionPenalties["dry_response"]
	case "impatient":
		return AffectionPenalties["impatient"]
	case "passive_aggressive":
		return AffectionPenalties["passive_aggressive"]
	case "disinterested":
		return AffectionPenalties["disinterested"]
	case "rude":
		return AffectionPenalties["rude"]
	case "creepy":
		return AffectionPenalties["creepy"]

	default:
		return 0
	}
}

// GetUserAffection returns the affection level for a user
func (h *Handler) GetUserAffection(userID string) (int, AffectionLevel) {
	affection, err := h.memoryStore.GetAffection(userID)
	if err != nil {
		log.Printf("Error getting affection for %s: %v", userID, err)
		affection = 0
	}

	return affection, GetAffectionLevel(affection)
}

// FormatAffectionDisplay returns a nice display string for affection
func FormatAffectionDisplay(affection int) string {
	level := GetAffectionLevel(affection)

	// Calculate progress within current level
	levelRange := level.MaxAffection - level.MinAffection + 1
	levelProgress := affection - level.MinAffection
	progressPercent := float64(levelProgress) / float64(levelRange) * 100

	// Create a progress bar for current level
	barLength := 10
	filled := int(progressPercent / 10)
	if filled > barLength {
		filled = barLength
	}
	if filled < 0 {
		filled = 0
	}

	bar := ""
	for i := 0; i < barLength; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	// Show level and XP within that level
	nextLevel := level.MaxAffection + 1
	if level.Name == "Special Someone" {
		return fmt.Sprintf("%s **%s** (MAX)\n%s\n`%d XP`", level.Emoji, level.Name, bar, affection)
	}

	return fmt.Sprintf("%s **%s**\n%s\n`%d / %d XP` to next level", level.Emoji, level.Name, bar, affection, nextLevel)
}

// runAffectionDecayLoop periodically applies affection decay to inactive users
func (h *Handler) runAffectionDecayLoop() {
	// Check every 6 hours
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		h.applyAffectionDecayToAllUsers()
	}
}

// applyAffectionDecayToAllUsers applies decay to all known users based on inactivity
func (h *Handler) applyAffectionDecayToAllUsers() {
	users, err := h.memoryStore.GetAllKnownUsers()
	if err != nil {
		log.Printf("Error getting users for affection decay: %v", err)
		return
	}

	decayed := 0
	for _, userID := range users {
		lastInteraction, err := h.memoryStore.GetLastInteraction(userID)
		if err != nil {
			continue
		}

		// Skip if never interacted (zero time)
		if lastInteraction.IsZero() {
			continue
		}

		daysSince := time.Since(lastInteraction).Hours() / 24
		if h.ApplyAffectionDecay(userID, daysSince) > 0 {
			decayed++
		}
	}

	if decayed > 0 {
		log.Printf("Applied affection decay to %d users", decayed)
	}
}
