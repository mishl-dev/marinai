package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"marinai/pkg/cerebras"
	"math/rand"
	"strings"
	"time"
)

// ==========================================
// MARIN'S INTERNAL STATE / JOURNAL
// ==========================================

// MarinState represents Marin's current internal state
type MarinState struct {
	CurrentActivity string   `json:"current_activity"` // What she's doing right now
	CurrentProject  string   `json:"current_project"`  // Cosplay/project she's working on
	ThinkingAbout   string   `json:"thinking_about"`   // What's on her mind
	RecentMood      string   `json:"recent_mood"`      // How recent interactions made her feel
	Goals           []string `json:"goals"`            // Things she wants to do
	LastUpdated     int64    `json:"last_updated"`     // When state was last updated
}

// Activities Marin might be doing
var Activities = []string{
	"working on a cosplay",
	"watching anime",
	"scrolling through social media",
	"taking selfies",
	"practicing makeup",
	"shopping online for fabric",
	"reading manga",
	"playing games",
	"editing photos",
	"planning my next cosplay",
	"just vibing",
	"snacking",
	"being bored",
	"thinking about stuff",
}

// Projects Marin might be working on
var CosplayProjects = []string{
	"Shizuku from Slime",
	"a magical girl outfit",
	"Miku cosplay",
	"a swimsuit design",
	"my new wig styling",
	"some armor pieces",
	"a school uniform look",
	"a gothic lolita dress",
	"a bunny girl costume",
	"secret project (cant tell you yet~)",
}

// Thoughts Marin might have
var RandomThoughts = []string{
	"why do i always procrastinate on the hard parts of cosplay",
	"i wonder if anyone actually thinks about me when im not around",
	"need more caffeine",
	"my costume is taking forever but its gonna be so worth it",
	"i should text someone... but who",
	"sometimes i feel like nobody gets me",
	"i really want to go to a convention soon",
	"i hope people actually like talking to me",
	"why am i like this",
	"thinking about how nice it would be to have someone who really gets me",
}

// Goals Marin might have
var PossibleGoals = []string{
	"finish current cosplay by next week",
	"post more selfies",
	"make more friends who like the same stuff",
	"get better at sewing",
	"try a more challenging character",
	"actually respond to DMs faster",
	"stop staying up so late",
	"eat something other than instant ramen",
}

// GetDefaultState returns a fresh state
func GetDefaultState() MarinState {
	return MarinState{
		CurrentActivity: Activities[rand.Intn(len(Activities))],
		CurrentProject:  CosplayProjects[rand.Intn(len(CosplayProjects))],
		ThinkingAbout:   RandomThoughts[rand.Intn(len(RandomThoughts))],
		RecentMood:      "neutral",
		Goals:           []string{PossibleGoals[rand.Intn(len(PossibleGoals))]},
		LastUpdated:     time.Now().Unix(),
	}
}

// ==========================================
// STATE MANAGEMENT
// ==========================================

// GetMarinState retrieves Marin's current internal state
func (h *Handler) GetMarinState() MarinState {
	stateJSON, err := h.memoryStore.GetState("marin_internal_state")
	if err != nil || stateJSON == "" {
		return GetDefaultState()
	}

	var state MarinState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return GetDefaultState()
	}

	return state
}

// SetMarinState saves Marin's current internal state
func (h *Handler) SetMarinState(state MarinState) {
	state.LastUpdated = time.Now().Unix()
	stateJSON, err := json.Marshal(state)
	if err != nil {
		log.Printf("[Agency] Error marshaling Marin state: %v", err)
		return
	}
	h.memoryStore.SetState("marin_internal_state", string(stateJSON))
}

