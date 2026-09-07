package main

import "forst/bridgert"

const (
	forstBridgeGenStepDone  = "done"
	forstBridgeGenStepError = "error"
)

var forstBridgeManifestJSON string = "{\"version\":1,\"exports\":[{\"moduleId\":\"legacy/events.js\",\"name\":\"dispatch\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/events.js\",\"name\":\"subscribe\",\"kind\":\"asyncGenerator\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"concurrentEcho\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"create\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"delayed\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"failWithError\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/payment.js\",\"name\":\"failWithObject\",\"kind\":\"asyncFunction\"}]}"

type (
	forstBridgeGenStep_t_g2n6vmumv8n struct {
		Kind    string
		Message string
		Value   T_G2n6vMumV8n
	}
	forstBridgeSeq_t_g2n6vmumv8n struct {
		inner *bridgert.Seq[T_G2n6vMumV8n]
	}
)

func (s *forstBridgeSeq_t_g2n6vmumv8n) Close() {
	s.inner.Close()
}

func (s *forstBridgeSeq_t_g2n6vmumv8n) NextBatch(maxItems int) ([]forstBridgeGenStep_t_g2n6vmumv8n, error) {
	var raw, err = s.inner.NextBatch(maxItems)
	if err != nil {
		return nil, err
	}
	var out []forstBridgeGenStep_t_g2n6vmumv8n
	for _, step := range raw {
		out = append(out, forstBridgeGenStep_t_g2n6vmumv8n{Kind: string(step.Kind), Value: step.Value, Message: step.Message})
	}
	return out, nil
}

func forst_bridge_callasync_legacy_events_js_dispatch(arg0 T_G2n6vMumV8n) (struct {
}, error) {
	return bridgert.CallAsync[struct {
	}]("legacy/events.js", "dispatch", arg0)
}

func forst_bridge_callasync_legacy_payment_js_concurrentEcho(arg0 float64) (T_NTbLJjyksQg, error) {
	return bridgert.CallAsync[T_NTbLJjyksQg]("legacy/payment.js", "concurrentEcho", arg0)
}

func forst_bridge_callasync_legacy_payment_js_create(arg0 float64, arg1 string) (T_BSiWS9EsB18, error) {
	return bridgert.CallAsync[T_BSiWS9EsB18]("legacy/payment.js", "create", arg0, arg1)
}

func forst_bridge_open_seq_legacy_events_js_subscribe(arg0 string) (*forstBridgeSeq_t_g2n6vmumv8n, error) {
	var seq, err = bridgert.OpenSeq[T_G2n6vMumV8n]("legacy/events.js", "subscribe", bridgert.ExportKindAsyncGenerator, arg0)
	if err != nil {
		return nil, err
	}
	return &forstBridgeSeq_t_g2n6vmumv8n{inner: seq}, nil
}

func init() {
	bridgert.MustConfigureFromManifest(forstBridgeManifestJSON)
}
