package main

import (
	"encoding/json"
	"forst/bridgert"
)

const (
	forstBridgeGenStepDone  = "done"
	forstBridgeGenStepError = "error"
)

var forstBridgeManifestJSON string = "{\"version\":1,\"exports\":[{\"moduleId\":\"legacy/counter.js\",\"name\":\"inc\",\"kind\":\"function\"}]}"

func ForstBridgeWaitForShutdown() {
	bridgert.WaitForShutdown()
}

func forst_bridge_callsync_legacy_counter_js_inc() (float64, error) {
	return bridgert.CallSyncArgs[float64]("legacy/counter.js", "inc", json.RawMessage("[]"))
}

func init() {
	bridgert.MustConfigureFromManifest(forstBridgeManifestJSON)
}
