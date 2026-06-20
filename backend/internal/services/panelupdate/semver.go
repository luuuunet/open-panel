package panelupdate

import (
	"strconv"
	"strings"
)

func CompareVersions(a, b string) int {
	pa := parseVersionParts(a)
	pb := parseVersionParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(pa) {
			ai = pa[i]
		}
		if i < len(pb) {
			bi = pb[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(v), "v"))
	if v == "" || v == "dev" {
		return []int{0}
	}
	main := v
	pre := ""
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		main = v[:idx]
		pre = v[idx:]
	}
	parts := strings.Split(main, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		digits := strings.TrimLeftFunc(p, func(r rune) bool {
			return r < '0' || r > '9'
		})
		if digits == "" {
			out = append(out, 0)
			continue
		}
		n, _ := strconv.Atoi(digits)
		out = append(out, n)
	}
	if pre != "" {
		out = append(out, -1)
	}
	return out
}
