package sessionio

import (
	"encoding/json"
	"reflect"
	"testing"
)

func FuzzUsageRecordJSON(f *testing.F) {
	f.Add(byte(0), []byte(`{"version":1,"request":{"request_id":"one","model":"m","category":"generation","status":"completed","source":"reported","usage":{"input_tokens":10,"output_tokens":2}}}`))
	f.Add(byte(0), []byte(`{"version":1,"request":{"request_id":"one","model":"m","category":"generation","status":"failed","source":"unavailable","usage":null}}`))
	f.Add(byte(1), []byte(`{"version":1,"prior_usage_unknown":false}`))
	f.Add(byte(1), []byte(`{"version":1,"prior_usage_unknown":true,"prior_usage_unknown":false}`))
	f.Fuzz(func(t *testing.T, kind byte, raw []byte) {
		if len(raw) > 128<<10 {
			return
		}
		if kind%2 == 0 {
			var first, second usageRecord
			if err := decodeUsageRecord(raw, &first); err != nil {
				return
			}
			canonical, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			if err := decodeUsageRecord(canonical, &second); err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("accepted usage round trip failed: %v", err)
			}
		} else {
			var first, second usageTracking
			if err := decodeUsageTracking(raw, &first); err != nil {
				return
			}
			canonical, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			if err := decodeUsageTracking(canonical, &second); err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("accepted tracking round trip failed: %v", err)
			}
		}
	})
}
