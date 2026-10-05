package beads

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	readableKind        = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	bracketedTag        = regexp.MustCompile(`\[[^\]]{1,16}\]`)
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
	// "item(s)" is a plural, not the word "s"; a "[MEDIUM]" tag says nothing about the work.
	title = bracketedTag.ReplaceAllString(strings.ReplaceAll(title, "(s)", ""), " ")
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

// ReadableKindID builds <prefix>-<kind>-<slug> for beads that have no ticket,
// such as mail and escalations. Use kind "wisp" for ephemeral beads: the rest of
// Gas Town recognises a wisp by "-wisp-" in its ID, and flags a persistent bead
// that has it.
func ReadableKindID(prefix, kind, title string) (string, error) {
	if !readablePrefix.MatchString(prefix) {
		return "", fmt.Errorf("bead ID prefix %q must be lowercase letters and digits", prefix)
	}
	if !readableKind.MatchString(kind) {
		return "", fmt.Errorf("bead ID kind %q must be lowercase letters and digits", kind)
	}
	slug := readableSlug(title)
	if slug == "" {
		return "", fmt.Errorf("title %q has no words to build an ID from", title)
	}
	return prefix + "-" + kind + "-" + slug, nil
}

// IsDuplicateIDError reports whether err is bd refusing to create a bead because
// the ID is taken. bd never overwrites in that case.
func IsDuplicateIDError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// CreateUnderReadableID calls create with base, then base-2, base-3, then
// base-<time> when each is taken. If every one is taken it calls create with an
// empty ID, which means "let bd pick", so the caller still gets its bead. Only a
// duplicate-ID error moves to the next candidate; any other error is returned at
// once. It returns the ID that worked, or "" when bd picked.
//
// There is no existence check first: two writers can pick the same slug at the
// same moment, and bd's duplicate error is what settles the race.
func CreateUnderReadableID(base string, now time.Time, create func(id string) error) (string, error) {
	candidates := []string{
		base,
		base + "-2",
		base + "-3",
		base + "-" + strconv.FormatInt(now.Unix(), 36),
		"",
	}
	for _, id := range candidates {
		err := create(id)
		if err == nil {
			return id, nil
		}
		if !IsDuplicateIDError(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("creating bead under %q: every candidate ID was taken", base)
}
