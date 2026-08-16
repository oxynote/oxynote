package testutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_AddChiCtx(t *testing.T) {
	// fresh context - a chi route context is created
	ctx := AddChiCtx(context.Background(), "key", "value")

	rctx, ok := ctx.Value(chi.RouteCtxKey).(*chi.Context)
	require.True(t, ok)
	assert.Equal(t, "value", rctx.URLParam("key"))

	// existing context - the chi route context is reused
	ctx2 := AddChiCtx(ctx, "key2", "value2")

	assert.Equal(t, ctx, ctx2)
	assert.Equal(t, "value", rctx.URLParam("key"))
	assert.Equal(t, "value2", rctx.URLParam("key2"))
}

func Test_AssertEqualError(t *testing.T) {
	exp := errors.New("expected error")

	// success - no error expected and none occurred
	nt := &testing.T{}
	AssertEqualError(nt, nil, nil)
	assert.False(t, nt.Failed())

	// error - no error expected but one occurred
	nt = &testing.T{}
	AssertEqualError(nt, nil, assert.AnError)
	assert.True(t, nt.Failed())

	// success - generic error expected and one occurred
	nt = &testing.T{}
	AssertEqualError(nt, assert.AnError, errors.New("error"))
	assert.False(t, nt.Failed())

	// error - generic error expected but none occurred
	nt = &testing.T{}
	AssertEqualError(nt, assert.AnError, nil)
	assert.True(t, nt.Failed())

	// success - concrete error expected and matched
	nt = &testing.T{}
	AssertEqualError(nt, exp, exp)
	assert.False(t, nt.Failed())

	// error - concrete error expected but a different one occurred
	nt = &testing.T{}
	AssertEqualError(nt, exp, errors.New("other error"))
	assert.True(t, nt.Failed())
}

func Test_RequireEqualError(t *testing.T) {
	// require assertions stop the calling goroutine via FailNow on
	// failure, so every call runs in its own goroutine.
	run := func(exp, err error) *testing.T {
		nt := &testing.T{}
		done := make(chan struct{})

		go func() {
			defer close(done)

			RequireEqualError(nt, exp, err) //nolint:testifylint // the helper's require behavior is under test
		}()

		<-done

		return nt
	}

	exp := errors.New("expected error")

	// success - no error expected and none occurred
	assert.False(t, run(nil, nil).Failed())

	// error - no error expected but one occurred
	assert.True(t, run(nil, assert.AnError).Failed())

	// success - generic error expected and one occurred
	assert.False(t, run(assert.AnError, errors.New("error")).Failed())

	// error - generic error expected but none occurred
	assert.True(t, run(assert.AnError, nil).Failed())

	// success - concrete error expected and matched
	assert.False(t, run(exp, exp).Failed())

	// error - concrete error expected but a different one occurred
	assert.True(t, run(exp, errors.New("other error")).Failed())
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

	// success - unexported fields are compared as well
	type hidden struct {
		name string
	}

	err = FilterEqual(hidden{name: "a"}, hidden{name: "a"})
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

func Test_MockHTTP(t *testing.T) {
	client, mt := MockHTTP()

	require.NotNil(t, client)
	require.NotNil(t, mt)
	assert.Same(t, mt, client.Transport)
}

func Test_NewBuffer(t *testing.T) {
	w, b := NewBuffer()

	require.NotNil(t, w)
	require.NotNil(t, b)
	assert.NotNil(t, w.out)
}

func Test_Writer_Write(t *testing.T) {
	w, _ := NewBuffer()

	n, err := w.Write([]byte("data"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
}

func Test_Writer_Flush(t *testing.T) {
	w, b := NewBuffer()

	_, err := w.Write([]byte("data"))
	require.NoError(t, err)
	require.Empty(t, b.String())

	require.NoError(t, w.Flush())
	assert.Equal(t, "data", b.String())
}
