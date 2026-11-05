package icon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Find(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		ID     string
		Result bool
	}{
		"Offered icon":                         {ID: "mingcute:rocket-fill", Result: true},
		"Offered icon from another collection": {ID: "simple-icons:kubernetes", Result: true},
		"Unknown icon":                         {ID: "lucide:rocket"},
		"Empty id":                             {ID: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, Find(c.ID))
		})
	}
}

func Test_Entry_Words(t *testing.T) {
	t.Parallel()

	e := Entry{ID: "simple-icons:amazonwebservices", Name: "aws"}

	assert.Equal(t, []string{"aws", "amazonwebservices"}, e.Words())
}

func Test_extractWords(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		ID     string
		Result []string
	}{
		"Collection and fill":     {ID: "mingcute:shield-check-fill", Result: []string{"shield", "check"}},
		"Fluent size and style":   {ID: "fluent:cloud-sync-24-filled", Result: []string{"cloud", "sync"}},
		"Variant number":          {ID: "mingcute:rocket-2-fill", Result: []string{"rocket"}},
		"No collection":           {ID: "Rocket", Result: []string{"rocket"}},
		"Underscores and outline": {ID: "x:file_code-outline", Result: []string{"file", "code"}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, extractWords(c.ID))
		})
	}
}

func Test_words(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"let", "s", "encrypt", "node", "js"}, words("Let's encrypt node.js"))
}
