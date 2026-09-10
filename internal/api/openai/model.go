package openai

import (
	"encoding/json"
	"errors"
)

var errModelEmpty = errors.New("model is required")

func modelName(body []byte) (string, error) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", err
	}
	if req.Model == "" {
		return "", errModelEmpty
	}
	return req.Model, nil
}

func streamFlag(body []byte) bool {
	var req struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	return req.Stream
}
