package ioutil

import (
	"errors"
	"io"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_MultiCloser(t *testing.T) {
	c1, c2 := &closerMock{}, &closerMock{}
	mc := MultiCloser(true, c1, c2)
	require.NotNil(t, mc)
	require.IsType(t, &multiCloser{}, mc)
	require.Len(t, mc.(*multiCloser).closers, 2)
	require.True(t, mc.(*multiCloser).force)
	assert.Same(t, c1, mc.(*multiCloser).closers[0])
	assert.Same(t, c2, mc.(*multiCloser).closers[1])
}

func Test_multiClose_Close(t *testing.T) {
	// error
	c1, c2 := &closerMock{err: assert.AnError}, &closerMock{}
	mc := &multiCloser{closers: []io.Closer{c1, c2}}
	err := mc.Close()
	require.Error(t, err)

	_, ok := err.(*multierror.Error) //nolint:errorlint // it's a test
	require.False(t, ok)
	assert.True(t, c1.called)
	assert.False(t, c2.called)

	// multi error
	c1, c2 = &closerMock{err: assert.AnError}, &closerMock{err: assert.AnError}
	mc = &multiCloser{closers: []io.Closer{c1, c2}, force: true}
	err = mc.Close()
	require.Error(t, err)
	require.IsType(t, &multierror.Error{}, err)      //nolint:testifylint // a direct type check is needed
	assert.Len(t, err.(*multierror.Error).Errors, 2) //nolint:errorlint // it's a test
	assert.True(t, c1.called)
	assert.True(t, c2.called)

	// success
	c1, c2 = &closerMock{}, &closerMock{}
	mc = &multiCloser{closers: []io.Closer{c1, c2}}
	assert.NoError(t, mc.Close())
	assert.True(t, c1.called)
	assert.True(t, c2.called)
}

func Test_CloserFunc_Close(t *testing.T) {
	fn := CloserFunc(func() error {
		return assert.AnError
	})

	assert.Error(t, fn.Close())

	fn = CloserFunc(func() error {
		return nil
	})

	assert.NoError(t, fn.Close())
}

func Test_AppendCloseErr(t *testing.T) {
	// closer error
	err1 := errors.New("err1")
	err2 := errors.New("err2")

	cl := &closerMock{
		err: err2,
	}

	err := AppendCloseErr(cl, err1)
	assert.EqualError(t, err, "2 errors occurred:\n\t* err1\n\t* err2\n\n")

	// closer no error
	cl = &closerMock{}
	err = AppendCloseErr(cl, err1)

	assert.EqualError(t, err, "err1")
}

type closerMock struct {
	called bool
	err    error
}

func (c *closerMock) Close() error {
	c.called = true
	return c.err
}
