package main

import (
	"encoding/json"
	"forst/bridgert"
)

const (
	forstBridgeGenStepDone  = "done"
	forstBridgeGenStepError = "error"
)

var forstBridgeManifestJSON string = "{\"version\":1,\"exports\":[{\"moduleId\":\"legacy/activity.js\",\"name\":\"activityFeed\",\"kind\":\"asyncGenerator\"},{\"moduleId\":\"legacy/activity.js\",\"name\":\"dispatchActivity\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/activity.js\",\"name\":\"recentTitles\",\"kind\":\"generator\"},{\"moduleId\":\"legacy/todos.js\",\"name\":\"addTodo\",\"kind\":\"function\"},{\"moduleId\":\"legacy/todos.js\",\"name\":\"allTodos\",\"kind\":\"generator\"},{\"moduleId\":\"legacy/todos.js\",\"name\":\"bumpEditCount\",\"kind\":\"function\"},{\"moduleId\":\"legacy/todos.js\",\"name\":\"formatTodoList\",\"kind\":\"function\"},{\"moduleId\":\"legacy/todos.js\",\"name\":\"openCount\",\"kind\":\"function\"},{\"moduleId\":\"legacy/todos.js\",\"name\":\"persistSnapshot\",\"kind\":\"asyncFunction\"},{\"moduleId\":\"legacy/todos.js\",\"name\":\"todoCount\",\"kind\":\"function\"},{\"moduleId\":\"legacy/todos.js\",\"name\":\"toggleTodo\",\"kind\":\"function\"}]}"

type (
	forstBridgeGenStep_string struct {
		Kind    string
		Message string
		Value   string
	}
	forstBridgeSeq_string struct {
		inner *bridgert.Seq[string]
	}
)

type (
	forstBridgeGenStep_t_lkhz7dyfnqt struct {
		Kind    string
		Message string
		Value   T_LKhz7DyfNqT
	}
	forstBridgeSeq_t_lkhz7dyfnqt struct {
		inner *bridgert.Seq[T_LKhz7DyfNqT]
	}
)

func (s *forstBridgeSeq_string) Close() {
	s.inner.Close()
}

func (s *forstBridgeSeq_t_lkhz7dyfnqt) Close() {
	s.inner.Close()
}

func ForstBridgeWaitForShutdown() {
	bridgert.WaitForShutdown()
}

func (s *forstBridgeSeq_t_lkhz7dyfnqt) NextBatch(maxItems int) ([]forstBridgeGenStep_t_lkhz7dyfnqt, error) {
	var raw, err = s.inner.NextBatch(maxItems)
	if err != nil {
		return nil, err
	}
	var out []forstBridgeGenStep_t_lkhz7dyfnqt
	for _, step := range raw {
		out = append(out, forstBridgeGenStep_t_lkhz7dyfnqt{Kind: string(step.Kind), Value: step.Value, Message: step.Message})
	}
	return out, nil
}

func (s *forstBridgeSeq_string) NextBatch(maxItems int) ([]forstBridgeGenStep_string, error) {
	var raw, err = s.inner.NextBatch(maxItems)
	if err != nil {
		return nil, err
	}
	var out []forstBridgeGenStep_string
	for _, step := range raw {
		out = append(out, forstBridgeGenStep_string{Kind: string(step.Kind), Value: step.Value, Message: step.Message})
	}
	return out, nil
}

func forst_bridge_callasync_legacy_activity_js_dispatchActivity(arg0 T_LKhz7DyfNqT) (struct {
}, error) {
	return bridgert.CallAsync[struct {
	}]("legacy/activity.js", "dispatchActivity", arg0)
}

func forst_bridge_callasync_legacy_todos_js_persistSnapshot() (T_8ycLsMp1YzS, error) {
	return bridgert.CallAsyncArgs[T_8ycLsMp1YzS]("legacy/todos.js", "persistSnapshot", json.RawMessage("[]"))
}

func forst_bridge_callsync_legacy_todos_js_addTodo(arg0 string) (T_KuaRmDfgFpc, error) {
	return bridgert.CallSync[T_KuaRmDfgFpc]("legacy/todos.js", "addTodo", arg0)
}

func forst_bridge_callsync_legacy_todos_js_bumpEditCount() (float64, error) {
	return bridgert.CallSyncArgs[float64]("legacy/todos.js", "bumpEditCount", json.RawMessage("[]"))
}

func forst_bridge_callsync_legacy_todos_js_formatTodoList() (string, error) {
	return bridgert.CallSyncArgs[string]("legacy/todos.js", "formatTodoList", json.RawMessage("[]"))
}

func forst_bridge_callsync_legacy_todos_js_openCount() (float64, error) {
	return bridgert.CallSyncArgs[float64]("legacy/todos.js", "openCount", json.RawMessage("[]"))
}

func forst_bridge_callsync_legacy_todos_js_todoCount() (float64, error) {
	return bridgert.CallSyncArgs[float64]("legacy/todos.js", "todoCount", json.RawMessage("[]"))
}

func forst_bridge_callsync_legacy_todos_js_toggleTodo(arg0 string) (T_KuaRmDfgFpc, error) {
	return bridgert.CallSync[T_KuaRmDfgFpc]("legacy/todos.js", "toggleTodo", arg0)
}

func forst_bridge_open_seq_legacy_activity_js_activityFeed() (*forstBridgeSeq_t_lkhz7dyfnqt, error) {
	var seq, err = bridgert.OpenSeqArgs[T_LKhz7DyfNqT]("legacy/activity.js", "activityFeed", bridgert.ExportKindAsyncGenerator, json.RawMessage("[\"demo\"]"))
	if err != nil {
		return nil, err
	}
	return &forstBridgeSeq_t_lkhz7dyfnqt{inner: seq}, nil
}

func forst_bridge_open_seq_legacy_activity_js_recentTitles() (*forstBridgeSeq_string, error) {
	var seq, err = bridgert.OpenSeqArgs[string]("legacy/activity.js", "recentTitles", bridgert.ExportKindGenerator, json.RawMessage("[3]"))
	if err != nil {
		return nil, err
	}
	return &forstBridgeSeq_string{inner: seq}, nil
}

func init() {
	bridgert.MustConfigureFromManifest(forstBridgeManifestJSON)
}
