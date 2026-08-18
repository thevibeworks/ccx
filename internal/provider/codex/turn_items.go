package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// Codex 0.147 made item_completed TurnItems the persisted, UI-facing
// transcript. Raw response_item messages are model I/O and can contain injected
// developer/user envelopes, so they are not a safe conversation source.
type completedTurnMessage struct {
	ID   string
	Role string
	Text string
}

type itemCompletedPayload struct {
	Type string          `json:"type"`
	Item json.RawMessage `json:"item"`
}

type turnItemMessagePayload struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Content []turnItemContent `json:"content"`
}

type turnItemContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
}

func hasCompletedTurnMessages(filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBufferBytes)
	for scanner.Scan() {
		var rollout rolloutLine
		if err := json.Unmarshal(scanner.Bytes(), &rollout); err != nil || rollout.Type != "event_msg" {
			continue
		}
		if _, ok := decodeCompletedTurnMessage(rollout.Payload); ok {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func decodeCompletedTurnMessage(raw json.RawMessage) (completedTurnMessage, bool) {
	var completed itemCompletedPayload
	if err := json.Unmarshal(raw, &completed); err != nil || completed.Type != "item_completed" {
		return completedTurnMessage{}, false
	}

	var item turnItemMessagePayload
	if err := json.Unmarshal(completed.Item, &item); err != nil {
		return completedTurnMessage{}, false
	}

	role := ""
	switch item.Type {
	case "UserMessage":
		role = "user"
	case "AgentMessage":
		role = "assistant"
	default:
		return completedTurnMessage{}, false
	}

	parts := make([]string, 0, len(item.Content))
	hasAttachment := false
	for _, content := range item.Content {
		switch strings.ToLower(content.Type) {
		case "text":
			if text := strings.TrimSpace(content.Text); text != "" {
				parts = append(parts, text)
			}
		case "image", "local_image":
			hasAttachment = true
		case "audio", "local_audio":
			hasAttachment = true
		case "skill", "mention":
			if name := strings.TrimSpace(content.Name); name != "" {
				parts = append(parts, "["+name+"]")
			}
		}
	}

	text := strings.Join(parts, "\n")
	if text == "" && hasAttachment {
		text = imageOnlyMessagePlaceholder
	}
	return completedTurnMessage{ID: item.ID, Role: role, Text: text}, true
}
