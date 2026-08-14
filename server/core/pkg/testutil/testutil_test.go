package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_FilterEqual(t *testing.T) {
	type object struct {
		Name      string
		CreatedAt time.Time
	}

	// error - values differ
	err := FilterEqual(object{Name: "a"}, object{Name: "b"})
	assert.Error(t, err)

	// success - ignored types are not compared
	err = FilterEqual(
		object{Name: "a", CreatedAt: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC)},
		object{Name: "a", CreatedAt: time.Date(2022, 3, 4, 0, 0, 0, 0, time.UTC)},
		time.Time{},
	)
	assert.NoError(t, err)

	// success - equal instants in different locations are equal
	err = FilterEqual(
		object{CreatedAt: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC)},
		object{CreatedAt: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC).Local()},
	)
	assert.NoError(t, err)
}

func Test_AssertFilterEqual(t *testing.T) {
	// error - values differ
	nt := &testing.T{}

	AssertFilterEqual(nt, 1, 2)
	assert.True(t, nt.Failed())

	// success
	nt = &testing.T{}

	AssertFilterEqual(nt, 1, 1)
	assert.False(t, nt.Failed())
}
