package tcwh

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nicklaw5/helix/v2"
	"github.com/stretchr/testify/require"
)

func TestFormat(t *testing.T) {
	t.Parallel()

	var event helix.EventSubChannelChatMessageEvent
	require.NoError(t, json.Unmarshal(TestEvent, &event))

	fmt.Println(hex.EncodeToString([]byte(DefaultTemplate)))
}
