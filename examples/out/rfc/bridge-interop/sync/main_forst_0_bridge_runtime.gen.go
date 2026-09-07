package main

import (
	"encoding/json"
	"forst/bridgert"
)

const (
	forstBridgeGenStepDone  = "done"
	forstBridgeGenStepError = "error"
)

var forstBridgeManifestJSON string = "{\"version\":1,\"exports\":[{\"moduleId\":\"legacy/payment.js\",\"name\":\"create\",\"kind\":\"function\"}]}"

func forst_bridge_callsync_legacy_payment_js_create() (T_BSiWS9EsB18, error) {
	return bridgert.CallSyncArgs[T_BSiWS9EsB18]("legacy/payment.js", "create", json.RawMessage("[100,\"USD\"]"))
}

func init() {
	bridgert.MustConfigureFromManifest(forstBridgeManifestJSON)
}
