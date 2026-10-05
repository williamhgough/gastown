package beads

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	readableSlugMaxWords = 5
	readableSlugMaxChars = 34
	readableMaxSuffix    = 99
)

var (
	// linearTicketPattern matches a tracker ticket such as ENG-3243 as a whole value.
	linearTicketPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{1,9}-[0-9]{1,7}$`)
	// linearTicketInTitle finds the first ticket in free text.
	linearTicketInTitle = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]{1,9}-[0-9]{1,7}\b`)
	readablePrefix      = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	nonAlnum            = regexp.MustCompile(`[^a-z0-9]+`)
)

var readableStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "to": true, "of": true, "for": true,
	"in": true, "on": true, "at": true, "and": true, "or": true, "is": true,
	"are": true, "be": true, "with": true, "from": true, "by": true,
	"that": true, "this": true, "it": true,
}

// ReadableID builds a bead ID a person can follow at a glance:
// <prefix>-<ticket>-<slug>, for example cf-eng-3243-fix-propelauth-refresh.
//
// The ticket is the tracker ID (ENG-3243). When ticket is empty, the first
// ticket found in the title is used. With no ticket the ID is <prefix>-<slug>.
// taken reports whether an ID already exists; a taken ID gets a -2, -3, ...
// suffix. A lookup error is returned, never treated as "free".
func ReadableID(prefix, ticket, title string, taken func(id string) (bool, error)) (string, error) {
	if !readablePrefix.MatchString(prefix) {
		return "", fmt.Errorf("bead ID prefix %q must be lowercase letters and digits", prefix)
	}

	chosenTicket := ticket
	if chosenTicket == "" {
		chosenTicket = linearTicketInTitle.FindString(title)
	} else if !linearTicketPattern.MatchString(chosenTicket) {
		return "", fmt.Errorf("ticket %q is not a tracker ID like ENG-3243", ticket)
	}

	slug := readableSlug(stripTicket(title, chosenTicket))
	if chosenTicket == "" && slug == "" {
		return "", fmt.Errorf("title %q has no words to build an ID from; pass --ticket or a clearer title", title)
	}

	parts := []string{prefix}
	if chosenTicket != "" {
		parts = append(parts, strings.ToLower(chosenTicket))
	}
	if slug != "" {
		parts = append(parts, slug)
	}
	base := strings.Join(parts, "-")

	for n := 1; n <= readableMaxSuffix; n++ {
		id := base
		if n > 1 {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		exists, err := taken(id)
		if err != nil {
			return "", fmt.Errorf("checking whether bead ID %q exists: %w", id, err)
		}
		if !exists {
			return id, nil
		}
	}
	return "", fmt.Errorf("bead ID %q and its first %d suffixes are all taken", base, readableMaxSuffix)
}

// stripTicket removes the chosen ticket from the title so it is not repeated in the slug.
func stripTicket(title, ticket string) string {
	if ticket == "" {
		return title
	}
	return regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(ticket)+`\b`).ReplaceAllString(title, " ")
}

// readableSlug lowercases the title, drops filler words, and keeps whole words
// until the word or length limit. The first word is kept even when it alone
// passes the length limit, cut to fit.
func readableSlug(title string) string {
	var kept []string
	length := 0
	for _, word := range strings.Fields(nonAlnum.ReplaceAllString(strings.ToLower(title), " ")) {
		if readableStopwords[word] {
			continue
		}
		if len(kept) == readableSlugMaxWords {
			break
		}
		next := length + len(word)
		if len(kept) > 0 {
			next++
		}
		if next > readableSlugMaxChars {
			if len(kept) == 0 {
				kept = append(kept, word[:readableSlugMaxChars])
			}
			break
		}
		kept = append(kept, word)
		length = next
	}
	return strings.Join(kept, "-")
}
