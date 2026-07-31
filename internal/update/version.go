package update

import (
	"fmt"
	"strconv"
	"strings"
)

type semanticVersion struct {
	core       [3]uint64
	prerelease []string
}

func parseSemanticVersion(raw string) (semanticVersion, error) {
	var parsed semanticVersion
	value := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if value == "" {
		return parsed, fmt.Errorf("version is empty")
	}
	if build := strings.IndexByte(value, '+'); build >= 0 {
		value = value[:build]
	}
	if pre := strings.IndexByte(value, '-'); pre >= 0 {
		if pre == len(value)-1 {
			return parsed, fmt.Errorf("version %q has an empty prerelease", raw)
		}
		parsed.prerelease = strings.Split(value[pre+1:], ".")
		value = value[:pre]
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return parsed, fmt.Errorf("version %q must contain major.minor.patch", raw)
	}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return parsed, fmt.Errorf("version %q has an invalid numeric component", raw)
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return parsed, fmt.Errorf("version %q has an invalid numeric component: %w", raw, err)
		}
		parsed.core[i] = number
	}
	for _, identifier := range parsed.prerelease {
		if identifier == "" {
			return parsed, fmt.Errorf("version %q has an empty prerelease identifier", raw)
		}
		for _, character := range identifier {
			if (character < '0' || character > '9') &&
				(character < 'A' || character > 'Z') &&
				(character < 'a' || character > 'z') &&
				character != '-' {
				return parsed, fmt.Errorf("version %q has an invalid prerelease identifier", raw)
			}
		}
		if isNumericIdentifier(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return parsed, fmt.Errorf("version %q has a prerelease numeric identifier with a leading zero", raw)
		}
	}
	return parsed, nil
}

func compareSemanticVersions(leftRaw, rightRaw string) (int, error) {
	left, err := parseSemanticVersion(leftRaw)
	if err != nil {
		return 0, err
	}
	right, err := parseSemanticVersion(rightRaw)
	if err != nil {
		return 0, err
	}
	for index := range left.core {
		if left.core[index] < right.core[index] {
			return -1, nil
		}
		if left.core[index] > right.core[index] {
			return 1, nil
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
		return 0, nil
	}
	if len(left.prerelease) == 0 {
		return 1, nil
	}
	if len(right.prerelease) == 0 {
		return -1, nil
	}
	for index := 0; index < len(left.prerelease) && index < len(right.prerelease); index++ {
		leftID, rightID := left.prerelease[index], right.prerelease[index]
		leftNumeric, rightNumeric := isNumericIdentifier(leftID), isNumericIdentifier(rightID)
		switch {
		case leftNumeric && rightNumeric:
			leftNumber, _ := strconv.ParseUint(leftID, 10, 64)
			rightNumber, _ := strconv.ParseUint(rightID, 10, 64)
			if leftNumber < rightNumber {
				return -1, nil
			}
			if leftNumber > rightNumber {
				return 1, nil
			}
		case leftNumeric:
			return -1, nil
		case rightNumeric:
			return 1, nil
		default:
			if leftID < rightID {
				return -1, nil
			}
			if leftID > rightID {
				return 1, nil
			}
		}
	}
	switch {
	case len(left.prerelease) < len(right.prerelease):
		return -1, nil
	case len(left.prerelease) > len(right.prerelease):
		return 1, nil
	default:
		return 0, nil
	}
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
