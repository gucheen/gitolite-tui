package gitolite

import (
	"reflect"
	"testing"
)

func TestParseInfo(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wanted []Repository
	}{
		{
			name: "standard tab separated output",
			input: `hello alice, this is gitolite3 v3.6.12 running on git 2.45.1

 R W	gitolite-admin
 R  	platform/api
   W	platform/write-only
`,
			wanted: []Repository{
				{Name: "gitolite-admin", Access: "R W"},
				{Name: "platform/api", Access: "R"},
				{Name: "platform/write-only", Access: "W"},
			},
		},
		{
			name:  "spaces CRLF duplicates and noise",
			input: "hello bob, this is gitolite3\r\n R W repo-z\r\n R repo-a\r\n R repo-a\r\nERROR\tbad-line\r\nDENIED by fallthru\r\n",
			wanted: []Repository{
				{Name: "repo-a", Access: "R"},
				{Name: "repo-z", Access: "R W"},
			},
		},
		{
			name:   "empty output",
			input:  "\nhello user\n",
			wanted: []Repository{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseInfo(test.input)
			if err != nil {
				t.Fatalf("ParseInfo returned error: %v", err)
			}
			if !reflect.DeepEqual(got, test.wanted) {
				t.Fatalf("ParseInfo() = %#v, want %#v", got, test.wanted)
			}
		})
	}
}

func TestCloneURL(t *testing.T) {
	client := Client{Host: "git.example.com", User: "git"}
	for input, wanted := range map[string]string{
		"team/api":     "git@git.example.com:team/api.git",
		"team/api.git": "git@git.example.com:team/api.git",
	} {
		if got := client.CloneURL(input); got != wanted {
			t.Errorf("CloneURL(%q) = %q, want %q", input, got, wanted)
		}
	}
}
