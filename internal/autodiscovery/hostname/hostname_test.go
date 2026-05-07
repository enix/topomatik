package hostname

import (
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExtractHostnameData(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		pattern  string
		want     map[string]string
	}{
		{
			name:     "no pattern returns only value",
			hostname: "node-eu-rack1-7",
			pattern:  "",
			want:     map[string]string{"value": "node-eu-rack1-7"},
		},
		{
			name:     "named submatches are extracted",
			hostname: "node-eu-rack1-7",
			pattern:  `^(?P<role>[a-z]+)-(?P<zone>[a-z]+)-(?P<rack>[a-z0-9]+)-(?P<id>[0-9]+)$`,
			want: map[string]string{
				"value": "node-eu-rack1-7",
				"role":  "node",
				"zone":  "eu",
				"rack":  "rack1",
				"id":    "7",
			},
		},
		{
			name:     "pattern that does not match leaves only value",
			hostname: "totally-different",
			pattern:  `^(?P<zone>[a-z]+)-r-(?P<id>[0-9]+)$`,
			want:     map[string]string{"value": "totally-different"},
		},
		{
			name:     "anonymous groups are ignored",
			hostname: "abc-42",
			pattern:  `^([a-z]+)-(?P<id>[0-9]+)$`,
			want: map[string]string{
				"value": "abc-42",
				"id":    "42",
			},
		},
		{
			name:     "empty hostname with permissive pattern",
			hostname: "",
			pattern:  `^(?P<zone>.*)$`,
			want: map[string]string{
				"value": "",
				"zone":  "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p *regexp.Regexp
			if tt.pattern != "" {
				p = regexp.MustCompile(tt.pattern)
			}
			got := extractHostnameData(tt.hostname, p)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("extractHostnameData (-want +got):\n%s", diff)
			}
		})
	}
}
