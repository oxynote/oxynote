package mathutil

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_SafeDiv(t *testing.T) {
	cc := map[string]struct {
		A      decimal.Decimal
		B      decimal.Decimal
		Result decimal.Decimal
	}{
		"Division by zero returns zero": {
			A:      decimal.NewFromInt(42),
			B:      decimal.Zero,
			Result: decimal.Zero,
		},
		"Successful division": {
			A:      decimal.NewFromInt(50),
			B:      decimal.NewFromInt(200),
			Result: decimal.NewFromFloat(0.25),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res := SafeDiv(c.A, c.B)
			assert.Equal(t, c.Result.String(), res.String())
		})
	}
}
