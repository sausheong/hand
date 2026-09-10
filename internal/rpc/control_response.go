package rpc

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/sausheong/hand/protocol"
)

func validateControlResponse(record RequestRecord) error {
	fields, err := ledgerObject(record.Result)
	if err != nil {
		return err
	}
	for key := range fields {
		switch key {
		case "version", "request_id", "result", "error":
		default:
			return errors.New("unknown control response field")
		}
	}
	if raw, exists := fields["error"]; exists {
		detail, err := ledgerObject(raw)
		if err != nil {
			return err
		}
		if len(detail) != 2 || detail["code"] == nil || detail["message"] == nil {
			return errors.New("control error requires exact code and message fields")
		}
		var code, message *string
		if err := json.Unmarshal(detail["code"], &code); err != nil {
			return err
		}
		if err := json.Unmarshal(detail["message"], &message); err != nil {
			return err
		}
		if code == nil || strings.TrimSpace(*code) == "" || message == nil {
			return errors.New("invalid control error values")
		}
	}
	var response protocol.Response
	if err := json.Unmarshal(record.Result, &response); err != nil {
		return errors.New("invalid control response")
	}
	if response.Version != protocol.Version || response.RequestID != record.ID || (response.Error == nil) == (len(response.Result) == 0) {
		return errors.New("invalid control response identity or result")
	}
	return nil
}
