package main

import (
	"encoding/json"
	"forst/bridgert"
)

const (
	forstBridgeGenStepDone  = "done"
	forstBridgeGenStepError = "error"
)

var forstBridgeManifestJSON string = "{\"version\":1,\"exports\":[{\"moduleId\":\"legacy/api/checkout.js\",\"name\":\"createOrder\",\"kind\":\"function\"}]}"

func forst_bridge_callsync_legacy_api_checkout_js_createOrder() (T_Zn4FXrBCht3, error) {
	return bridgert.CallSyncArgs[T_Zn4FXrBCht3]("legacy/api/checkout.js", "createOrder", json.RawMessage("[100,\"USD\"]"))
}

func init() {
	bridgert.MustConfigureFromManifest(forstBridgeManifestJSON)
}
