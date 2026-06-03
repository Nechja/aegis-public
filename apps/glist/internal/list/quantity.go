package list

import (
	"regexp"
	"strconv"
	"strings"
)

// word numerals → numeric value
var wordNum = map[string]float64{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"eleven": 11, "twelve": 12, "fifteen": 15, "twenty": 20,
	"dozen": 12, "couple": 2, "pair": 2,
	"half": 0.5, "quarter": 0.25,
}

// fuzzy quantifiers → canonical label. Stored as count=0 + unit=<label>.
var fuzzyWords = map[string]string{
	"some":    "some",
	"many":    "many",
	"lots":    "lots",
	"lot":     "lots",
	"several": "several",
	"few":     "a few",
	"handful": "a handful",
	"plenty":  "plenty",
	"tons":    "tons",
	"most":    "most",
}

// recognized units. empty means "each" (plain count).
var unitSet = map[string]string{
	"lb": "lb", "lbs": "lbs", "pound": "lb", "pounds": "lbs",
	"oz": "oz", "ounce": "oz", "ounces": "oz",
	"kg": "kg", "g": "g", "gram": "g", "grams": "g",
	"ml": "ml", "l": "l", "liter": "l", "liters": "l",
	"cup": "cup", "cups": "cups",
	"tbsp": "tbsp", "tsp": "tsp",
	"pack": "pack", "packs": "packs", "pkg": "pkg", "pkgs": "pkg",
	"bottle": "bottle", "bottles": "bottles",
	"can": "can", "cans": "cans",
	"bag": "bag", "bags": "bags",
	"box": "box", "boxes": "boxes",
	"head": "head", "heads": "heads",
	"bunch": "bunch", "bunches": "bunches",
	"gallon": "gallon", "gallons": "gallons",
	"qt": "qt", "quart": "qt", "quarts": "qt",
	"pint": "pint", "pints": "pints",
	"dozen": "dozen",
	"stick": "stick", "sticks": "sticks",
	"loaf": "loaf", "loaves": "loaves",
}

var (
	// "<num>" token: 3, 1.5, 1/2
	numToken = `(?:\d+\s*/\s*\d+|\d+(?:\.\d+)?)`
	// leading "x3" or "x 3" prefix
	xLeadRe = regexp.MustCompile(`^\s*x\s*(` + numToken + `)\s+(.+)$`)
	// "3x" or "3 x" suffix multiplier
	xTrailRe = regexp.MustCompile(`^\s*(` + numToken + `)\s*x\s+(.+)$`)
	// plain leading number
	numOnlyRe = regexp.MustCompile(`^\s*(` + numToken + `)\s+(.+)$`)
	// leading article
	articleRe = regexp.MustCompile(`^\s*(?i:a|an)\s+`)
)

// ParseQuantity extracts count + unit from the start of raw input.
// Returns (count, unit, remainingText).
//   - count=1, unit=""            → no quantity specified (e.g. "coke")
//   - count=N, unit=""            → plain count ("3 onions" → 3,"")
//   - count=N, unit="lbs"         → measured ("2 lbs chicken")
//   - count=0, unit="some"        → fuzzy quantifier ("some apples")
func ParseQuantity(raw string) (count float64, unit, text string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 1, "", ""
	}

	// fuzzy quantifier path — must run before the generic paths
	if c, u, t, ok := tryFuzzy(s); ok {
		return c, u, t
	}

	// x3 / x 3
	if m := xLeadRe.FindStringSubmatch(s); m != nil {
		c := parseNum(m[1])
		return c, "", strings.TrimSpace(m[2])
	}
	// 3x / 3 x
	if m := xTrailRe.FindStringSubmatch(s); m != nil {
		c := parseNum(m[1])
		return c, "", strings.TrimSpace(m[2])
	}
	// "3 lbs chicken" → num + unit word + rest
	if m := numOnlyRe.FindStringSubmatch(s); m != nil {
		c := parseNum(m[1])
		rest := strings.TrimSpace(m[2])
		// does the next word look like a unit?
		if unit, more, ok := takeUnit(rest); ok {
			return c, unit, more
		}
		// "a dozen eggs"-style: "1 dozen eggs" is valid but numOnlyRe also catches
		// "3 dozen eggs" — unit=dozen captured above; count=3, unit=dozen
		return c, "", rest
	}
	// word numeral path: "three onions", "a dozen eggs", "a pair of shoes"
	fields := strings.Fields(s)
	if len(fields) >= 2 {
		first := strings.ToLower(strings.TrimRight(fields[0], ".,:;"))
		if q, ok := wordNum[first]; ok {
			rest := dropLeadingOf(fields[1:])
			if unit, more, ok := takeUnit(rest); ok {
				return q, unit, more
			}
			return q, "", rest
		}
		if (first == "a" || first == "an") && len(fields) >= 3 {
			second := strings.ToLower(strings.TrimRight(fields[1], ".,:;"))
			if q, ok := wordNum[second]; ok {
				rest := dropLeadingOf(fields[2:])
				if unit, more, ok := takeUnit(rest); ok {
					return q, unit, more
				}
				return q, "", rest
			}
		}
	}
	// no count; strip leading article if present
	return 1, "", strings.TrimSpace(articleRe.ReplaceAllString(s, ""))
}

