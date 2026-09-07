package main

import (
	"encoding/json"
	"forst/bridgert"
)

const (
	forstBridgeGenStepDone  = "done"
	forstBridgeGenStepError = "error"
)

var forstBridgeManifestJSON string = "{\"version\":1,\"exports\":[{\"moduleId\":\"legacy/generators.js\",\"name\":\"asyncNumbers\",\"kind\":\"asyncGenerator\"},{\"moduleId\":\"legacy/generators.js\",\"name\":\"emptyGen\",\"kind\":\"generator\"},{\"moduleId\":\"legacy/generators.js\",\"name\":\"finallyRunCount\",\"kind\":\"function\"},{\"moduleId\":\"legacy/generators.js\",\"name\":\"resetFinallyCount\",\"kind\":\"function\"},{\"moduleId\":\"legacy/generators.js\",\"name\":\"returnGen\",\"kind\":\"generator\"},{\"moduleId\":\"legacy/generators.js\",\"name\":\"syncNumbers\",\"kind\":\"generator\"},{\"moduleId\":\"legacy/generators.js\",\"name\":\"throwGen\",\"kind\":\"generator\"},{\"moduleId\":\"legacy/generators.js\",\"name\":\"withFinally\",\"kind\":\"generator\"}]}"

type (
	forstBridgeGenStep_float64 struct {
		Kind    string
		Message string
		Value   float64
	}
	forstBridgeSeq_float64 struct {
		inner *bridgert.Seq[float64]
	}
)

func (s *forstBridgeSeq_float64) Close() {
	s.inner.Close()
}

func (s *forstBridgeSeq_float64) NextBatch(maxItems int) ([]forstBridgeGenStep_float64, error) {
	var raw, err = s.inner.NextBatch(maxItems)
	if err != nil {
		return nil, err
	}
	var out []forstBridgeGenStep_float64
	for _, step := range raw {
		out = append(out, forstBridgeGenStep_float64{Kind: string(step.Kind), Value: step.Value, Message: step.Message})
	}
	return out, nil
}

func forst_bridge_open_seq_legacy_generators_js_asyncNumbers() (*forstBridgeSeq_float64, error) {
	var seq, err = bridgert.OpenSeqArgs[float64]("legacy/generators.js", "asyncNumbers", bridgert.ExportKindAsyncGenerator, json.RawMessage("[3]"))
	if err != nil {
		return nil, err
	}
	return &forstBridgeSeq_float64{inner: seq}, nil
}

func forst_bridge_open_seq_legacy_generators_js_emptyGen() (*forstBridgeSeq_float64, error) {
	var seq, err = bridgert.OpenSeqArgs[float64]("legacy/generators.js", "emptyGen", bridgert.ExportKindGenerator, json.RawMessage("[]"))
	if err != nil {
		return nil, err
	}
	return &forstBridgeSeq_float64{inner: seq}, nil
}

func forst_bridge_open_seq_legacy_generators_js_syncNumbers() (*forstBridgeSeq_float64, error) {
	var seq, err = bridgert.OpenSeqArgs[float64]("legacy/generators.js", "syncNumbers", bridgert.ExportKindGenerator, json.RawMessage("[3]"))
	if err != nil {
		return nil, err
	}
	return &forstBridgeSeq_float64{inner: seq}, nil
}

func forst_bridge_open_seq_legacy_generators_js_withFinally() (*forstBridgeSeq_float64, error) {
	var seq, err = bridgert.OpenSeqArgs[float64]("legacy/generators.js", "withFinally", bridgert.ExportKindGenerator, json.RawMessage("[]"))
	if err != nil {
		return nil, err
	}
	return &forstBridgeSeq_float64{inner: seq}, nil
}

func init() {
	bridgert.MustConfigureFromManifest(forstBridgeManifestJSON)
}