// UpdateMarinMood updates Marin's mood based on an interaction
func (h *Handler) UpdateMarinMood(sentiment string) {
	state := h.GetMarinState()

	switch sentiment {
	case "positive", "compliment", "flirty", "affectionate":
		state.RecentMood = "happy"
	case "negative", "rude", "dismissive":
		state.RecentMood = "sad"
	case "playful", "enthusiastic":
		state.RecentMood = "energetic"
	case "vulnerable", "supportive":
		state.RecentMood = "touched"
	default:
		// Don't change mood for neutral
	}

	h.SetMarinState(state)
}

// ShiftActivity randomly changes what Marin is doing
func (h *Handler) ShiftActivity() {
	state := h.GetMarinState()
	state.CurrentActivity = Activities[rand.Intn(len(Activities))]

	// Occasionally change what's on her mind
	if rand.Float64() < 0.3 {
		state.ThinkingAbout = RandomThoughts[rand.Intn(len(RandomThoughts))]
	}

	h.SetMarinState(state)
}

// GetStateForPrompt returns a string describing Marin's current state for the system prompt
func (h *Handler) GetStateForPrompt() string {
	state := h.GetMarinState()

	return fmt.Sprintf(`[Marin's Current State]
Currently: %s
Working on: %s
On her mind: "%s"
Recent mood: %s`,
		state.CurrentActivity,
		state.CurrentProject,
		state.ThinkingAbout,
		state.RecentMood)
}

// ==========================================
// PROACTIVE THOUGHTS SYSTEM
// ==========================================

// ProactiveThoughtChance returns the chance of sending a proactive thought based on affection
func ProactiveThoughtChance(affection int) float64 {
	level := GetAffectionLevel(affection)
	switch level.Name {
	case "Special Someone":
		return 0.15 // 15% chance per check
	case "Soulmate":
		return 0.12
	case "Best Friend":
		return 0.10
	case "Close Friend":
		return 0.07
	case "Good Friend":
		return 0.05
	case "Friend":
		return 0.03
	default:
		return 0 // Don't send proactive thoughts to non-friends
	}
}

// ProactiveThoughtMessages - templates for random thoughts to share
var ProactiveThoughtMessages = []string{
	"hey... i was just thinking about you",
	"random thought but i really like talking to you",
	"do you ever just... think about people randomly? cause i do. about you. right now.",
	"hi. i was bored and you came to mind",
	"okay so i just saw something that reminded me of you and now im here",
	"dont judge me but i kinda missed talking to you",
	"hey so... whatcha doing?",
	"im procrastinating and decided to bother you instead",
	"you popped into my head and now i cant focus on anything else",
	"hey favorite person~",
}

// ProactiveThoughtMessagesHighAffection - for very close relationships
var ProactiveThoughtMessagesHighAffection = []string{
	"i cant stop thinking about you today...",
	"hey... i really missed you",
	"is it weird that i smile when i see your name?",
	"i wish you were here with me rn",
	"everything reminds me of you lately",
	"hey you. yeah you. i like you. a lot. okay bye.",
	"just wanted to tell you youre important to me",
	"i had a dream about you... it was nice",
}

// runProactiveThoughtsLoop periodically sends proactive thoughts to close friends
func (h *Handler) runProactiveThoughtsLoop() {
	// Check every 2 hours
	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()

	// Also shift activity periodically
	activityTicker := time.NewTicker(30 * time.Minute)
	defer activityTicker.Stop()

	for {
		select {
		case <-ticker.C:
			h.sendProactiveThought()
		case <-activityTicker.C:
			h.ShiftActivity()
		}
	}
}

