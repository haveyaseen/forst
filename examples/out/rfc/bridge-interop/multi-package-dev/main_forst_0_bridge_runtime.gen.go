package main

import (
	"encoding/json"
	"forst/bridgert"
)

const (
	forstBridgeGenStepDone  = "done"
	forstBridgeGenStepError = "error"
)

var forstBridgeManifestJSON string = "{\"version\":1,\"exports\":[{\"moduleId\":\"host.js\",\"name\":\"hostPing\",\"kind\":\"function\"}]}"

func ForstBridgeWaitForShutdown() {
	bridgert.WaitForShutdown()
}

func forst_bridge_callsync_host_js_hostPing() (string, error) {
	return bridgert.CallSyncArgs[string]("host.js", "hostPing", json.RawMessage("[]"))
}

func init() {
	bridgert.MustConfigureFromManifest(forstBridgeManifestJSON)
}
