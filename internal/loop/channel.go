package loop

import (
	"fmt"

	"github.com/eachlabs/klaw/pkg/channel"
)

func progressMsg(format string, args ...interface{}) *channel.Message {
	return &channel.Message{
		Role:    "assistant",
		Content: fmt.Sprintf("🔄 "+format, args...),
	}
}

func completionMsg(format string, args ...interface{}) *channel.Message {
	return &channel.Message{
		Role:    "assistant",
		Content: fmt.Sprintf("✅ "+format, args...),
	}
}

func errorMsg(format string, args ...interface{}) *channel.Message {
	return &channel.Message{
		Role:    "assistant",
		Content: fmt.Sprintf("⚠️ "+format, args...),
	}
}
