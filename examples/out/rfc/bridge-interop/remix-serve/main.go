package main

import (
	fmt "fmt"
	os "os"
	"strconv"
)

// AddTodoRequest: TypeDefShapeExpr({title: String})
type AddTodoRequest struct {
	Title string `json:"title"`
}

// AddTodoResponse: TypeDefShapeExpr({id: String, title: String, status: String})
type AddTodoResponse struct {
	Id     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

// CompleteTodoRequest: TypeDefShapeExpr({id: String})
type CompleteTodoRequest struct {
	Id string `json:"id"`
}

// CompleteTodoResponse: TypeDefShapeExpr({id: String, title: String, status: String})
type CompleteTodoResponse struct {
	Id     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

// DashboardResponse: TypeDefShapeExpr({open: Int, recentTitles: String, activityKinds: String, savedAt: String})
type DashboardResponse struct {
	ActivityKinds string `json:"activityKinds"`
	Open          int    `json:"open"`
	RecentTitles  string `json:"recentTitles"`
	SavedAt       string `json:"savedAt"`
}

// ListTodosResponse: TypeDefShapeExpr({open: Int, done: Int, encoded: String})
type ListTodosResponse struct {
	Done    int    `json:"done"`
	Encoded string `json:"encoded"`
	Open    int    `json:"open"`
}

type T_7nWLvcjQ76D struct {
	ActivityKinds string  `json:"activityKinds"`
	Open          float64 `json:"open"`
	RecentTitles  string  `json:"recentTitles"`
	SavedAt       string  `json:"savedAt"`
}

type T_8ycLsMp1YzS struct {
	SavedAt string `json:"savedAt"`
}

type T_D415raHQ7uQ struct {
	Done    float64 `json:"done"`
	Encoded string  `json:"encoded"`
	Open    float64 `json:"open"`
}

type T_KuaRmDfgFpc struct {
	Id     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

type T_LKhz7DyfNqT struct {
	Kind string `json:"kind"`
}

func AddTodo(input AddTodoRequest) (AddTodoResponse, error) {
	println("api:AddTodo:" + input.Title)
	created, createdErr := forst_bridge_callsync_legacy_todos_js_addTodo(input.Title)
	if createdErr != nil {
		return AddTodoResponse{Status: "", Id: "", Title: ""}, createdErr
	}
	return AddTodoResponse{Id: created.Id, Title: created.Title, Status: created.Status}, nil
}

func CompleteTodo(input CompleteTodoRequest) (CompleteTodoResponse, error) {
	println("api:CompleteTodo:" + input.Id)
	updated, updatedErr := forst_bridge_callsync_legacy_todos_js_toggleTodo(input.Id)
	if updatedErr != nil {
		return CompleteTodoResponse{Status: "", Id: "", Title: ""}, updatedErr
	}
	return CompleteTodoResponse{Id: updated.Id, Title: updated.Title, Status: updated.Status}, nil
}

func GetDashboard() (T_7nWLvcjQ76D, error) {
	println("api:GetDashboard")
	open, openErr := forst_bridge_callsync_legacy_todos_js_openCount()
	if openErr != nil {
		return T_7nWLvcjQ76D{ActivityKinds: "", SavedAt: "", Open: 0.0, RecentTitles: ""}, openErr
	}
	snap, snapErr := forst_bridge_callasync_legacy_todos_js_persistSnapshot()
	if snapErr != nil {
		return T_7nWLvcjQ76D{Open: 0.0, RecentTitles: "", ActivityKinds: "", SavedAt: ""}, snapErr
	}
	return T_7nWLvcjQ76D{Open: open, RecentTitles: "", ActivityKinds: "ready", SavedAt: snap.SavedAt}, nil
}

func ListTodos() (T_D415raHQ7uQ, error) {
	println("api:ListTodos")
	encoded, encodedErr := forst_bridge_callsync_legacy_todos_js_formatTodoList()
	if encodedErr != nil {
		return T_D415raHQ7uQ{Open: 0.0, Done: 0.0, Encoded: ""}, encodedErr
	}
	open, openErr := forst_bridge_callsync_legacy_todos_js_openCount()
	if openErr != nil {
		return T_D415raHQ7uQ{Encoded: "", Open: 0.0, Done: 0.0}, openErr
	}
	total, totalErr := forst_bridge_callsync_legacy_todos_js_todoCount()
	if totalErr != nil {
		return T_D415raHQ7uQ{Done: 0.0, Encoded: "", Open: 0.0}, totalErr
	}
	done := total - open
	return T_D415raHQ7uQ{Open: open, Done: done, Encoded: encoded}, nil
}

func main() {
	first, firstErr := forst_bridge_callsync_legacy_todos_js_bumpEditCount()
	if firstErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", firstErr)
			os.Exit(1)
		}
	}
	println("sync:" + strconv.FormatFloat(first, 'f', 0, 64))
	second, secondErr := forst_bridge_callsync_legacy_todos_js_bumpEditCount()
	if secondErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", secondErr)
			os.Exit(1)
		}
	}
	println("sync:" + strconv.FormatFloat(second, 'f', 0, 64))
	snap, snapErr := forst_bridge_callasync_legacy_todos_js_persistSnapshot()
	if snapErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", snapErr)
			os.Exit(1)
		}
	}
	println("async:" + snap.SavedAt)
	var titleCount int = 0
	titleSeq, titleSeqErr := forst_bridge_open_seq_legacy_activity_js_recentTitles()
	if titleSeqErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", titleSeqErr)
			os.Exit(1)
		}
	}
	{
		_nodeIt := titleSeq
		defer _nodeIt.Close()
		var (
			_nodeStep     forstBridgeGenStep_string
			_nodeBatch    []forstBridgeGenStep_string
			_nodeBatchIdx int
			_nodeBatchErr error
		)
		_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
		if _nodeBatchErr != nil {
			panic(_nodeBatchErr)
		}
		_nodeBatchIdx = 0
		for {
			if _nodeBatchIdx >= len(_nodeBatch) {
				_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
				if _nodeBatchErr != nil {
					panic(_nodeBatchErr)
				}
				_nodeBatchIdx = 0
			}
			_nodeStep = _nodeBatch[_nodeBatchIdx]
			_nodeBatchIdx++
			if _nodeStep.Kind == forstBridgeGenStepDone {
				break
			}
			if _nodeStep.Kind == forstBridgeGenStepError {
				panic(_nodeStep.Message)
			}
			titleCount = titleCount + 1
		}
	}
	println("gen:" + strconv.Itoa(titleCount))
	var feedCount int = 0
	feed, feedErr := forst_bridge_open_seq_legacy_activity_js_activityFeed()
	if feedErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", feedErr)
			os.Exit(1)
		}
	}
	{
		_nodeIt := feed
		defer _nodeIt.Close()
		var (
			_nodeStep     forstBridgeGenStep_t_lkhz7dyfnqt
			_nodeBatch    []forstBridgeGenStep_t_lkhz7dyfnqt
			_nodeBatchIdx int
			_nodeBatchErr error
		)
		_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
		if _nodeBatchErr != nil {
			panic(_nodeBatchErr)
		}
		_nodeBatchIdx = 0
		for {
			if _nodeBatchIdx >= len(_nodeBatch) {
				_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
				if _nodeBatchErr != nil {
					panic(_nodeBatchErr)
				}
				_nodeBatchIdx = 0
			}
			_nodeStep = _nodeBatch[_nodeBatchIdx]
			_nodeBatchIdx++
			if _nodeStep.Kind == forstBridgeGenStepDone {
				break
			}
			if _nodeStep.Kind == forstBridgeGenStepError {
				panic(_nodeStep.Message)
			}
			evt := _nodeStep.Value
			forst_bridge_callasync_legacy_activity_js_dispatchActivity(evt)
			feedCount = feedCount + 1
		}
	}
	println("events:" + strconv.Itoa(feedCount))
	ForstInvokeWaitForShutdown()
}