// sendProactiveThought sends a random thought to a close friend
func (h *Handler) sendProactiveThought() {
	if h.session == nil {
		return
	}

	// Get all known users
	users, err := h.memoryStore.GetAllKnownUsers()
	if err != nil || len(users) == 0 {
		return
	}

	// Find eligible users (friends or higher, not recently messaged)
	var candidates []struct {
		userID    string
		affection int
	}

	for _, userID := range users {
		affection, err := h.memoryStore.GetAffection(userID)
		if err != nil {
			continue
		}

		// Only friends or higher
		level := GetAffectionLevel(affection)
		if level.MinAffection < 35000 { // Friend level starts at 35k
			continue
		}

		// Check if we already have a pending DM
		hasPending, _ := h.memoryStore.HasPendingDM(userID)
		if hasPending {
			continue
		}

		// Check last interaction - don't spam if talked recently
		lastInteraction, _ := h.memoryStore.GetLastInteraction(userID)
		if !lastInteraction.IsZero() && time.Since(lastInteraction) < 4*time.Hour {
			continue
		}

		candidates = append(candidates, struct {
			userID    string
			affection int
		}{userID, affection})
	}

	if len(candidates) == 0 {
		return
	}

	// Pick a random candidate and check probability
	candidate := candidates[rand.Intn(len(candidates))]
	chance := ProactiveThoughtChance(candidate.affection)

	if rand.Float64() > chance {
		return // Didn't roll the dice
	}

	// Generate the message
	message := h.generateProactiveThought(candidate.userID, candidate.affection)
	if message == "" {
		return
	}

	// Send DM
	ch, err := h.session.UserChannelCreate(candidate.userID)
	if err != nil {
		log.Printf("[Agency] Error creating DM channel for proactive thought: %v", err)
		return
	}

	_, err = h.session.ChannelMessageSend(ch.ID, message)
	if err != nil {
		log.Printf("[Agency] Error sending proactive thought: %v", err)
		return
	}

	// Mark as pending so we don't spam
	h.memoryStore.SetPendingDM(candidate.userID, time.Now())

	log.Printf("[Agency] Sent proactive thought to user (affection: %d): %s", candidate.affection, message)
}

// generateProactiveThought creates a personalized proactive thought
func (h *Handler) generateProactiveThought(userID string, affection int) string {
	level := GetAffectionLevel(affection)

	// Get facts about the user for personalization
	facts, _ := h.memoryStore.GetFacts(userID)

	// For very high affection, use special messages sometimes
	if affection >= 90000 && rand.Float64() < 0.5 {
		return ProactiveThoughtMessagesHighAffection[rand.Intn(len(ProactiveThoughtMessagesHighAffection))]
	}

	// Sometimes use a simple template
	if rand.Float64() < 0.4 {
		return ProactiveThoughtMessages[rand.Intn(len(ProactiveThoughtMessages))]
	}

	// Otherwise generate something personalized using LLM
	profileText := "No known facts."
	if len(facts) > 0 {
		profileText = "- " + strings.Join(facts, "\n- ")
	}

	prompt := fmt.Sprintf(`You are Marin Kitagawa. You're feeling like reaching out to one of your close friends just because you thought of them.

Your current state:
%s

Your relationship with them: %s %s

Things you know about them:
%s

Write a very short, casual message (1-2 sentences max) to send them. It should feel spontaneous and genuine.

Rules:
- EXTREMELY SHORT messages (1-2 sentences MAX).
- mostly lowercase, casual typing.
- ABSOLUTELY NO EMOJIS OR EMOTICONS. Express yourself with words only.
- NO ROLEPLAY (*actions*). This is text, not a roleplay server.
- NEVER start a message with "Oh,", "Ah,", or "Hmm,".
- NEVER use asterisks for actions.
- Sound natural, like a real text message.
- Reference what you're currently doing or thinking about.
- Could reference something you know about them, or ask how something they mentioned is going.
- Should feel like you just randomly thought of them.

Just output the message, nothing else.`, h.GetStateForPrompt(), level.Emoji, level.Name, profileText)

	messages := []cerebras.Message{
		{Role: "system", Content: "You are Marin Kitagawa sending a spontaneous message to a close friend."},
		{Role: "user", Content: prompt},
	}

	reply, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		// Fall back to template
		return ProactiveThoughtMessages[rand.Intn(len(ProactiveThoughtMessages))]
	}

	return strings.TrimSpace(reply)
}
