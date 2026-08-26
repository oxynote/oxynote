package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_newLogger(t *testing.T) {
	t.Parallel()

	log := newLogger()
	require.NotNil(t, log)
	assert.NotNil(t, log.Handler())
}
