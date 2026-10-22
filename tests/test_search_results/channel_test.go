package test_search_results

import (
	"encoding/json"
	"testing"

	"github.com/hngprojects/telex_be/utility"
)

func TestSearchResultChannelName(t *testing.T) {
	for _, index := range []string{"threads", "cybertron_threads", "custom-threads", "threads_threads", "threads_v2", "cybertron_threads_v1"} {
		t.Run(index, func(t *testing.T) {
			source := map[string]any{"channels_id": "channel-1", "channel_name": "general", "id": "thread-1", "message": "hello"}
			result := utility.ProcessMessageHit(index, source)
			if result.Channel.ChannelID != "channel-1" || result.Channel.ChannelName != "general" {
				t.Fatalf("channel = %+v, want channel-1/general", result.Channel)
			}
			body, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatal(err)
			}
			if response["channel"].(map[string]any)["channel_name"] != "general" {
				t.Fatalf("response lost channel name: %s", body)
			}
		})
	}
}

func TestSearchResultWithoutThreadChannelName(t *testing.T) {
	for _, index := range []string{"messages", "cybertron_messages", "custom_messages"} {
		t.Run(index, func(t *testing.T) {
			result := utility.ProcessMessageHit(index, map[string]any{"channels_id": "channel-1", "channel_name": "ignored"})
			if result.Channel.ChannelName != "" {
				t.Fatalf("message hit unexpectedly includes thread-only channel name: %+v", result.Channel)
			}
		})
	}
	for _, value := range []any{nil, 123, ""} {
		result := utility.ProcessMessageHit("cybertron_threads", map[string]any{"channel_name": value})
		if result.Channel.ChannelName != "" {
			t.Fatalf("unexpected channel name for %v: %+v", value, result.Channel)
		}
	}
}
