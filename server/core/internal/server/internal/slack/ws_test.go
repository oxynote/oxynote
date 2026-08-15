package slack

import (
	"testing"

	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Handler_BindPostMessage(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	tpc := &wsMock.Topic{}

	hdl.BindPostMessage(tpc)
	require.NotNil(t, hdl.message.postCallback)

	hdl.message.postCallback()

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, Message{}, pubs[0].Payload)
	assert.Nil(t, pubs[0].Filter)
}
