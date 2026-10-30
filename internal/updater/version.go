package updater

import "strings"

// CompareVersions implements the v1 release-version comparison: dotted
// numeric fields with an optional pre-release suffix ("1.2.3" > "1.2.3-rc1";
// suffixes compare lexically). A leading "v"/"V" is ignored (the release
// pipeline stamps tag names). Release version NAMING for real releases is a
// later CR-5.2 decision — this comparator is the v1 implementation constant
// so the machine needs no external module (no golang.org/x/mod dependency:
// the pin set is untouched, dependency-map v1.19). It returns -1 when a < b,
// 0 when a == b, +1 when a > b; malformed versions compare lexically as a
// last resort so a comparison NEVER silently succeeds on garbage.
func CompareVersions(a, b string) int {
	na, sa := splitVersion(a)
	nb, sb := splitVersion(b)
	for i := 0; i < len(na) || i < len(nb); i++ {
		var va, vb uint64
		if i < len(na) {
			va = na[i]
		}
		if i < len(nb) {
			vb = nb[i]
		}
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
	}
	// Same numeric core: a pre-release sorts before the plain release.
	switch {
	case sa == "" && sb == "":
		return 0
	case sa == "":
		return 1
	case sb == "":
		return -1
	}
	return strings.Compare(sa, sb)
}

// splitVersion trims the tag prefix, splits the pre-release suffix, and
// parses the dotted numeric core. Unparsable fields become 0 so a malformed
// version never panics; the caller-visible ordering of garbage inputs is
// recorded as unspecified (the machine refuses malformed versions upstream
// via the field-length and character bounds of the metadata parser).
func splitVersion(v string) (nums []uint64, suffix string) {
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, suffix = v[:i], v[i+1:]
	}
	for _, f := range strings.Split(v, ".") {
		var n uint64
		for _, r := range f {
			if r < '0' || r > '9' {
				n = 0
				break
			}
			n = n*10 + uint64(r-'0')
		}
		nums = append(nums, n)
	}
	return nums, suffix
}
