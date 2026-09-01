package flags

import (
	"strconv"
	"strings"
)

type semver struct {
	major, minor, patch int64
	pre                 []string
}

// parseSemver parses semver 2.0 leniently: optional "v" prefix, 1–3
// numeric components, optional -prerelease, +build ignored.
func parseSemver(s string) (semver, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "v"))
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre, s = s[i+1:], s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return semver{}, false
	}
	var nums [3]int64
	for i, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return semver{}, false
		}
		nums[i] = n
	}
	v := semver{major: nums[0], minor: nums[1], patch: nums[2]}
	if pre != "" {
		v.pre = strings.Split(pre, ".")
	}
	return v, true
}

// compareSemver returns -1, 0 or 1 following semver 2.0 precedence (§11).
func compareSemver(a, b semver) int {
	if c := cmpInt(a.major, b.major); c != 0 {
		return c
	}
	if c := cmpInt(a.minor, b.minor); c != 0 {
		return c
	}
	if c := cmpInt(a.patch, b.patch); c != 0 {
		return c
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1 // release > prerelease
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		ai, bi := a.pre[i], b.pre[i]
		an, aNum := strconv.ParseInt(ai, 10, 64)
		bn, bNum := strconv.ParseInt(bi, 10, 64)
		switch {
		case aNum == nil && bNum == nil:
			if c := cmpInt(an, bn); c != 0 {
				return c
			}
		case aNum == nil:
			return -1 // numeric < alphanumeric
		case bNum == nil:
			return 1
		default:
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
		}
	}
	return cmpInt(int64(len(a.pre)), int64(len(b.pre)))
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func matchSemver(op Op, v, ref string) bool {
	av, ok := parseSemver(v)
	if !ok {
		return false
	}
	bv, ok := parseSemver(ref)
	if !ok {
		return false
	}
	c := compareSemver(av, bv)
	switch op {
	case SemverGt:
		return c > 0
	case SemverGte:
		return c >= 0
	case SemverLt:
		return c < 0
	case SemverLte:
		return c <= 0
	}
	return false
}
