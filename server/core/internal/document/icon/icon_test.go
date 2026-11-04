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
