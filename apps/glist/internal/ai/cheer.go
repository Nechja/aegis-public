package ai

import (
	"context"
	"log"
	"math/rand/v2"
	"sync"
	"time"
)

type CheerKind string

const (
	CheerFirst       CheerKind = "first"
	CheerMilestone25 CheerKind = "milestone25"
	CheerMilestone50 CheerKind = "milestone50"
	CheerMilestone75 CheerKind = "milestone75"
	CheerComplete    CheerKind = "complete"
	CheerMissing     CheerKind = "missing"
)

// Seed pool — hardcoded fallbacks that also appear in rotation before Ollama
// fills the bowl with fresh variants.
var cheerSeed = map[CheerKind][]string{
	CheerFirst: {
		"let's gooo",
		"one down, many to go",
		"here we go",
		"first one's the hardest",
		"you got this",
	},
	CheerMilestone25: {
		"off to a great start",
		"quarter done, nice pace",
		"25% — keep it rolling",
		"good groove",
		"building momentum",
	},
	CheerMilestone50: {
		"halfway there!",
		"you're crushing this",
		"50% — rest is downhill",
		"momentum!",
		"the easy half is done",
	},
	CheerMilestone75: {
		"three quarters done",
		"almost home",
		"home stretch — nice",
		"you can see the finish",
		"75% — so close",
	},
	CheerComplete: {
		"shopping complete — nice work!",
		"all done, go home hero",
		"list conquered",
		"crushed it",
		"mission accomplished",
		"groceries: acquired",
	},
	CheerMissing: {
		"happens — try ⇄ for a sub?",
		"no worries, not every aisle has it",
		"bummer — maybe substitute?",
		"the shelves betray us sometimes",
		"it wasn't your fault",
	},
}

var KindPromptContext = map[CheerKind]string{
	CheerFirst:       "just checked off the very first item on their list",
	CheerMilestone25: "just hit 25% done with their shopping",
	CheerMilestone50: "just hit the halfway mark on their shopping",
	CheerMilestone75: "just hit 75% done — they're in the home stretch",
	CheerComplete:    "just finished their entire shopping list",
	CheerMissing:     "couldn't find an item and had to mark it as missing",
}

type Cheer struct {
	mu   sync.Mutex
	pool map[CheerKind][]string
	llm  *Ollama
}

func NewCheer(llm *Ollama) *Cheer {
	c := &Cheer{pool: map[CheerKind][]string{}, llm: llm}
	for k, v := range cheerSeed {
		c.pool[k] = append([]string{}, v...)
	}
	return c
}

// Pick returns a cheer for the given kind and triggers a background refill
// when the pool gets low. Pool never runs empty — falls back to a fresh copy
// of the seed when exhausted.
func (c *Cheer) Pick(k CheerKind) string {
	c.mu.Lock()
	p := c.pool[k]
	if len(p) == 0 {
		p = append([]string{}, cheerSeed[k]...)
	}
	if len(p) == 0 {
		c.mu.Unlock()
		return "nice"
	}
	idx := rand.IntN(len(p))
	pick := p[idx]
	c.pool[k] = append(p[:idx], p[idx+1:]...)
	needRefill := len(c.pool[k]) < 3
	c.mu.Unlock()

	if needRefill && c.llm != nil {
		go c.refill(k)
	}
	return pick
}

// refill fetches 5 fresh cheers from the LLM and appends to the pool.
// Non-blocking; errors just leave the seeded pool in place.
func (c *Cheer) refill(k CheerKind) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fresh, err := c.llm.GenerateCheers(ctx, k, 5)
	if err != nil || len(fresh) == 0 {
		return
	}
	c.mu.Lock()
	c.pool[k] = append(c.pool[k], fresh...)
	c.mu.Unlock()
	log.Printf("cheer refill %s: %d new (pool=%d)", k, len(fresh), len(c.pool[k]))
}

// Prewarm kicks off a background refill for every kind. Call once at boot
// so the first milestone the user hits is LLM-flavored rather than seed.
func (c *Cheer) Prewarm() {
	if c.llm == nil {
		return
	}
	for k := range cheerSeed {
		go c.refill(k)
	}
}

func IsValidKind(s string) bool {
	switch CheerKind(s) {
	case CheerFirst, CheerMilestone25, CheerMilestone50, CheerMilestone75,
		CheerComplete, CheerMissing:
		return true
	}
	return false
}