func parseNum(raw string) float64 {
	t := strings.Join(strings.Fields(raw), "")
	if strings.Contains(t, "/") {
		parts := strings.SplitN(t, "/", 2)
		if len(parts) == 2 {
			a, errA := strconv.ParseFloat(parts[0], 64)
			b, errB := strconv.ParseFloat(parts[1], 64)
			if errA == nil && errB == nil && b != 0 {
				return a / b
			}
		}
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 1
	}
	return f
}

// tryFuzzy recognizes "some", "a few", "lots of", "a handful of", etc. at the
// start of s. Returns (0, fuzzy-label, rest, true) on match.
func tryFuzzy(s string) (float64, string, string, bool) {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0, "", "", false
	}
	first := strings.ToLower(strings.TrimRight(fields[0], ".,:;"))
	// "a few X", "a handful of X", "a lot of X"
	if (first == "a" || first == "an") && len(fields) >= 3 {
		second := strings.ToLower(strings.TrimRight(fields[1], ".,:;"))
		if label, ok := fuzzyWords[second]; ok {
			rest := fields[2:]
			if len(rest) >= 2 && strings.ToLower(rest[0]) == "of" {
				rest = rest[1:]
			}
			return 0, label, strings.TrimSpace(strings.Join(rest, " ")), true
		}
	}
	// "some X", "many X", "lots of X", "several X"
	if label, ok := fuzzyWords[first]; ok {
		rest := fields[1:]
		if len(rest) >= 2 && strings.ToLower(rest[0]) == "of" {
			rest = rest[1:]
		}
		return 0, label, strings.TrimSpace(strings.Join(rest, " ")), true
	}
	return 0, "", "", false
}

// dropLeadingOf removes a leading "of" token, then joins remaining fields.
func dropLeadingOf(fields []string) string {
	if len(fields) >= 2 && strings.ToLower(fields[0]) == "of" {
		fields = fields[1:]
	}
	return strings.TrimSpace(strings.Join(fields, " "))
}

// takeUnit consumes a leading unit token from s. Returns (unit, rest, ok).
func takeUnit(s string) (string, string, bool) {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return "", s, false
	}
	first := strings.ToLower(strings.TrimRight(fields[0], ".,:;"))
	if u, ok := unitSet[first]; ok {
		return u, strings.Join(fields[1:], " "), true
	}
	return "", s, false
}

// FormatQuantity produces a human-readable "×3" / "2 lbs" / "some" / "1/2 gallon".
// Returns empty string when count == 1 and unit is empty (nothing to show).
func FormatQuantity(count float64, unit string) string {
	if count == 1 && unit == "" {
		return ""
	}
	if count == 0 && unit != "" {
		return unit // fuzzy quantifier
	}
	n := formatNumber(count)
	if unit == "" {
		return "×" + n
	}
	return n + " " + unit
}

func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	// try common fractions
	switch f {
	case 0.25:
		return "1/4"
	case 0.5:
		return "1/2"
	case 0.75:
		return "3/4"
	case 1.5:
		return "1 1/2"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
