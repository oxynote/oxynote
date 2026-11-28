package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_EnvName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "OXYNOTE_CORE_FOO", EnvName("FOO"))
}

func Test_Getenv(t *testing.T) {
	t.Setenv("OXYNOTE_CORE_TEST_KEY", "test-value")

	assert.Equal(t, "test-value", Getenv("TEST_KEY"))
}
