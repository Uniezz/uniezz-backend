package auth

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	minUniversityNameLength = 3
	maxUniversityNameLength = 200
	minFacultyNameLength    = 3
	maxFacultyNameLength    = 200
	maxFacultyAddressLength = 200
	maxDomainLength         = 253
	maxDomains              = 20
	maxFaculties            = 50
)

var (
	universityIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	domainPattern       = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

func cleanUniversityID(id UniversityID) (UniversityID, error) {
	if !universityIDPattern.MatchString(string(id)) {
		return "", ErrInvalidInput
	}
	return id, nil
}

func cleanUniversityName(s string) (string, error) {
	s = text.Plain(s, false)
	if n := utf8.RuneCountInString(s); n < minUniversityNameLength || n > maxUniversityNameLength {
		return "", ErrInvalidInput
	}
	return s, nil
}

func cleanFacultyName(s string) (string, error) {
	s = text.Plain(s, false)
	if n := utf8.RuneCountInString(s); n < minFacultyNameLength || n > maxFacultyNameLength {
		return "", ErrInvalidInput
	}
	return s, nil
}

func cleanFacultyAddress(s string) (string, error) {
	s = text.Plain(s, false)
	if n := utf8.RuneCountInString(s); n < 1 || n > maxFacultyAddressLength {
		return "", ErrInvalidInput
	}
	return s, nil
}

func cleanDomains(domains []string) ([]string, error) {
	if len(domains) > maxDomains {
		return nil, ErrInvalidInput
	}
	clean := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if len(d) > maxDomainLength || !domainPattern.MatchString(d) {
			return nil, ErrInvalidInput
		}
		clean = append(clean, d)
	}
	slices.Sort(clean)
	return slices.Compact(clean), nil
}
