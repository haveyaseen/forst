package main

import (
	"encoding/json"
	"forst/bridgert"
)

const (
	forstBridgeGenStepDone  = "done"
	forstBridgeGenStepError = "error"
)

var forstBridgeManifestJSON string = "{\"version\":1,\"exports\":[{\"moduleId\":\"legacy/payment.js\",\"name\":\"concurrentEcho\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"create\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"delayed\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"failWithError\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"failWithObject\",\"kind\":\"asyncFunction\"}]}"

func forst_bridge_callasync_legacy_payment_js_concurrentEcho() (T_NTbLJjyksQg, error) {
	return bridgert.CallAsyncArgs[T_NTbLJjyksQg]("legacy/payment.js", "concurrentEcho", json.RawMessage("[7]"))
}

func forst_bridge_callasync_legacy_payment_js_create() (T_BSiWS9EsB18, error) {
	return bridgert.CallAsyncArgs[T_BSiWS9EsB18]("legacy/payment.js", "create", json.RawMessage("[100,\"USD\"]"))
}

func init() {
	bridgert.MustConfigureFromManifest(forstBridgeManifestJSON)
}
