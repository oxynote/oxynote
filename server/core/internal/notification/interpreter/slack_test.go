package interpreter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewSlackFormatter(t *testing.T) {
	t.Parallel()

	require.NotNil(t, NewSlackFormatter())
}

func Test_SlackFormatter_Link(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		URL    string
		Text   string
		Result string
	}{
		"Plain link": {
			URL:    "https://app.test/doc",
			Text:   "My Doc",
			Result: "<https://app.test/doc|My Doc>",
		},
		"Mrkdwn control characters in the text are escaped": {
			URL:    "https://app.test/doc",
			Text:   "a <b> & c",
			Result: "<https://app.test/doc|a &lt;b&gt; &amp; c>",
		},
		"Pipe in the text is stripped": {
			URL:    "https://app.test/doc",
			Text:   "evil|https://phish.test",
			Result: "<https://app.test/doc|evilhttps://phish.test>",
		},
		"URL is escaped too": {
			URL:    "https://app.test/doc?a=1&b=<2>|x",
			Text:   "My Doc",
			Result: "<https://app.test/doc?a=1&amp;b=&lt;2&gt;x|My Doc>",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, NewSlackFormatter().Link(c.URL, c.Text))
		})
	}
}
