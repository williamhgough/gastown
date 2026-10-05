package beads

import (
	"errors"
	"testing"
)

func neverTaken(string) (bool, error) { return false, nil }

func takenSet(ids ...string) func(string) (bool, error) {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return func(id string) (bool, error) { return set[id], nil }
}

func TestReadableID(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		ticket string
		title  string
		taken  func(string) (bool, error)
		want   string
	}{
		{
			name:   "ticket flag leads the ID",
			prefix: "cf",
			ticket: "ENG-3243",
			title:  "Fix the PropelAuth refresh on 4xx",
			want:   "cf-eng-3243-fix-propelauth-refresh-4xx",
		},
		{
			name:   "no ticket gives a slug only",
			prefix: "cf",
			title:  "Fix the PropelAuth refresh on 4xx",
			want:   "cf-fix-propelauth-refresh-4xx",
		},
		{
			name:   "ticket found in the title when no flag is given",
			prefix: "hq",
			title:  "ENG-3243: Customer dashboard token refresh error",
			want:   "hq-eng-3243-customer-dashboard-token-refresh",
		},
		{
			name:   "flag wins over a ticket in the title",
			prefix: "hq",
			ticket: "ENG-1",
			title:  "ENG-2 something else",
			want:   "hq-eng-1-eng-2-something-else",
		},
		{
			name:   "slug stops at the word limit",
			prefix: "cf",
			title:  "one two three four five six seven eight",
			want:   "cf-one-two-three-four-five",
		},
		{
			name:   "slug stops before it passes the length cap",
			prefix: "cf",
			title:  "authentication authorization infrastructure documentation",
			want:   "cf-authentication-authorization",
		},
		{
			name:   "punctuation and non-ASCII are dropped",
			prefix: "cf",
			title:  "Bearer: token endpoint (Plaid/Stripe) éè",
			want:   "cf-bearer-token-endpoint-plaid-stripe",
		},
		{
			name:   "collision gets a numeric suffix",
			prefix: "cf",
			title:  "Fix the login",
			taken:  takenSet("cf-fix-login", "cf-fix-login-2"),
			want:   "cf-fix-login-3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taken := tt.taken
			if taken == nil {
				taken = neverTaken
			}
			got, err := ReadableID(tt.prefix, tt.ticket, tt.title, taken)
			if err != nil {
				t.Fatalf("ReadableID() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadableID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadableIDRejects(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		ticket string
		title  string
	}{
		{name: "empty prefix", prefix: "", title: "Fix the login"},
		{name: "title with no usable words and no ticket", prefix: "cf", title: "the of -- !!"},
		{name: "malformed ticket", prefix: "cf", ticket: "3243", title: "Fix the login"},
		{name: "ticket with a path in it", prefix: "cf", ticket: "ENG-3243/../x", title: "Fix the login"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := ReadableID(tt.prefix, tt.ticket, tt.title, neverTaken); err == nil {
				t.Errorf("ReadableID() = %q, want an error", got)
			}
		})
	}
}

func TestReadableIDTicketOnlyIsAllowed(t *testing.T) {
	got, err := ReadableID("cf", "ENG-3243", "the of", neverTaken)
	if err != nil {
		t.Fatalf("ReadableID() error = %v", err)
	}
	if want := "cf-eng-3243"; got != want {
		t.Errorf("ReadableID() = %q, want %q", got, want)
	}
}

func TestReadableIDPropagatesLookupError(t *testing.T) {
	lookupErr := errors.New("dolt unavailable")
	_, err := ReadableID("cf", "", "Fix the login", func(string) (bool, error) { return false, lookupErr })
	if !errors.Is(err, lookupErr) {
		t.Errorf("ReadableID() error = %v, want it to wrap %v", err, lookupErr)
	}
}
