package packages

import (
	"fmt"
	"strings"
)

// CompareVersions implements semantic-version precedence without converting
// numeric components to fixed-width integers. Build metadata has no precedence.
func CompareVersions(a, b string) (int, error) {
	if !validVersion(a) || !validVersion(b) {
		return 0, fmt.Errorf("valid semantic versions required")
	}
	split := func(s string) ([]string, []string) {
		s = strings.SplitN(s, "+", 2)[0]
		parts := strings.SplitN(s, "-", 2)
		if len(parts) == 1 {
			return strings.Split(parts[0], "."), nil
		}
		return strings.Split(parts[0], "."), strings.Split(parts[1], ".")
	}
	ac, ap := split(a)
	bc, bp := split(b)
	number := func(a, b string) int {
		if len(a) < len(b) {
			return -1
		}
		if len(a) > len(b) {
			return 1
		}
		return strings.Compare(a, b)
	}
	for i := range ac {
		if c := number(ac[i], bc[i]); c != 0 {
			return c, nil
		}
	}
	if ap == nil && bp == nil {
		return 0, nil
	}
	if ap == nil {
		return 1, nil
	}
	if bp == nil {
		return -1, nil
	}
	for i := 0; i < len(ap) && i < len(bp); i++ {
		an := strings.Trim(ap[i], "0123456789") == ""
		bn := strings.Trim(bp[i], "0123456789") == ""
		var c int
		switch {
		case an && bn:
			c = number(ap[i], bp[i])
		case an:
			c = -1
		case bn:
			c = 1
		default:
			c = strings.Compare(ap[i], bp[i])
		}
		if c != 0 {
			return c, nil
		}
	}
	if len(ap) < len(bp) {
		return -1, nil
	}
	if len(ap) > len(bp) {
		return 1, nil
	}
	return 0, nil
}
func (m Manifest) CheckHandVersion(version string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	comparison, err := CompareVersions(strings.TrimPrefix(version, "v"), m.Compatibility.MinimumHand)
	if err != nil {
		return fmt.Errorf("cannot verify Hand compatibility for %q: %w", version, err)
	}
	if comparison < 0 {
		return fmt.Errorf("package %s requires Hand >= %s; selected %s", m.Name, m.Compatibility.MinimumHand, version)
	}
	return nil
}
