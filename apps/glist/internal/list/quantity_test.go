package list

import (
	"fmt"
	"testing"
)

func TestParseQuantity_All(t *testing.T) {
	cases := []struct {
		in      string
		wantCnt float64
		wantU   string
		wantT   string
	}{
		// numeric
		{"3 onions", 3, "", "onions"},
		{"three onions", 3, "", "onions"},
		{"three onionx", 3, "", "onionx"},
		{"x3 onions", 3, "", "onions"},
		{"3x onions", 3, "", "onions"},
		{"onion", 1, "", "onion"},
		{"2 lbs chicken", 2, "lbs", "chicken"},
		{"1/2 gallon milk", 0.5, "gallon", "milk"},
		{"a dozen eggs", 12, "", "eggs"},
		{"an apple", 1, "", "apple"},
		{"coke", 1, "", "coke"},
		{"12 eggs", 12, "", "eggs"},
		{"a couple tomatoes", 2, "", "tomatoes"},
		{"a pair of shoes", 2, "", "shoes"},
		// fuzzy quantifiers
		{"some apples", 0, "some", "apples"},
		{"many onions", 0, "many", "onions"},
		{"lots of rice", 0, "lots", "rice"},
		{"a lot of tomatoes", 0, "lots", "tomatoes"},
		{"a few grapes", 0, "a few", "grapes"},
		{"a handful of nuts", 0, "a handful", "nuts"},
		{"several cookies", 0, "several", "cookies"},
		{"plenty of carrots", 0, "plenty", "carrots"},
		{"tons of garlic", 0, "tons", "garlic"},
		{"most of the cereal", 0, "most", "the cereal"},
	}
	for _, c := range cases {
		cnt, u, txt := ParseQuantity(c.in)
		status := "OK"
		if cnt != c.wantCnt || u != c.wantU || txt != c.wantT {
			status = "FAIL"
			t.Errorf("ParseQuantity(%q) = count=%v unit=%q text=%q; want count=%v unit=%q text=%q",
				c.in, cnt, u, txt, c.wantCnt, c.wantU, c.wantT)
		}
		fmt.Printf("%-5s %-22q → count=%-5v unit=%-10q text=%q\n", status, c.in, cnt, u, txt)
	}
}
