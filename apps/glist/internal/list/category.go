package list

import (
	"strings"
	"unicode"
)

var Categories = []string{
	"produce", "dairy", "meat", "pantry",
	"frozen", "bakery", "beverages", "snacks", "household", "other",
}

func IsValidCategory(c string) bool {
	for _, v := range Categories {
		if v == c {
			return true
		}
	}
	return false
}

var rule = map[string]string{
	// produce
	"apple": "produce", "apples": "produce", "avocado": "produce", "avocados": "produce",
	"banana": "produce", "bananas": "produce", "berry": "produce", "berries": "produce",
	"blueberry": "produce", "blueberries": "produce", "broccoli": "produce",
	"cabbage": "produce", "carrot": "produce", "carrots": "produce", "celery": "produce",
	"cilantro": "produce", "cucumber": "produce", "cucumbers": "produce",
	"garlic": "produce", "ginger": "produce", "grape": "produce", "grapes": "produce",
	"greens": "produce", "herb": "produce", "herbs": "produce", "kale": "produce",
	"lemon": "produce", "lemons": "produce", "lettuce": "produce", "lime": "produce", "limes": "produce",
	"mushroom": "produce", "mushrooms": "produce", "onion": "produce", "onions": "produce",
	"orange": "produce", "oranges": "produce", "parsley": "produce", "pear": "produce",
	"pepper": "produce", "peppers": "produce", "potato": "produce", "potatoes": "produce",
	"raspberry": "produce", "raspberries": "produce", "scallion": "produce", "scallions": "produce",
	"spinach": "produce", "strawberry": "produce", "strawberries": "produce",
	"tomato": "produce", "tomatoes": "produce", "watermelon": "produce", "zucchini": "produce",

	// dairy + eggs
	"butter": "dairy", "cheese": "dairy", "cheddar": "dairy", "cream": "dairy",
	"egg": "dairy", "eggs": "dairy", "feta": "dairy", "mozzarella": "dairy",
	"milk": "dairy", "parmesan": "dairy", "ricotta": "dairy", "sourcream": "dairy",
	"yogurt": "dairy", "yoghurt": "dairy",

	// meat + fish
	"bacon": "meat", "beef": "meat", "brisket": "meat", "chicken": "meat",
	"chops": "meat", "cod": "meat", "duck": "meat", "fish": "meat",
	"ground": "meat", "ham": "meat", "lamb": "meat", "meat": "meat",
	"mince": "meat", "pepperoni": "meat", "pork": "meat", "prosciutto": "meat",
	"salami": "meat", "salmon": "meat", "sausage": "meat", "sausages": "meat",
	"shrimp": "meat", "steak": "meat", "tuna": "meat", "turkey": "meat",

	// pantry
	"bean": "pantry", "beans": "pantry", "broth": "pantry", "chickpeas": "pantry",
	"cornmeal": "pantry", "cornstarch": "pantry", "flour": "pantry",
	"honey": "pantry", "ketchup": "pantry", "lentil": "pantry", "lentils": "pantry",
	"mustard": "pantry", "noodle": "pantry", "noodles": "pantry", "oats": "pantry",
	"oil": "pantry", "olive": "pantry", "pasta": "pantry", "peanut": "pantry",
	"quinoa": "pantry", "rice": "pantry", "salt": "pantry", "sauce": "pantry",
	"soy": "pantry", "spaghetti": "pantry", "spice": "pantry", "spices": "pantry",
	"stock": "pantry", "sugar": "pantry", "tortilla": "pantry", "tortillas": "pantry",
	"vinegar": "pantry", "yeast": "pantry",

	// frozen
	"frozen": "frozen", "icecream": "frozen", "ice-cream": "frozen",
	"peas": "frozen", "popsicle": "frozen",

	// bakery
	"bagel": "bakery", "bagels": "bakery", "baguette": "bakery", "bread": "bakery",
	"brioche": "bakery", "bun": "bakery", "buns": "bakery", "cake": "bakery",
	"croissant": "bakery", "donut": "bakery", "donuts": "bakery",
	"muffin": "bakery", "muffins": "bakery",
	"pita": "bakery", "roll": "bakery", "rolls": "bakery", "sourdough": "bakery",

	// beverages
	"beer": "beverages", "beverage": "beverages", "coffee": "beverages", "coke": "beverages",
	"cola": "beverages", "drink": "beverages", "energy": "beverages", "espresso": "beverages",
	"juice": "beverages", "kombucha": "beverages", "lemonade": "beverages",
	"lacroix": "beverages", "pepsi": "beverages", "seltzer": "beverages", "soda": "beverages",
	"sparkling": "beverages", "sprite": "beverages", "tea": "beverages", "water": "beverages",
	"wine": "beverages",

	// snacks
	"candy": "snacks", "chip": "snacks", "chips": "snacks", "chocolate": "snacks",
	"cookie": "snacks", "cookies": "snacks", "cracker": "snacks", "crackers": "snacks",
	"granola": "snacks", "gum": "snacks", "jerky": "snacks", "nut": "snacks",
	"nuts": "snacks", "popcorn": "snacks", "pretzel": "snacks", "pretzels": "snacks",
	"snack": "snacks",

	// household
	"battery": "household", "batteries": "household", "bleach": "household",
	"detergent": "household", "dish": "household", "floss": "household",
	"foil": "household", "laundry": "household", "lightbulb": "household",
	"napkin": "household", "napkins": "household",
	"paper": "household", "shampoo": "household", "soap": "household",
	"sponge": "household", "sponges": "household", "tissue": "household",
	"toothpaste": "household", "towel": "household", "trash": "household", "wipes": "household",
}

// Rule returns (category, true) if any token matches a known entry.
func Rule(text string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return "", false
	}
	fields := strings.FieldsFunc(t, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(".,!?:;\"'()[]{}/\\", r)
	})
	joined := strings.ReplaceAll(t, " ", "-")
	if c, ok := rule[joined]; ok {
		return c, true
	}
	for _, w := range fields {
		if c, ok := rule[w]; ok {
			return c, true
		}
	}
	for _, w := range fields {
		if len(w) > 3 && strings.HasSuffix(w, "s") {
			if c, ok := rule[w[:len(w)-1]]; ok {
				return c, true
			}
		}
	}
	return "", false
}
