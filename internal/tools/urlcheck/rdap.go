package urlcheck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// The sentences shown in place of a registration date. An unknown age is normal
// and is never a finding.
const (
	ageSkipped = "Domain age was not looked up."
	ageIsIP    = "A bare IP address has no registration date to look up."
	ageUnknown = "The age of this domain could not be looked up. Many country domains do not publish it, and the lookup service is not always available. On its own this means nothing either way."
)

// rdapRedirects is how many times the age lookup follows a redirect of its own.
// rdap.org is a front end that answers with a 302 pointing at the registry that
// actually holds the record, and the shared client never follows anything by
// itself.
const rdapRedirects = 3

// age decides whether the destination's registration date is worth asking for,
// and says plainly when it is not.
func (c *Client) age(ctx context.Context, p Params, final HostName, now time.Time) Age {
	if p.SkipAge {
		return Age{Note: ageSkipped}
	}
	if final.IsIP {
		return Age{Note: ageIsIP}
	}
	return c.LookupAge(ctx, final.Registrable, now)
}

// LookupAge asks rdap.org when the registrable domain was registered. It is best
// effort: many country domains publish nothing and the service can be down.
// An unknown answer is normal and is never a finding.
func (c *Client) LookupAge(ctx context.Context, domain string, now time.Time) Age {
	unknown := Age{Note: ageUnknown}
	if domain == "" {
		return unknown
	}

	ctx, cancel := context.WithTimeout(ctx, rdapTimeout)
	defer cancel()

	target := c.RDAPBase + domain
	for i := 0; i <= rdapRedirects; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return unknown
		}
		req.Header.Set("Accept", "application/rdap+json")
		req.Header.Set("User-Agent", userAgent)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return unknown
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()

		if location := resp.Header.Get("Location"); resp.StatusCode >= 300 && resp.StatusCode <= 399 && location != "" {
			next, err := resp.Request.URL.Parse(location)
			if err != nil {
				return unknown
			}
			target = next.String()
			continue
		}
		if resp.StatusCode != http.StatusOK || readErr != nil {
			return unknown
		}
		return ageFrom(body, now, unknown)
	}
	return unknown
}

func ageFrom(body []byte, now time.Time, unknown Age) Age {
	registered, ok := parseRDAP(body)
	if !ok || registered.After(now) {
		return unknown
	}
	days := int(now.Sub(registered).Hours() / 24)
	return Age{
		Known:      true,
		Registered: registered.Format("2006-01-02"),
		Days:       days,
		Human:      humanAge(days),
	}
}

// parseRDAP pulls the registration date out of an RDAP domain object. Pure.
func parseRDAP(body []byte) (time.Time, bool) {
	var doc struct {
		Events []struct {
			EventAction string `json:"eventAction"`
			EventDate   string `json:"eventDate"`
		} `json:"events"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return time.Time{}, false
	}
	for _, event := range doc.Events {
		if strings.ToLower(event.EventAction) != "registration" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02"} {
			if when, err := time.Parse(layout, event.EventDate); err == nil {
				return when, true
			}
		}
		return time.Time{}, false
	}
	return time.Time{}, false
}
